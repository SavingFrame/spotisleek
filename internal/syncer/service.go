package syncer

import (
	"log/slog"

	"github.com/SavingFrame/spotisleep/internal/mediaplayer"
	playlistprovider "github.com/SavingFrame/spotisleep/internal/playlist_provider"
)

type Service struct {
	provider playlistprovider.PlaylistProvider
	player   mediaplayer.MusicPlayer
}

func NewService(provider playlistprovider.PlaylistProvider, player mediaplayer.MusicPlayer) *Service {
	return &Service{
		provider: provider,
		player:   player,
	}
}

func (s *Service) RunOnce(dry bool) error {
	songs, err := s.provider.GetPlaylist("me")
	if err != nil {
		return err
	}
	for _, song := range songs {
		_, err := s.player.SongExists(song)
		if err != nil {
			slog.Error("Error checking if song exists in media player", "error", err, "song", song)
			continue
		}

	}
	// Process the songs as needed
	return nil
}
