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
)

func (s *Slskd) MoveFileToMusic(ctx context.Context, song *domain.Song) error {
	if !s.moveFile {
		return nil
	}
	slog.Info("Moving downloaded file to music directory", "filename", song.FilePath)

	absDownloadPath, err := s.resolveDownloadedFilePath(song.FilePath)
	if err != nil {
		slog.Error("Downloaded file does not exist at expected path", "path", song.FilePath)
		return err
	}
	if _, err := os.Stat(absDownloadPath); os.IsNotExist(err) {
		slog.Error("Downloaded file does not exist at expected path", "path", absDownloadPath)
		return fmt.Errorf("downloaded file does not exist at expected path: %s", absDownloadPath)
	}

	musicFilePath := s.resolveMusicFilePath(song)
	ensureErr := s.ensureMusicDirExists(musicFilePath)
	if ensureErr != nil {
		return fmt.Errorf("failed to ensure music directory exists: %w", ensureErr)
	}
	err = os.Rename(absDownloadPath, musicFilePath)
	if err != nil {
		return fmt.Errorf("failed to move downloaded file to music directory: %w", err)
	}
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
