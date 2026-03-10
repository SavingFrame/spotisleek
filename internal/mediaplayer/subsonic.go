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
	"regexp"
	"strings"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/SavingFrame/spotisleep/internal/logutil"
)

const (
	songDurationTolerance       = 5 * time.Second
	strictSongDurationTolerance = 15 * time.Second
)

var (
	featuringSegmentPattern   = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*\b(?:ft|feat|featuring)\b[^\)\]]*[\)\]]`)
	trailingBracketTagPattern = regexp.MustCompile(`\s*[\(\[][^\)\]]+[\)\]]\s*$`)
	textNormalizationReplacer = strings.NewReplacer(
		"’", "'",
		"‘", "'",
		"`", "'",
		"´", "'",
		"“", `"`,
		"”", `"`,
	)
)

type SubsonicPlayer struct {
	Player
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

type ArtistsResponse struct {
	Name string `json:"name"`
}

type SongResponse struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Album         string            `json:"album"`
	Artist        string            `json:"artist"`
	Artists       []ArtistsResponse `json:"artists"`
	Duration      int               `json:"duration"`
	MusicBrainzID string            `json:"musicBrainzId"`
	DisplayArtist string            `json:"displayArtist"`
}

func NewSubsonicProvider(uri, username, password string) *SubsonicPlayer {
	return &SubsonicPlayer{
		Player: Player{
			URI:      uri,
			Username: username,
			Password: password,
		},
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *SubsonicPlayer) SongExists(s *domain.Song) (*domain.Song, error) {
	if s == nil {
		return nil, fmt.Errorf("song is nil")
	}

	queries := buildSearchQueries(s)
	if len(queries) == 0 {
		slog.Warn("Skipping Subsonic lookup: empty title", "song", logutil.Song(s))
		return s, nil
	}

	slog.Info("Searching Subsonic library", "song", logutil.Song(s))
	for idx, query := range queries {
		slog.Debug("Trying Subsonic search query", "song", logutil.Song(s), "query", query, "attempt", idx+1, "total_attempts", len(queries))
		songs, err := c.searchSong(query)
		if err != nil {
			return s, err
		}
		slog.Debug("Subsonic search returned candidates", "song", logutil.Song(s), "query", query, "candidates", len(songs))
		for i := range songs {
			if c.compareSongs(s, &songs[i]) {
				s.Exists = true
				slog.Debug("Found in Subsonic library", "song", logutil.Song(s), "match", songs[i].Title)
				return s, nil
			}
		}
	}

	slog.Info("Song not found in Subsonic library", "song", logutil.Song(s))
	return s, nil
}

func buildSearchQueries(song *domain.Song) []string {
	queries := make([]string, 0, len(song.Artists)+1)
	seen := make(map[string]struct{}, len(song.Artists)+1)
	addQuery := func(query string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		normalized := strings.ToLower(query)
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		queries = append(queries, query)
	}

	title := strings.TrimSpace(song.Title)
	if title == "" {
		return queries
	}

	if len(song.Artists) > 0 {
		addQuery(fmt.Sprintf("%s %s", song.Artists[0], title))
	}
	addQuery(title)
	for _, artist := range song.Artists[1:] {
		addQuery(fmt.Sprintf("%s %s", artist, title))
	}

	return queries
}

func (c *SubsonicPlayer) searchSong(query string) ([]SongResponse, error) {
	p := url.Values{}
	p.Add("query", query)
	body := APIResponse{}
	_, err := c.execRequest("search3", p, &body)
	if err != nil {
		slog.Error("Failed to search for song in Subsonic", "error", err)
		return nil, err
	}
	if body.SubsonicResponse.Error.Code != 0 {
		slog.Error("Subsonic API returned an error", "code", body.SubsonicResponse.Error.Code, "message", body.SubsonicResponse.Error.Message)
		return nil, fmt.Errorf("Subsonic API error: %s", body.SubsonicResponse.Error.Message)
	}
	return body.SubsonicResponse.SearchResult3.Song, nil
}

func (c *SubsonicPlayer) execRequest(method string, params url.Values, resBody any) (*http.Response, error) {
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

func (c *SubsonicPlayer) generateHash(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func (c *SubsonicPlayer) getMD5Password(password, salt string) string {
	hash := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(hash[:])
}

func (c *SubsonicPlayer) compareSongs(providerSong *domain.Song, subsonicSong *SongResponse) bool {
	if providerSong == nil || subsonicSong == nil {
		return false
	}

	strictTitleMatch, looseTitleMatch := titleMatch(providerSong.Title, subsonicSong.Title)
	if !strictTitleMatch && !looseTitleMatch {
		return false
	}
	if !artistsOverlap(providerSong.Artists, subsonicSong) {
		return false
	}

	tolerance := songDurationTolerance
	if strictTitleMatch {
		tolerance = strictSongDurationTolerance
	}
	if !durationClose(providerSong.Duration, subsonicSong.Duration, tolerance) {
		return false
	}

	return true
}

func titlesEqual(providerTitle, subsonicTitle string) bool {
	strictMatch, looseMatch := titleMatch(providerTitle, subsonicTitle)
	return strictMatch || looseMatch
}

func titleMatch(providerTitle, subsonicTitle string) (strictMatch, looseMatch bool) {
	providerStrict := normalizeText(providerTitle)
	subsonicStrict := normalizeText(subsonicTitle)
	strictMatch = providerStrict != "" && providerStrict == subsonicStrict
	if strictMatch {
		return strictMatch, true
	}

	providerLoose := normalizeText(normalizeTitleLoose(providerTitle))
	subsonicLoose := normalizeText(normalizeTitleLoose(subsonicTitle))
	looseMatch = providerLoose != "" && providerLoose == subsonicLoose
	return strictMatch, looseMatch
}

func normalizeTitleLoose(title string) string {
	title = featuringSegmentPattern.ReplaceAllString(title, " ")
	for {
		updated := trailingBracketTagPattern.ReplaceAllString(title, "")
		if updated == title {
			break
		}
		title = updated
	}
	return title
}

func artistsOverlap(providerArtists []string, subsonicSong *SongResponse) bool {
	providerSet := make(map[string]struct{}, len(providerArtists))
	for _, artist := range providerArtists {
		addNormalizedArtist(providerSet, artist)
	}
	if len(providerSet) == 0 {
		return false
	}

	subsonicSet := make(map[string]struct{}, len(subsonicSong.Artists)+2)
	for _, artist := range subsonicSong.Artists {
		addNormalizedArtist(subsonicSet, artist.Name)
	}
	if len(subsonicSet) == 0 {
		// Fallback for servers that don't return searchResult3.song[].artists
		addNormalizedArtist(subsonicSet, subsonicSong.DisplayArtist)
		addNormalizedArtist(subsonicSet, subsonicSong.Artist)
	}
	if len(subsonicSet) == 0 {
		return false
	}

	for artist := range providerSet {
		if _, ok := subsonicSet[artist]; ok {
			return true
		}
	}
	return false
}

func addNormalizedArtist(set map[string]struct{}, artist string) {
	normalized := normalizeText(artist)
	if normalized == "" {
		return
	}
	set[normalized] = struct{}{}
}

func durationClose(providerDuration time.Duration, subsonicDurationSeconds int, tolerance time.Duration) bool {
	if providerDuration <= 0 || subsonicDurationSeconds <= 0 {
		return true
	}

	subsonicDuration := time.Duration(subsonicDurationSeconds) * time.Second
	diff := providerDuration - subsonicDuration
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func normalizeText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = textNormalizationReplacer.Replace(value)
	value = strings.ToLower(value)
	return strings.Join(strings.Fields(value), " ")
}
