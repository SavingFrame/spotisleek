package syncer

import (
	"context"
	"log/slog"

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
	slog.Info("Starting sync process", "dry_run", dry)
	providerSongs, err := s.provider.GetPlaylist("me")
	if err != nil {
		return err
	}
	for _, providerSong := range providerSongs[0:1] {
		playerSong, err := s.player.SongExists(providerSong)
		if err != nil {
			slog.Error("Error checking if song exists in media player", "error", err, "song", providerSong)
			continue
		}
		if !playerSong.Exists && !dry {
			err := s.slskd.DownloadSong(context.Background(), playerSong)
			if err != nil {
				slog.Error("Error downloading song from slskd", "error", err, "song", playerSong)
				continue
			}
			err = s.slskd.MoveFileToMusic(context.Background(), playerSong)
			if err != nil {
				slog.Error("Error moving downloaded song to music directory", "error", err, "song", playerSong)
			}
			// TODO: DELETE, IT JUST FOR ONE SONG
			break
		}

	}
	// Process the songs as needed
	return nil
}
