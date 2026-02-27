package spotify

import (
	"fmt"
	"log/slog"
	"net/http"
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

const spotifyAuthorizeEndpoint = "https://accounts.spotify.com/authorize"

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func NewSpotifyProvider(authServer *SpotifyAuthServer) *SpotifyProvider {
	return &SpotifyProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		authServer: authServer,
	}
}

func (p *SpotifyProvider) GetPlaylist(id string) ([]*domain.Song, error) {
	token, err := p.getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get Spotify access token: %w", err)
	}
	slog.Info("Obtained Spotify access token", "token", token)
	return nil, fmt.Errorf("GetPlaylist not implemented yet")
}

func (p *SpotifyProvider) getAccessToken() (string, error) {
	return "", nil
	// p.mu.Lock()
	// defer p.mu.Unlock()
	//
	// const refreshSkew = 30 * time.Second
	// if p.cachedToken != "" && time.Now().Before(p.tokenExpireAt.Add(-refreshSkew)) {
	// 	return p.cachedToken, nil
	// }
	//
	// uri := "https://accounts.spotify.com/api/token"
	// headers := http.Header{}
	// headers.Add("Authorization", p.getBasicAuthHeader())
	// headers.Add("Content-Type", "application/x-www-form-urlencoded")
	// form := url.Values{}
	// form.Add("grant_type", "client_credentials")
	// slog.Debug("Requesting Spotify access token", "uri", uri, "headers", headers, "form", form)
	// httpReq, err := http.NewRequest("POST", uri, strings.NewReader(form.Encode()))
	// if err != nil {
	// 	slog.Error("Failed to create HTTP request for Spotify access token", "error", err)
	// 	return "", err
	// }
	// httpReq.Header = headers
	// res, err := p.httpClient.Do(httpReq)
	// if err != nil {
	// 	slog.Error("Failed to execute HTTP request for Spotify access token", "error", err)
	// 	return "", err
	// }
	// defer res.Body.Close()
	//
	// if res.StatusCode != http.StatusOK {
	// 	slog.Error("Received non-OK response from Spotify access token endpoint", "status", res.StatusCode)
	// 	return "", fmt.Errorf("received non-OK response from Spotify access token endpoint: %d", res.StatusCode)
	// }
	// slog.Debug("Received response from Spotify access token endpoint", "status", res.StatusCode)
	// responseBody := &accessTokenResponse{}
	// decoder := json.NewDecoder(res.Body)
	// if err := decoder.Decode(responseBody); err != nil {
	// 	return "", fmt.Errorf("failed to decode Spotify access token response: %w", err)
	// }
	// slog.Info("Decoded Spotify access token response", "access_token", responseBody.AccessToken, "expires_in", responseBody.ExpiresIn, "token_type", responseBody.TokenType)
	//
	// p.cachedToken = responseBody.AccessToken
	// p.tokenExpireAt = time.Now().Add(time.Duration(responseBody.ExpiresIn) * time.Second)
	// return p.cachedToken, nil
}
