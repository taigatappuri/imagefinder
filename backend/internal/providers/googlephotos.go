package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"imagefinder/internal/retry"
)

type MediaItem struct {
	ID          string
	BaseURL     string
	MimeType    string
	CreatedTime *time.Time
	Location    *string
}

type GooglePhotosClient interface {
	ListMediaItems(ctx context.Context, accessToken string, pageSize int, pageToken string) ([]MediaItem, string, error)
}

type HTTPGooglePhotosClient struct {
	BaseURL    string
	Client     *http.Client
	RetryMax   int
	RetryDelay time.Duration
}

func (c *HTTPGooglePhotosClient) ListMediaItems(ctx context.Context, accessToken string, pageSize int, pageToken string) ([]MediaItem, string, error) {
	endpoint, err := url.Parse(c.BaseURL + "/mediaItems")
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	if pageSize > 0 {
		query.Set("pageSize", fmt.Sprintf("%d", pageSize))
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	endpoint.RawQuery = query.Encode()

	var payload struct {
		MediaItems []struct {
			ID            string `json:"id"`
			BaseURL       string `json:"baseUrl"`
			MimeType      string `json:"mimeType"`
			MediaMetadata struct {
				CreationTime string `json:"creationTime"`
				Location     struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"location"`
			} `json:"mediaMetadata"`
		} `json:"mediaItems"`
		NextPageToken string `json:"nextPageToken"`
	}

	err = retry.Do(ctx, c.RetryMax, c.RetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := c.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("Google Photos API エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("Google Photos API エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	items := make([]MediaItem, 0, len(payload.MediaItems))
	for _, item := range payload.MediaItems {
		if item.MimeType == "" || item.ID == "" {
			continue
		}
		var createdAt *time.Time
		if item.MediaMetadata.CreationTime != "" {
			if parsed, err := time.Parse(time.RFC3339, item.MediaMetadata.CreationTime); err == nil {
				createdAt = &parsed
			}
		}
		var location *string
		if item.MediaMetadata.Location.Latitude != 0 || item.MediaMetadata.Location.Longitude != 0 {
			loc := fmt.Sprintf("%.6f,%.6f", item.MediaMetadata.Location.Latitude, item.MediaMetadata.Location.Longitude)
			location = &loc
		}
		items = append(items, MediaItem{
			ID:          item.ID,
			BaseURL:     item.BaseURL,
			MimeType:    item.MimeType,
			CreatedTime: createdAt,
			Location:    location,
		})
	}
	return items, payload.NextPageToken, nil
}

type MockGooglePhotosClient struct {
	Items []MediaItem
}

func (m *MockGooglePhotosClient) ListMediaItems(ctx context.Context, accessToken string, pageSize int, pageToken string) ([]MediaItem, string, error) {
	if len(m.Items) == 0 {
		return []MediaItem{}, "", nil
	}
	return m.Items, "", nil
}
