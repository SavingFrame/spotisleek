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
		require.Equal(t, "test-api-key", r.Header.Get("X-API-KEY"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req struct {
			SearchText string `json:"searchText"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "Artist - Song", req.SearchText)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"search-1","searchText":"Artist - Song","state":"InProgress","isComplete":false}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "test-api-key", false, "", "")
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
		_, _ = w.Write([]byte(`{"id":"abc-123","searchText":"Artist - Song","state":"Completed","isComplete":true,"responseCount":12,"fileCount":45}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "", false, "", "")
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

	client := NewSlskd(server.URL, "", false, "", "")
	responses, err := client.getSearchResponses(context.Background(), "search-1")
	require.NoError(t, err)
	require.Len(t, responses, 1)
	assert.Equal(t, "peer-1", responses[0].Username)
	require.Len(t, responses[0].Files, 1)
	assert.Equal(t, "Artist - Song.flac", responses[0].Files[0].Filename)
}

func TestEnqueueDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v0/transfers/downloads/peer-1", r.URL.Path)
		require.Equal(t, "test-api-key", r.Header.Get("X-API-KEY"))

		var req []QueueDownloadRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req, 1)
		assert.Equal(t, "Artist - Song.flac", req[0].Filename)
		assert.EqualValues(t, 12345, req[0].Size)

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enqueued":[{"id":"dl-1","username":"peer-1","filename":"Artist - Song.flac","state":"Queued"}],"failed":[]}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "test-api-key", false, "", "")
	transfer, err := client.enqueueDownload(context.Background(), &searchFile{
		Filename: "Artist - Song.flac",
		Size:     12345,
		username: "peer-1",
	})
	require.NoError(t, err)
	require.NotNil(t, transfer)
	assert.Equal(t, "dl-1", transfer.ID)
	assert.Equal(t, "peer-1", transfer.Username)
}

func TestGetDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v0/transfers/downloads/peer-1/dl-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"dl-1","username":"peer-1","filename":"Artist - Song.flac","state":"Completed, Succeeded","bytesTransferred":12345}`))
	}))
	defer server.Close()

	client := NewSlskd(server.URL, "", false, "", "")
	transfer, err := client.getDownload(context.Background(), "peer-1", "dl-1")
	require.NoError(t, err)
	require.NotNil(t, transfer)
	assert.Equal(t, "dl-1", transfer.ID)
	assert.Equal(t, "Completed, Succeeded", transfer.State)
	assert.EqualValues(t, 12345, transfer.BytesTransferred)
}
