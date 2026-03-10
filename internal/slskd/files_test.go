package slskd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDownloadedFilePath(t *testing.T) {
	filename := `Music\\Nexus\\Sueltas\\Artemas\\Artemas - i like the way you kiss me (southstar remix).flac`
	downloadsDir := t.TempDir()
	requiredPath := filepath.Join(downloadsDir, "Artemas", "Artemas - i like the way you kiss me (southstar remix).flac")
	client := NewSlskd("http://example.com", "", true, downloadsDir, "")
	resolvedPath, err := client.resolveDownloadedFilePath(filename)
	assert.NoError(t, err)
	assert.Equal(t, requiredPath, resolvedPath)
}

func TestMoveFileToMusic_FindsLastFolderAndFileFallback(t *testing.T) {
	downloadsDir := t.TempDir()
	filename := `Music\\Nexus\\Sueltas\\Artemas\\Artemas - i like the way you kiss me (southstar remix).flac`
	actualPath := filepath.Join(downloadsDir, "Artemas", "Artemas - i like the way you kiss me (southstar remix).flac")

	require.NoError(t, os.MkdirAll(filepath.Dir(actualPath), 0o755))
	require.NoError(t, os.WriteFile(actualPath, []byte("test"), 0o644))

	client := NewSlskd("http://example.com", "", false, downloadsDir, "")
	err := client.MoveFileToMusic(context.Background(), &domain.Song{FilePath: filename})
	require.NoError(t, err)
}

func TestMoveFileToMusic_CreatesArtistDirectoryAndMovesFile(t *testing.T) {
	downloadsDir := t.TempDir()
	musicDir := t.TempDir()
	filename := `music\\1VA\\Packet Tracer's DnB Picks\\115 - Feint - Snake Eyes (feat. CoMa).flac`
	downloadedPath := filepath.Join(downloadsDir, "Packet Tracer's DnB Picks", "115 - Feint - Snake Eyes (feat. CoMa).flac")

	require.NoError(t, os.MkdirAll(filepath.Dir(downloadedPath), 0o755))
	require.NoError(t, os.WriteFile(downloadedPath, []byte("test"), 0o644))

	song := &domain.Song{
		Artists:  []string{"Feint", "CoMa"},
		Title:    "Snake Eyes",
		FilePath: filename,
	}
	client := NewSlskd("http://example.com", "", true, downloadsDir, musicDir)

	require.NoError(t, client.MoveFileToMusic(context.Background(), song))

	expectedPath := filepath.Join(musicDir, "Feint", "Feint, CoMa - Snake Eyes.flac")
	_, err := os.Stat(expectedPath)
	require.NoError(t, err)
}
