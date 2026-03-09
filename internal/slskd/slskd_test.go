package slskd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchSong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v0/searches", r.URL.Path)
		require.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req struct {
			SearchText string `json:"searchText"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "Artist - Song", req.SearchText)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"search-1","searchText":"Artist - Song","state":0,"isComplete":false}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "test-api-key")
	search, err := client.searchSong(context.Background(), &domain.Song{
		Artists:  []string{"Artist", "Feat Artist"},
		Title:    "Song",
		Duration: 3 * time.Minute,
	})
	require.NoError(t, err)
	require.NotNil(t, search)
	assert.Equal(t, "search-1", search.ID)
	assert.Equal(t, "Artist - Song", search.SearchText)
	assert.False(t, search.IsComplete)
}

func TestGetSearchStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v0/searches/abc-123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc-123","searchText":"Artist - Song","state":4,"isComplete":true,"responseCount":12,"fileCount":45}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "")
	search, err := client.getSearchStatus(context.Background(), "abc-123")
	require.NoError(t, err)
	require.NotNil(t, search)
	assert.Equal(t, "abc-123", search.ID)
	assert.True(t, search.IsComplete)
	assert.Equal(t, 12, search.ResponseCount)
	assert.Equal(t, 45, search.FileCount)
}

func TestGetSearchResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v0/searches/search-1/responses", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"username":"peer-1",
			"fileCount":1,
			"lockedFileCount":0,
			"hasFreeUploadSlot":true,
			"queueLength":1,
			"token":10,
			"uploadSpeed":500,
			"files":[{"filename":"Artist - Song.flac","size":12345,"code":1,"extension":"flac","isLocked":false}],
			"lockedFiles":[]
		}]`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "")
	responses, err := client.getSearchResponses(context.Background(), "search-1")
	require.NoError(t, err)
	require.Len(t, responses, 1)
	assert.Equal(t, "peer-1", responses[0].Username)
	require.Len(t, responses[0].Files, 1)
	assert.Equal(t, "Artist - Song.flac", responses[0].Files[0].Filename)
}
