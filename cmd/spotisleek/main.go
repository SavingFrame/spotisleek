package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/SavingFrame/spotisleep/internal/config"
	"github.com/SavingFrame/spotisleep/internal/playlist_provider/spotify"
)

// var action = flag.String("action", "server", "Action to perform: server or auth")
var port = flag.Int("port", 8989, "Port to run the server on")

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		slog.Error("Error loading config", "error", err)
		return
	}
	flag.Parse()
	action := getActionArgument()
	providerAuthServer := spotify.NewSpotifyAuthServer(*port, config.SPOTIFY_CLIENT_ID, config.SPOTIFY_CLIENT_SECRET)
	// provider := spotify.NewSpotifyProvider(providerAuthServer)
	switch action {
	case "auth":
		handleAuth(providerAuthServer)
	}
	// providerAuthServer.Start()
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
