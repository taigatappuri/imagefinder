package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIEmbedderRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	embedder := &OpenAIEmbedder{
		Endpoint:   server.URL,
		APIKey:     "test",
		Model:      "text-embedding-3-small",
		Client:     server.Client(),
		RetryMax:   1,
		RetryDelay: 10 * time.Millisecond,
	}

	values, err := embedder.EmbedText(context.Background(), "hello")
	if err != nil {
		t.Fatalf("期待: エラーなし, 実際: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("期待: 2次元, 実際: %d", len(values))
	}
	if callCount != 2 {
		t.Fatalf("期待: 2回呼び出し, 実際: %d", callCount)
	}
}
