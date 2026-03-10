package slskd

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

const apiVersion = "v0"

type Slskd struct {
	httpClient *http.Client
	uri        string
	apiKey     string
}

type Transfer struct {
	AverageSpeed     float64 `json:"averageSpeed"`
	BytesRemaining   int64   `json:"bytesRemaining"`
	BytesTransferred int64   `json:"bytesTransferred"`
	Direction        string  `json:"direction"`
	EndTime          *string `json:"endTime"`
	Filename         string  `json:"filename"`
	ID               string  `json:"id"`
	PercentComplete  float64 `json:"percentComplete"`
	PlaceInQueue     *int    `json:"placeInQueue"`
	Size             int64   `json:"size"`
	StartOffset      int64   `json:"startOffset"`
	StartTime        *string `json:"startTime"`
	State            string  `json:"state"`
	Token            int     `json:"token"`
	Username         string  `json:"username"`
	Exception        *string `json:"exception"`
}

type QueueDownloadRequest struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size,omitempty"`
}

type enqueueDownloadResponse struct {
	Enqueued []Transfer `json:"enqueued"`
	Failed   []string   `json:"failed"`
}

type search struct {
	EndedAt         *time.Time `json:"endedAt"`
	FileCount       int        `json:"fileCount"`
	ID              string     `json:"id"`
	IsComplete      bool       `json:"isComplete"`
	LockedFileCount int        `json:"lockedFileCount"`
	ResponseCount   int        `json:"responseCount"`
	SearchText      string     `json:"searchText"`
	StartedAt       time.Time  `json:"startedAt"`
	State           string     `json:"state"`
	Token           int        `json:"token"`
}

type searchResponse struct {
	FileCount         int          `json:"fileCount"`
	Files             []searchFile `json:"files"`
	HasFreeUploadSlot bool         `json:"hasFreeUploadSlot"`
	LockedFileCount   int          `json:"lockedFileCount"`
	LockedFiles       []searchFile `json:"lockedFiles"`
	QueueLength       int64        `json:"queueLength"`
	Token             int          `json:"token"`
	UploadSpeed       int          `json:"uploadSpeed"`
	Username          string       `json:"username"`
}

type searchFile struct {
	BitDepth          *int   `json:"bitDepth"`
	BitRate           *int   `json:"bitRate"`
	Code              int    `json:"code"`
	Extension         string `json:"extension"`
	Filename          string `json:"filename"`
	IsVariableBitRate *bool  `json:"isVariableBitRate"`
	Length            *int   `json:"length"`
	SampleRate        *int   `json:"sampleRate"`
	Size              int64  `json:"size"`
	IsLocked          bool   `json:"isLocked"`
	username          string
}

func NewSlskd(uri, apiKey string) *Slskd {
	return &Slskd{
		uri:        strings.TrimRight(uri, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
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

func (s *Slskd) enqueueDownload(ctx context.Context, file *searchFile) (*Transfer, error) {
	if strings.TrimSpace(file.username) == "" {
		return nil, fmt.Errorf("username is empty")
	}
	var validFiles []QueueDownloadRequest
	validFiles = append(validFiles, QueueDownloadRequest{
		Filename: file.Filename,
		Size:     file.Size,
	})

	var result enqueueDownloadResponse
	err := s.doJSONRequest(ctx, http.MethodPost, path.Join("/transfers/downloads", file.username), nil, validFiles, &result, http.StatusCreated)
	if err != nil {
		slog.Error("Failed to enqueue download", "error", err, "username", file.username)
		return nil, err
	}
	if len(result.Enqueued) == 0 {
		if len(result.Failed) > 0 {
			return nil, fmt.Errorf("enqueue download failed: %s", strings.Join(result.Failed, ", "))
		}
		return nil, fmt.Errorf("enqueue download returned no enqueued transfers")
	}
	return &result.Enqueued[0], nil
}

func (s *Slskd) getDownload(ctx context.Context, username, id string) (*Transfer, error) {
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("username is empty")
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("download id is empty")
	}

	var result Transfer
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/transfers/downloads", username, id), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to get download info", "error", err, "username", username, "downloadID", id)
		return nil, err
	}
	return &result, nil
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

func (s *Slskd) searchSong(ctx context.Context, track *domain.Song) (*search, error) {
	if track == nil {
		return nil, fmt.Errorf("song is nil")
	}
	if len(track.Artists) == 0 || strings.TrimSpace(track.Artists[0]) == "" {
		return nil, fmt.Errorf("song has no artists")
	}
	if strings.TrimSpace(track.Title) == "" {
		return nil, fmt.Errorf("song title is empty")
	}

	body := map[string]string{
		"searchText": fmt.Sprintf("%s - %s", track.Artists[0], track.Title),
	}

	var result search
	err := s.doJSONRequest(ctx, http.MethodPost, "/searches", nil, body, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to start SLSKD search", "error", err, "searchText", body["searchText"])
		return nil, err
	}

	return &result, nil
}

func (s *Slskd) getSearchStatus(ctx context.Context, id string) (*search, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("search id is empty")
	}

	var result search
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/searches", id), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to fetch SLSKD search status", "error", err, "searchID", id)
		return nil, err
	}

	return &result, nil
}

func (s *Slskd) getSearchResponses(ctx context.Context, id string) ([]*searchResponse, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("search id is empty")
	}

	var result []*searchResponse
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/searches", id, "responses"), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to fetch SLSKD search responses", "error", err, "searchID", id)
		return nil, err
	}

	return result, nil
}

func (s *Slskd) doJSONRequest(ctx context.Context, method, endpointPath string, query url.Values, requestBody any, responseBody any, expectedStatusCodes ...int) error {
	requestURL, err := s.buildURL(endpointPath, query)
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	if requestBody != nil {
		body, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", s.apiKey)

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if !statusExpected(res.StatusCode, expectedStatusCodes) {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("unexpected status code %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if responseBody == nil {
		return nil
	}

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if len(payload) == 0 {
		return nil
	}

	if err := json.Unmarshal(payload, responseBody); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (s *Slskd) buildURL(endpointPath string, query url.Values) (string, error) {
	base, err := url.Parse(s.uri)
	if err != nil {
		return "", fmt.Errorf("parse slskd uri: %w", err)
	}
	base.Path = path.Join(base.Path, "api", apiVersion, endpointPath)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func statusExpected(statusCode int, expectedStatusCodes []int) bool {
	for _, expectedStatusCode := range expectedStatusCodes {
		if statusCode == expectedStatusCode {
			return true
		}
	}
	return false
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
