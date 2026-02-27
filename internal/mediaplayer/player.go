package mediaplayer

import "github.com/SavingFrame/spotisleep/internal/domain"

type Player struct {
	URI      string
	Username string
	Password string
}

type MusicPlayer interface {
	SongExists(s *domain.Song) (*domain.Song, error)
}
