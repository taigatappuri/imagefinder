package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"imagefinder/internal/retry"
)

type PickerPollingConfig struct {
	PollInterval string `json:"pollInterval"`
	TimeoutIn    string `json:"timeoutIn"`
}

type PickerSession struct {
	ID            string               `json:"id"`
	PickerURI     string               `json:"pickerUri"`
	MediaItemsSet bool                 `json:"mediaItemsSet"`
	PollingConfig *PickerPollingConfig `json:"pollingConfig"`
}

type PickedMediaItem struct {
	ID          string
	BaseURL     string
	MimeType    string
	Filename    string
	CreatedTime *time.Time
}

type PhotosPickerClient interface {
	CreateSession(ctx context.Context, accessToken string) (PickerSession, error)
	GetSession(ctx context.Context, accessToken, sessionID string) (PickerSession, error)
	DeleteSession(ctx context.Context, accessToken, sessionID string) error
	ListMediaItems(ctx context.Context, accessToken, sessionID string, pageSize int, pageToken string) ([]PickedMediaItem, string, error)
}

type HTTPPhotosPickerClient struct {
	BaseURL    string
	Client     *http.Client
	RetryMax   int
	RetryDelay time.Duration
}

func (c *HTTPPhotosPickerClient) CreateSession(ctx context.Context, accessToken string) (PickerSession, error) {
	endpoint := c.BaseURL + "/sessions"
	payload := map[string]any{}
	body, err := json.Marshal(payload)
	if err != nil {
		return PickerSession{}, err
	}
	var session PickerSession
	if err := c.doJSON(ctx, http.MethodPost, endpoint, accessToken, body, &session); err != nil {
		return PickerSession{}, err
	}
	return session, nil
}

func (c *HTTPPhotosPickerClient) GetSession(ctx context.Context, accessToken, sessionID string) (PickerSession, error) {
	endpoint := c.BaseURL + "/sessions/" + url.PathEscape(sessionID)
	var session PickerSession
	if err := c.doJSON(ctx, http.MethodGet, endpoint, accessToken, nil, &session); err != nil {
		return PickerSession{}, err
	}
	return session, nil
}

func (c *HTTPPhotosPickerClient) DeleteSession(ctx context.Context, accessToken, sessionID string) error {
	endpoint := c.BaseURL + "/sessions/" + url.PathEscape(sessionID)
	return c.doJSON(ctx, http.MethodDelete, endpoint, accessToken, nil, nil)
}

func (c *HTTPPhotosPickerClient) ListMediaItems(ctx context.Context, accessToken, sessionID string, pageSize int, pageToken string) ([]PickedMediaItem, string, error) {
	endpoint, err := url.Parse(c.BaseURL + "/mediaItems")
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("sessionId", sessionID)
	if pageSize > 0 {
		query.Set("pageSize", fmt.Sprintf("%d", pageSize))
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	endpoint.RawQuery = query.Encode()

	var payload struct {
		MediaItems []struct {
			ID        string `json:"id"`
			MediaFile struct {
				BaseURL           string `json:"baseUrl"`
				MimeType          string `json:"mimeType"`
				Filename          string `json:"filename"`
				MediaFileMetadata struct {
					CreationTime string `json:"creationTime"`
				} `json:"mediaFileMetadata"`
			} `json:"mediaFile"`
		} `json:"mediaItems"`
		NextPageToken string `json:"nextPageToken"`
	}

	if err := c.doJSON(ctx, http.MethodGet, endpoint.String(), accessToken, nil, &payload); err != nil {
		return nil, "", err
	}

	items := make([]PickedMediaItem, 0, len(payload.MediaItems))
	for _, item := range payload.MediaItems {
		var createdAt *time.Time
		if item.MediaFile.MediaFileMetadata.CreationTime != "" {
			if parsed, err := time.Parse(time.RFC3339, item.MediaFile.MediaFileMetadata.CreationTime); err == nil {
				createdAt = &parsed
			}
		}
		items = append(items, PickedMediaItem{
			ID:          item.ID,
			BaseURL:     item.MediaFile.BaseURL,
			MimeType:    item.MediaFile.MimeType,
			Filename:    item.MediaFile.Filename,
			CreatedTime: createdAt,
		})
	}
	return items, payload.NextPageToken, nil
}

func (c *HTTPPhotosPickerClient) doJSON(ctx context.Context, method, endpoint, accessToken string, body []byte, out any) error {
	return retry.Do(ctx, c.RetryMax, c.RetryDelay, func() error {
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		} else {
			req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
		}
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("Picker API エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("Picker API エラー: %s", resp.Status)
		}
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	})
}
