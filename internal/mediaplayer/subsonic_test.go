package mediaplayer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMD5Password(t *testing.T) {
	c := &SubsonicPlayer{}

	hash := c.getMD5Password("password", "salt")

	assert.Equal(t, "b305cadbb3bce54f3aa59c64fec00dea", hash)
}

func TestGenerateHash_LengthAndCharset(t *testing.T) {
	c := &SubsonicPlayer{}

	hash := c.generateHash(24)

	assert.Len(t, hash, 24)
	assert.Regexp(t, regexp.MustCompile(`^[a-zA-Z]+$`), hash)
}

func TestExecRequest_Success_AddsSubsonicParamsAndDecodesBody(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		assert.Equal(t, "/rest/search3", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"subsonic-response":{"status":"ok","searchResult3":{"song":[{"title":"Song A"}]}}}`)
	}))
	t.Cleanup(server.Close)

	c := NewSubsonicProvider(server.URL, "alice", "secret")
	c.httpClient = server.Client()

	params := url.Values{}
	params.Add("query", "artist title")
	var body APIResponse

	res, err := c.execRequest("search3", params, &body)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "alice", gotQuery.Get("u"))
	assert.Equal(t, "1.16.1", gotQuery.Get("v"))
	assert.Equal(t, "spotisleek", gotQuery.Get("c"))
	assert.Equal(t, "json", gotQuery.Get("f"))
	assert.NotEmpty(t, gotQuery.Get("s"))
	assert.Equal(t, c.getMD5Password("secret", gotQuery.Get("s")), gotQuery.Get("t"))
	assert.Equal(t, "Song A", body.SubsonicResponse.SearchResult3.Song[0].Title)
}

func TestExecRequest_Non200_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	c := NewSubsonicProvider(server.URL, "alice", "secret")
	c.httpClient = server.Client()

	var body APIResponse
	_, err := c.execRequest("search3", url.Values{}, &body)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 502")
}

func TestSongExists_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"subsonic-response":{"status":"ok","searchResult3":{"song":[{"title":"Bohemian Rhapsody","displayArtist":"Queen","duration":354}]}}}`)
	}))
	t.Cleanup(server.Close)

	c := NewSubsonicProvider(server.URL, "alice", "secret")
	c.httpClient = server.Client()

	song := &domain.Song{Artists: []string{"Queen"}, Title: "Bohemian Rhapsody", Duration: 354 * time.Second}
	got, err := c.SongExists(song)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Exists)
}

func TestSongExists_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"subsonic-response":{"status":"ok","searchResult3":{"song":[{"title":"Another Song","displayArtist":"Queen","duration":354}]}}}`)
	}))
	t.Cleanup(server.Close)

	c := NewSubsonicProvider(server.URL, "alice", "secret")
	c.httpClient = server.Client()

	song := &domain.Song{Artists: []string{"Queen"}, Title: "Bohemian Rhapsody", Duration: 354 * time.Second}
	got, err := c.SongExists(song)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Exists)
}

func TestSongExists_SubsonicError_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"subsonic-response":{"status":"failed","error":{"code":40,"message":"Wrong username or password"}}}`)
	}))
	t.Cleanup(server.Close)

	c := NewSubsonicProvider(server.URL, "alice", "secret")
	c.httpClient = server.Client()

	song := &domain.Song{Artists: []string{"Queen"}, Title: "Bohemian Rhapsody", Duration: 354 * time.Second}
	got, err := c.SongExists(song)

	require.Error(t, err)
	require.NotNil(t, got)
	assert.Contains(t, err.Error(), "Subsonic API error: Wrong username or password")
}

func TestTitlesEqual_TwoPass_StripsFtAndTrailingTags(t *testing.T) {
	assert.True(t, titlesEqual(
		"One More Time",
		"One More Time (feat. Romanthony) (Remastered 2001)",
	))
}

func TestCompareSongs_DurationTooFar_DoesNotMatch(t *testing.T) {
	c := &SubsonicPlayer{}
	providerSong := &domain.Song{
		Artists:  []string{"Daft Punk"},
		Title:    "One More Time",
		Duration: 320 * time.Second,
	}
	subsonicSong := &SongResponse{
		Title:    "One More Time (ft. Romanthony) (Remastered 2001)",
		Artists:  []ArtistsResponse{{Name: "Daft Punk"}},
		Duration: 333,
	}

	assert.False(t, c.compareSongs(providerSong, subsonicSong))
}
