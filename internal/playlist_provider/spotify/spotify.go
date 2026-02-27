package spotify

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

type SpotifyProvider struct {
	httpClient *http.Client
	authServer *SpotifyAuthServer

	mu            sync.Mutex
	cachedToken   string
	tokenExpireAt time.Time
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// spotifySavedTracksResponse models Spotify paginated saved tracks response.
// Keep only fields needed to map/compare against domain.Song.
type spotifySavedTracksResponse struct {
	Href     string                 `json:"href"`
	Limit    int                    `json:"limit"`
	Next     string                 `json:"next"`
	Offset   int                    `json:"offset"`
	Previous string                 `json:"previous"`
	Total    int                    `json:"total"`
	Items    []spotifySavedTrackRow `json:"items"`
}

type spotifySavedTrackRow struct {
	AddedAt string       `json:"added_at"`
	Track   spotifyTrack `json:"track"`
}

type spotifyTrack struct {
	Name       string          `json:"name"`
	DurationMs int             `json:"duration_ms"`
	Artists    []spotifyArtist `json:"artists"`
}

type spotifyArtist struct {
	Name string `json:"name"`
}

func NewSpotifyProvider(authServer *SpotifyAuthServer) *SpotifyProvider {
	return &SpotifyProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		authServer: authServer,
	}
}

func (p *SpotifyProvider) GetPlaylist(id string) ([]*domain.Song, error) {
	token, err := p.authServer.GetBearerToken()
	if err != nil {
		slog.Error("Failed to get Spotify access token from auth server", "error", err)
		os.Exit(1)
	}
	slog.Info("Obtained Spotify access token", "token", token)
	if id == "me" {
		return p.getFavouriteSongs()
	}
	return nil, fmt.Errorf("GetPlaylist not implemented yet")
}

func (p *SpotifyProvider) getFavouriteSongs() ([]*domain.Song, error) {
	limit := 50
	offset := 0
	parrams := url.Values{}
	var allSongs []*domain.Song
	url := "https://api.spotify.com/v1/me/tracks?"
	for {
		parrams.Set("limit", fmt.Sprintf("%d", limit))
		parrams.Set("offset", fmt.Sprintf("%d", offset))
		urlWithParams := url + parrams.Encode()
		resBody := spotifySavedTracksResponse{}
		if err := p.execGetRequest(urlWithParams, &resBody); err != nil {
			return nil, fmt.Errorf("failed to get Spotify saved tracks: %w", err)
		}

		// Process the response and add songs to allSongs
		for _, item := range resBody.Items {
			song := &domain.Song{
				Title: item.Track.Name,
				// Duration: item.Track.DurationMs,
				Duration: time.Duration(item.Track.DurationMs) * time.Millisecond,
				Artists:  make([]string, len(item.Track.Artists)),
			}
			for i, artist := range item.Track.Artists {
				song.Artists[i] = artist.Name
			}
			allSongs = append(allSongs, song)
		}

		if resBody.Next == "" {
			break
		}
		offset += limit
	}
	return allSongs, nil
}

func (p *SpotifyProvider) execGetRequest(uri string, resBody any) error {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request for Spotify API: %w", err)
	}
	token, err := p.authServer.GetBearerToken()
	if err != nil {
		return fmt.Errorf("failed to get Spotify access token from auth server: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := p.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed to execute GET request to Spotify API", "error", err, "uri", uri)
		return fmt.Errorf("failed to execute GET request to Spotify API: %w", err)
	}

	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		decoder := json.NewDecoder(res.Body)
		if err := decoder.Decode(&resBody); err != nil {
			return fmt.Errorf("failed to decode Spotify saved tracks response: %w", err)
		}
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("Spotify API returned 401 Unauthorized - access token may have expired")
	case http.StatusTooManyRequests:
		return fmt.Errorf("Spotify API rate limit exceeded (429 Too Many Requests)")
	default:
		return fmt.Errorf("Spotify API returned unexpected status code: %d", res.StatusCode)
	}
}
