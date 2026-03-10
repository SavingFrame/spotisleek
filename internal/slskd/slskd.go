package slskd

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

const apiVersion = "v0"

type Slskd struct {
	httpClient    *http.Client
	uri           string
	apiKey        string
	moveFile      bool
	downloadsPath string
	musicPath     string
}

func NewSlskd(uri, apiKey string, moveFile bool, downloads_path, music_path string) *Slskd {
	return &Slskd{
		uri:           strings.TrimRight(uri, "/"),
		apiKey:        apiKey,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		moveFile:      moveFile,
		downloadsPath: downloads_path,
		musicPath:     music_path,
	}
}

func (s *Slskd) DownloadSong(ctx context.Context, song *domain.Song) error {
	searchResult, err := s.searchSong(ctx, song)
	if err != nil {
		return err
	}
	// add if to check isComplete and responseCount > 0, otherwise we might end up in a loop if the search fails for some reason
	retryCount := 0
	searchTicker := time.NewTicker(5 * time.Second)
	defer searchTicker.Stop()
	for !searchResult.IsComplete {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-searchTicker.C:
			searchResult, err = s.getSearchStatus(ctx, searchResult.ID)
			if err != nil {
				return err
			}
			retryCount++
			if retryCount > 10 {
				return fmt.Errorf("search did not complete after %d retries", retryCount)
			}
		}
	}
	results, err := s.getSearchResponses(ctx, searchResult.ID)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no responses found for search %s", searchResult.ID)
	}
	files := s.findSongInResponses(results, song)
	if len(files) == 0 {
		return fmt.Errorf("no matching files found in search responses")
	}

	const (
		downloadPollInterval = 5 * time.Second
		downloadTimeout      = 15 * time.Minute
		stallTimeout         = 3 * time.Minute
	)

fileloop:
	for i, file := range files {
		transfer, err := s.enqueueDownload(ctx, &file)
		if err != nil {
			slog.Warn("Failed to enqueue download for file, trying next one", "error", err, "filename", file.Filename)
			continue
		}
		slog.Info("Enqueued download", "index", i, "filename", file.Filename, "username", transfer.Username, "downloadID", transfer.ID)

		startedAt := time.Now()
		lastProgressAt := time.Now()
		lastBytesTransferred := transfer.BytesTransferred

		ticker := time.NewTicker(downloadPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				transfer, err = s.getDownload(ctx, transfer.Username, transfer.ID)
				if err != nil {
					slog.Warn("Failed to get download status, retrying", "error", err, "username", transfer.Username, "downloadID", transfer.ID)
					continue fileloop
				}
				if transfer.BytesTransferred > lastBytesTransferred {
					lastProgressAt = time.Now()
					lastBytesTransferred = transfer.BytesTransferred
				}
				if transfer.State == "Completed, Succeeded" {
					slog.Info("Download completed successfully", "filename", file.Filename, "username", transfer.Username, "downloadID", transfer.ID)
					song.FilePath = file.Filename
					return nil
				} else if strings.HasPrefix(transfer.State, "Completed") {
					slog.Warn("Download completed with non-success state, trying next file", "state", transfer.State, "filename", file.Filename, "username", transfer.Username, "downloadID", transfer.ID)
					continue fileloop
				}

				if time.Since(startedAt) >= downloadTimeout || time.Since(lastProgressAt) >= stallTimeout {
					slog.Warn("Download timed out, trying next file", "filename", file.Filename, "username", transfer.Username, "downloadID", transfer.ID)
					// TODO: Cancel the download in SLSKD if possible
					continue fileloop
				}
			}
		}
	}
	return fmt.Errorf("failed to download song: no more matching files in search responses")
}

func (s *Slskd) findSongInResponses(responses []*searchResponse, song *domain.Song) []searchFile {
	songTitle := sanitizeString(song.Title)
	songArtist := sanitizeString(song.Artists[0])
	songAlbum := sanitizeString(song.Album)
	var files []searchFile
	for _, response := range responses {
		if response.FileCount < 1 || !response.HasFreeUploadSlot {
			continue
		}
		for _, file := range response.Files {
			if file.Extension == "" {
				file.Extension = strings.TrimPrefix(path.Ext(file.Filename), ".")
			}
			file.Extension = strings.ToLower(file.Extension)
			if file.Extension != "flac" {
				continue
			}
			if song.Duration.Abs().Seconds() > 0 && song.Duration.Abs().Seconds()-float64(*file.Length) > 5 {
				continue
			}
			sanitizedFilename := sanitizeString(file.Filename)
			if (strings.Contains(strings.ToLower(sanitizedFilename), strings.ToLower(songAlbum)) || strings.Contains(strings.ToLower(sanitizedFilename), strings.ToLower(songArtist))) && strings.Contains(strings.ToLower(sanitizedFilename), strings.ToLower(songTitle)) {
				file.username = response.Username
				files = append(files, file)
			}
		}
	}
	slices.SortFunc(files, func(a, b searchFile) int {
		ab, bb := derefOr(a.BitRate, 0), derefOr(b.BitRate, 0)
		if bb != ab {
			return cmp.Compare(bb, ab) // higher bitrate first
		}
		as, bs := derefOr(a.SampleRate, 0), derefOr(b.SampleRate, 0)
		return cmp.Compare(bs, as) // higher sample rate first
	})

	return files
}

var alnumRe = regexp.MustCompile(`[^\p{L}\d]+`)

// AlnumOnly removes everything except letters and digits
func sanitizeString(s string) string {
	return alnumRe.ReplaceAllString(s, "")
}

func derefOr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}
