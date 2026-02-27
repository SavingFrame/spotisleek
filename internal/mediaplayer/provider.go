package mediaplayer

import "github.com/SavingFrame/spotisleep/internal/domain"

type Provider struct {
	URI      string
	Username string
	Password string
}

type MusicProvider interface {
	SongExists(s *domain.Song) (*domain.Song, error)
}
