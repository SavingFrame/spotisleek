package slskd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/SavingFrame/spotisleep/internal/logutil"
)

func (s *Slskd) MoveFileToMusic(ctx context.Context, song *domain.Song) error {
	if !s.moveFile {
		slog.Info("Skipping file move; migration is disabled", "song", logutil.Song(song))
		return nil
	}

	absDownloadPath, err := s.resolveDownloadedFilePath(song.FilePath)
	if err != nil {
		slog.Error("Could not resolve downloaded file path", "error", err, "song", logutil.Song(song), "source", song.FilePath)
		return err
	}

	musicFilePath := s.resolveMusicFilePath(song)
	slog.Info("Moving downloaded file into library", "song", logutil.Song(song), "source", absDownloadPath, "destination", musicFilePath)
	if _, err := os.Stat(absDownloadPath); os.IsNotExist(err) {
		slog.Error("Downloaded file not found", "song", logutil.Song(song), "source", absDownloadPath)
		return fmt.Errorf("downloaded file does not exist at expected path: %s", absDownloadPath)
	}

	ensureErr := s.ensureMusicDirExists(musicFilePath)
	if ensureErr != nil {
		return fmt.Errorf("failed to ensure music directory exists: %w", ensureErr)
	}
	if err := os.Rename(absDownloadPath, musicFilePath); err != nil {
		return fmt.Errorf("failed to move downloaded file to music directory: %w", err)
	}

	song.FilePath = musicFilePath
	slog.Info("Moved downloaded file into library", "song", logutil.Song(song), "destination", musicFilePath)
	return nil
}

func (s *Slskd) resolveDownloadedFilePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("downloaded file path is empty")
	}
	p = strings.ReplaceAll(p, "\\", "/")
	filename := filepath.Base(p)
	lastDir := filepath.Base(filepath.Dir(p))
	downloadPath := path.Join(s.downloadsPath, lastDir, filename)
	if !path.IsAbs(downloadPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		downloadPath = path.Join(cwd, downloadPath)
	}
	return downloadPath, nil
}

func (s *Slskd) resolveMusicFilePath(song *domain.Song) string {
	artists := strings.Join(song.Artists, ", ")
	filename := fmt.Sprintf("%s - %s.flac", artists, song.Title)
	fp := path.Join(s.musicPath, song.Artists[0], filename)
	return fp
}

func (s *Slskd) ensureMusicDirExists(filePath string) error {
	stat, err := os.Stat(s.musicPath)
	var uid, guid int
	if err == nil {
		uid = int(stat.Sys().(*syscall.Stat_t).Uid)
		guid = int(stat.Sys().(*syscall.Stat_t).Gid)
	}
	dir := path.Dir(filePath)
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create music directory: %w", err)
	}
	if uid != 0 && guid != 0 {
		err = os.Chown(dir, uid, guid)
		if err != nil {
			return fmt.Errorf("failed to change ownership of music directory: %w", err)
		}
	}
	return nil
}
