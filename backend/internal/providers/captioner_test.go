package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGeminiCaptionerRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("dummy"))
			return
		}
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
  "candidates": [
    {
      "content": {
        "parts": [
          {"text": "{\"caption\":\"海辺の夕焼け\",\"people_count\":1,\"labels\":[\"beach\"]}"}
        ]
      }
    }
  ]
}`))
	}))
	defer server.Close()

	captioner := &GeminiCaptioner{
		Endpoint:   server.URL,
		APIKey:     "",
		Client:     server.Client(),
		RetryMax:   1,
		RetryDelay: 10 * time.Millisecond,
	}

	result, err := captioner.Caption(context.Background(), CaptionInput{ImageURL: server.URL + "/image"})
	if err != nil {
		t.Fatalf("期待: エラーなし, 実際: %v", err)
	}
	if result.Caption != "海辺の夕焼け" {
		t.Fatalf("期待: キャプション一致, 実際: %s", result.Caption)
	}
	if result.PeopleCount != 1 {
		t.Fatalf("期待: people_count=1, 実際: %d", result.PeopleCount)
	}
	if callCount != 2 {
		t.Fatalf("期待: 2回呼び出し, 実際: %d", callCount)
	}
}

func TestGeminiCaptionerParseCodeBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("dummy"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		payload := "{\n" +
			"  \"candidates\": [\n" +
			"    {\n" +
			"      \"content\": {\n" +
			"        \"parts\": [\n" +
			"          {\"text\": \"```json\\n{\\\"caption\\\":\\\"赤い提灯の居酒屋\\\",\\\"people_count\\\":2,\\\"labels\\\":[\\\"居酒屋\\\",\\\"提灯\\\"]}\\n```\"}\n" +
			"        ]\n" +
			"      }\n" +
			"    }\n" +
			"  ]\n" +
			"}"
		w.Write([]byte(payload))
	}))
	defer server.Close()

	captioner := &GeminiCaptioner{
		Endpoint:   server.URL,
		APIKey:     "",
		Client:     server.Client(),
		RetryMax:   0,
		RetryDelay: 10 * time.Millisecond,
	}

	result, err := captioner.Caption(context.Background(), CaptionInput{ImageURL: server.URL + "/image"})
	if err != nil {
		t.Fatalf("期待: エラーなし, 実際: %v", err)
	}
	if result.Caption != "赤い提灯の居酒屋" {
		t.Fatalf("期待: キャプション一致, 実際: %s", result.Caption)
	}
	if result.PeopleCount != 2 {
		t.Fatalf("期待: people_count=2, 実際: %d", result.PeopleCount)
	}
	if len(result.Labels) != 2 {
		t.Fatalf("期待: labels 数=2, 実際: %d", len(result.Labels))
	}
}
