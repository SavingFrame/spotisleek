package slskd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

const apiVersion = "v0"

type Slskd struct {
	httpClient *http.Client
	uri        string
	apiKey     string
}

type search struct {
	EndedAt         *time.Time `json:"endedAt"`
	FileCount       int        `json:"fileCount"`
	ID              string     `json:"id"`
	IsComplete      bool       `json:"isComplete"`
	LockedFileCount int        `json:"lockedFileCount"`
	ResponseCount   int        `json:"responseCount"`
	SearchText      string     `json:"searchText"`
	StartedAt       time.Time  `json:"startedAt"`
	State           int        `json:"state"`
	Token           int        `json:"token"`
}

type searchResponse struct {
	FileCount         int          `json:"fileCount"`
	Files             []searchFile `json:"files"`
	HasFreeUploadSlot bool         `json:"hasFreeUploadSlot"`
	LockedFileCount   int          `json:"lockedFileCount"`
	LockedFiles       []searchFile `json:"lockedFiles"`
	QueueLength       int64        `json:"queueLength"`
	Token             int          `json:"token"`
	UploadSpeed       int          `json:"uploadSpeed"`
	Username          string       `json:"username"`
}

type searchFile struct {
	BitDepth          *int   `json:"bitDepth"`
	BitRate           *int   `json:"bitRate"`
	Code              int    `json:"code"`
	Extension         string `json:"extension"`
	Filename          string `json:"filename"`
	IsVariableBitRate *bool  `json:"isVariableBitRate"`
	Length            *int   `json:"length"`
	SampleRate        *int   `json:"sampleRate"`
	Size              int64  `json:"size"`
	IsLocked          bool   `json:"isLocked"`
}

func NewSlskd(uri, apiKey string) *Slskd {
	return &Slskd{
		uri:        strings.TrimRight(uri, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Slskd) DownloadSong(ctx context.Context, track *domain.Song) error {
	searchResult, err := s.searchSong(ctx, track)
	if err != nil {
		return err
	}
	// add if to check isComplete and responseCount > 0, otherwise we might end up in a loop if the search fails for some reason
	retryCount := 0
	for !searchResult.IsComplete {
		searchResult, err = s.getSearchStatus(ctx, searchResult.ID)
		if err != nil {
			return err
		}
		retryCount++
		if retryCount > 10 {
			return fmt.Errorf("search did not complete after %d retries", retryCount)
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (s *Slskd) findSongInResponses(responses []*searchResponse) (*searchResponse, error) {
	for _, response := range responses {
	}
}

func (s *Slskd) searchSong(ctx context.Context, track *domain.Song) (*search, error) {
	if track == nil {
		return nil, fmt.Errorf("song is nil")
	}
	if len(track.Artists) == 0 || strings.TrimSpace(track.Artists[0]) == "" {
		return nil, fmt.Errorf("song has no artists")
	}
	if strings.TrimSpace(track.Title) == "" {
		return nil, fmt.Errorf("song title is empty")
	}

	body := map[string]string{
		"searchText": fmt.Sprintf("%s - %s", track.Artists[0], track.Title),
	}

	var result search
	err := s.doJSONRequest(ctx, http.MethodPost, "/searches", nil, body, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to start SLSKD search", "error", err, "searchText", body["searchText"])
		return nil, err
	}

	return &result, nil
}

func (s *Slskd) getSearchStatus(ctx context.Context, id string) (*search, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("search id is empty")
	}

	var result search
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/searches", id), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to fetch SLSKD search status", "error", err, "searchID", id)
		return nil, err
	}

	return &result, nil
}

func (s *Slskd) getSearchResponses(ctx context.Context, id string) ([]searchResponse, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("search id is empty")
	}

	var result []searchResponse
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/searches", id, "responses"), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to fetch SLSKD search responses", "error", err, "searchID", id)
		return nil, err
	}

	return result, nil
}

func (s *Slskd) doJSONRequest(ctx context.Context, method, endpointPath string, query url.Values, requestBody any, responseBody any, expectedStatusCodes ...int) error {
	requestURL, err := s.buildURL(endpointPath, query)
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	if requestBody != nil {
		body, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if !statusExpected(res.StatusCode, expectedStatusCodes) {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("unexpected status code %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if responseBody == nil {
		return nil
	}

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if len(payload) == 0 {
		return nil
	}

	if err := json.Unmarshal(payload, responseBody); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}

	return nil
}

func (s *Slskd) buildURL(endpointPath string, query url.Values) (string, error) {
	base, err := url.Parse(s.uri)
	if err != nil {
		return "", fmt.Errorf("parse slskd uri: %w", err)
	}
	base.Path = path.Join(base.Path, "api", apiVersion, endpointPath)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func statusExpected(statusCode int, expectedStatusCodes []int) bool {
	for _, expectedStatusCode := range expectedStatusCodes {
		if statusCode == expectedStatusCode {
			return true
		}
	}
	return false
}
