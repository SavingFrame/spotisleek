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

type Transfer struct {
	AverageSpeed     float64 `json:"averageSpeed"`
	BytesRemaining   int64   `json:"bytesRemaining"`
	BytesTransferred int64   `json:"bytesTransferred"`
	Direction        string  `json:"direction"`
	EndTime          *string `json:"endTime"`
	Filename         string  `json:"filename"`
	ID               string  `json:"id"`
	PercentComplete  float64 `json:"percentComplete"`
	PlaceInQueue     *int    `json:"placeInQueue"`
	Size             int64   `json:"size"`
	StartOffset      int64   `json:"startOffset"`
	StartTime        *string `json:"startTime"`
	State            string  `json:"state"`
	Token            int     `json:"token"`
	Username         string  `json:"username"`
	Exception        *string `json:"exception"`
}

type QueueDownloadRequest struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size,omitempty"`
}

type enqueueDownloadResponse struct {
	Enqueued []Transfer `json:"enqueued"`
	Failed   []string   `json:"failed"`
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
	State           string     `json:"state"`
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
	username          string
}

func (s *Slskd) getDownload(ctx context.Context, username, id string) (*Transfer, error) {
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("username is empty")
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("download id is empty")
	}

	var result Transfer
	err := s.doJSONRequest(ctx, http.MethodGet, path.Join("/transfers/downloads", username, id), nil, nil, &result, http.StatusOK)
	if err != nil {
		slog.Error("Failed to get download info", "error", err, "username", username, "downloadID", id)
		return nil, err
	}
	return &result, nil
}

func (s *Slskd) enqueueDownload(ctx context.Context, file *searchFile) (*Transfer, error) {
	if strings.TrimSpace(file.username) == "" {
		return nil, fmt.Errorf("username is empty")
	}
	var validFiles []QueueDownloadRequest
	validFiles = append(validFiles, QueueDownloadRequest{
		Filename: file.Filename,
		Size:     file.Size,
	})

	var result enqueueDownloadResponse
	err := s.doJSONRequest(ctx, http.MethodPost, path.Join("/transfers/downloads", file.username), nil, validFiles, &result, http.StatusCreated)
	if err != nil {
		slog.Error("Failed to enqueue download", "error", err, "username", file.username)
		return nil, err
	}
	if len(result.Enqueued) == 0 {
		if len(result.Failed) > 0 {
			return nil, fmt.Errorf("enqueue download failed: %s", strings.Join(result.Failed, ", "))
		}
		return nil, fmt.Errorf("enqueue download returned no enqueued transfers")
	}
	return &result.Enqueued[0], nil
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

func (s *Slskd) getSearchResponses(ctx context.Context, id string) ([]*searchResponse, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("search id is empty")
	}

	var result []*searchResponse
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
	req.Header.Set("X-API-KEY", s.apiKey)

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
