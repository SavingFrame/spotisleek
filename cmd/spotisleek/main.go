package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/SavingFrame/spotisleep/internal/config"
	"github.com/SavingFrame/spotisleep/internal/mediaplayer"
	"github.com/SavingFrame/spotisleep/internal/playlist_provider/spotify"
	"github.com/SavingFrame/spotisleep/internal/slskd"
	"github.com/SavingFrame/spotisleep/internal/syncer"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		slog.Error("Error loading config", "error", err)
		return
	}
	flag.Parse()
	action := getActionArgument()
	switch action {
	case "auth":

		authFlags := flag.NewFlagSet("auth", flag.ExitOnError)
		port := authFlags.Int("port", 8989, "Port to run the auth server on")
		authFlags.Parse(os.Args[2:])
		providerAuthServer := spotify.NewSpotifyAuthServer(*port, config.SPOTIFY_CLIENT_ID, config.SPOTIFY_CLIENT_SECRET, config.SPOTIFY_REFRESH_TOKEN)
		handleAuth(providerAuthServer)
	case "server":

		serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
		dryRun := serverFlags.Bool("dry-run", false, "Run without making changes")
		serverFlags.Parse(os.Args[2:])
		if config.SPOTIFY_REFRESH_TOKEN == "" {
			slog.Error("SPOTIFY_REFRESH_TOKEN is not set. Please run the auth flow first to obtain a refresh token.")
			os.Exit(1)
		}

		providerAuthServer := spotify.NewSpotifyAuthServer(8989, config.SPOTIFY_CLIENT_ID, config.SPOTIFY_CLIENT_SECRET, config.SPOTIFY_REFRESH_TOKEN)
		provider := spotify.NewSpotifyProvider(providerAuthServer)
		mediaplayer := mediaplayer.NewSubsonicProvider(config.NAVIDROME_URL, config.NAVIDROME_USERNAME, config.NAVIDROME_PASSWORD)
		if err != nil {
			slog.Error("Error creating media player", "error", err)
			os.Exit(1)
		}
		if config.SLSKD_URL == "" || config.SLSKD_API_KEY == "" {
			slog.Error("SLSKD_URL and SLSKD_API_KEY must be set in the config to use the sync service")
			os.Exit(1)
		}
		slskd := slskd.NewSlskd(config.SLSKD_URL, config.SLSKD_API_KEY, config.MIGRATE_DOWNLOADS, config.DOWNLOADS_PATH, config.MUSIC_PATH)
		service := syncer.NewService(provider, mediaplayer, slskd)
		serverErr := service.RunOnce(*dryRun)
		if serverErr != nil {
			slog.Error("Error running sync service", "error", serverErr)
			os.Exit(1)
		}

	}
}

func handleAuth(providerAuthServer *spotify.SpotifyAuthServer) {
	err, errCh := providerAuthServer.Start()
	if err != nil {
		fmt.Printf("Could not start Spotify authorization flow: %v\n", err)
		os.Exit(1)
	}

	if serverErr := <-errCh; serverErr != nil {
		fmt.Printf("Spotify authorization failed: %v\n", serverErr)
		os.Exit(1)
	}

	refreshToken := providerAuthServer.RefreshToken()
	if refreshToken == "" {
		fmt.Println("Spotify authorization finished, but no refresh token was returned.")
		fmt.Println("Please run the auth flow again.")
		os.Exit(1)
	}

	fmt.Println("Spotify authorization completed successfully.")
	fmt.Println("Copy this refresh token and add it to your environment:")
	fmt.Printf("SPOTIFY_REFRESH_TOKEN=%s\n", refreshToken)
	os.Exit(0)
}

func getActionArgument() string {
	if flag.NArg() > 0 {
		action := flag.Arg(0)
		if action != "server" && action != "auth" {
			fmt.Printf("Invalid action: %s. Valid actions are 'server' or 'auth'.\n", action)
			os.Exit(1)
		}
		return action
	}
	return "server"
}
