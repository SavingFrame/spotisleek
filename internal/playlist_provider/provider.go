package playlistprovider

import "github.com/SavingFrame/spotisleep/internal/domain"

type PlaylistProvider interface {
	GetPlaylist(id string) ([]*domain.Song, error)
}
