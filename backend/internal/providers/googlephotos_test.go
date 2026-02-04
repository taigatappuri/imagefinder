package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPGooglePhotosClientRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
  "mediaItems": [
    {
      "id": "photo-1",
      "baseUrl": "https://example.com/photo",
      "mimeType": "image/jpeg",
      "mediaMetadata": {
        "creationTime": "2026-02-03T00:00:00Z",
        "location": {"latitude": 35.0, "longitude": 139.0}
      }
    }
  ],
  "nextPageToken": ""
}`))
	}))
	defer server.Close()

	client := &HTTPGooglePhotosClient{
		BaseURL:    server.URL,
		Client:     server.Client(),
		RetryMax:   1,
		RetryDelay: 10 * time.Millisecond,
	}

	items, next, err := client.ListMediaItems(context.Background(), "token", 10, "")
	if err != nil {
		t.Fatalf("期待: エラーなし, 実際: %v", err)
	}
	if next != "" {
		t.Fatalf("期待: 次ページなし, 実際: %s", next)
	}
	if len(items) != 1 {
		t.Fatalf("期待: 1件, 実際: %d件", len(items))
	}
	if items[0].ID != "photo-1" {
		t.Fatalf("期待: photo-1, 実際: %s", items[0].ID)
	}
	if callCount != 2 {
		t.Fatalf("期待: 2回呼び出し, 実際: %d", callCount)
	}
}
