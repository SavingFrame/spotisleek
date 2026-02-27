package main

import (
	"log/slog"
	"time"

	"github.com/SavingFrame/spotisleep/internal/config"
	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/SavingFrame/spotisleep/internal/mediaplayer"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		slog.Error("Error loading config", "error", err)
		return
	}

	slog.Info("Config loaded successfully", "config", config)

	c := mediaplayer.NewSubsonicProvider(config.NAVIDROME_URL, config.NAVIDROME_USERNAME, config.NAVIDROME_PASSWORD)
	song := &domain.Song{
		Artist:   "Red Hot Chili Peppers",
		Title:    "Can't stop",
		Duration: 268 * time.Second,
	}
	c.SongExists(song)
}
