package mediaplayer

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

type SubsonicProvider struct {
	Provider
	httpClient *http.Client
}

type APIResponse struct {
	SubsonicResponse SubsonicResponse `json:"subsonic-response"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SubsonicResponse struct {
	Status        string        `json:"status"`
	Version       string        `json:"version"`
	Type          string        `json:"type"`
	ServerVersion string        `json:"serverVersion"`
	OpenSubsonic  bool          `json:"openSubsonic"`
	SearchResult3 SearchResult3 `json:"searchResult3,omitempty"`
	Error         ErrorResponse `json:"error,omitempty"`
}

type SearchResult3 struct {
	Song []SongResponse `json:"song"`
}

type SongResponse struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Album         string `json:"album"`
	Artist        string `json:"artist"`
	Duration      int    `json:"duration"`
	MusicBrainzID string `json:"musicBrainzId"`
	DisplayArtist string `json:"displayArtist"`
}

func NewSubsonicProvider(uri, username, password string) *SubsonicProvider {
	return &SubsonicProvider{
		Provider: Provider{
			URI:      uri,
			Username: username,
			Password: password,
		},
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *SubsonicProvider) SongExists(s *domain.Song) (*domain.Song, error) {
	p := url.Values{}
	p.Add("query", fmt.Sprintf("%s %s", s.Artist, s.Title))
	body := APIResponse{}
	_, err := c.execRequest("search3", p, &body)
	if err != nil {
		slog.Error("Failed to search for song in Subsonic", "error", err)
		return s, err
	}
	if body.SubsonicResponse.Error.Code != 0 {
		slog.Error("Subsonic API returned an error", "code", body.SubsonicResponse.Error.Code, "message", body.SubsonicResponse.Error.Message)
		return s, fmt.Errorf("Subsonic API error: %s", body.SubsonicResponse.Error.Message)
	}
	for _, song := range body.SubsonicResponse.SearchResult3.Song {
		if song.DisplayArtist == s.Artist && song.Title == s.Title && song.Duration == int(s.Duration.Seconds()) {
			s.Exists = true
			slog.Info("Song found in Subsonic library", "artist", s.Artist, "title", s.Title)
			return s, nil
		}
	}
	slog.Info("Song not found in Subsonic library", "artist", s.Artist, "title", s.Title)
	return s, nil
}

func (c *SubsonicProvider) execRequest(method string, params url.Values, resBody any) (*http.Response, error) {
	salt := c.generateHash(16)
	params.Add("u", c.Username)
	params.Add("t", c.getMD5Password(c.Password, salt))
	params.Add("s", salt)
	params.Add("v", "1.16.1")
	params.Add("c", "spotisleek")
	params.Add("f", "json")
	url := fmt.Sprintf("%s/rest/%s?%s", c.URI, method, params.Encode())
	res, err := c.httpClient.Get(url)
	if err != nil {
		slog.Error("Failed to make request to Subsonic", "error", err)
		return res, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(resBody); err != nil {
		return res, fmt.Errorf("failed to decode response body: %w", err)
	}
	return res, nil
}

func (c *SubsonicProvider) generateHash(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func (c *SubsonicProvider) getMD5Password(password, salt string) string {
	hash := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(hash[:])
}
