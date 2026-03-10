package syncer

import (
	"context"
	"log/slog"

	"github.com/SavingFrame/spotisleep/internal/logutil"
	"github.com/SavingFrame/spotisleep/internal/mediaplayer"
	playlistprovider "github.com/SavingFrame/spotisleep/internal/playlist_provider"
	"github.com/SavingFrame/spotisleep/internal/slskd"
)

type Service struct {
	provider playlistprovider.PlaylistProvider
	player   mediaplayer.MusicPlayer
	slskd    *slskd.Slskd
}

func NewService(provider playlistprovider.PlaylistProvider, player mediaplayer.MusicPlayer, slskd *slskd.Slskd) *Service {
	return &Service{
		provider: provider,
		player:   player,
		slskd:    slskd,
	}
}

func (s *Service) RunOnce(dry bool) error {
	slog.Info("Starting sync", "dry_run", dry)

	providerSongs, err := s.provider.GetPlaylist("me")
	if err != nil {
		return err
	}
	slog.Info("Loaded source playlist", "songs", len(providerSongs))

	checked := 0
	alreadyInLibrary := 0
	downloaded := 0
	failed := 0

	for _, providerSong := range providerSongs {
		checked++
		slog.Info("Checking song", "song", logutil.Song(providerSong), "index", checked)

		playerSong, err := s.player.SongExists(providerSong)
		if err != nil {
			failed++
			slog.Error("Library lookup failed", "error", err, "song", logutil.Song(providerSong))
			continue
		}

		if playerSong.Exists {
			alreadyInLibrary++
			slog.Info("Song already exists in library", "song", logutil.Song(playerSong))
			continue
		}

		if dry {
			slog.Info("Song missing from library; skipping download because dry-run is enabled", "song", logutil.Song(playerSong))
			continue
		}

		slog.Info("Song missing from library; starting download", "song", logutil.Song(playerSong))
		if err := s.slskd.DownloadSong(context.Background(), playerSong); err != nil {
			failed++
			slog.Error("Download failed", "error", err, "song", logutil.Song(playerSong))
			continue
		}

		if err := s.slskd.MoveFileToMusic(context.Background(), playerSong); err != nil {
			failed++
			slog.Error("Move failed", "error", err, "song", logutil.Song(playerSong))
			continue
		}

		downloaded++
		slog.Info("Song downloaded successfully", "song", logutil.Song(playerSong))

	}

	slog.Info("Sync finished", "checked", checked, "already_in_library", alreadyInLibrary, "downloaded", downloaded, "failed", failed, "dry_run", dry)
	return nil
}
