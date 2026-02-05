package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"imagefinder/internal/retry"
)

type Embedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
}

type MockEmbedder struct {
	Dim int
}

func (m *MockEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	dim := m.Dim
	if dim <= 0 {
		dim = 1536
	}
	hash := sha256.Sum256([]byte(text))
	values := make([]float32, dim)
	for i := 0; i < dim; i++ {
		idx := (i * 4) % (len(hash) - 3)
		chunk := binary.BigEndian.Uint32(hash[idx : idx+4])
		values[i] = float32(chunk%1000) / 1000.0
	}
	return values, nil
}

type OpenAIEmbedder struct {
	Endpoint   string
	APIKey     string
	Model      string
	Client     *http.Client
	RetryMax   int
	RetryDelay time.Duration
}

func (o *OpenAIEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{
		"model": o.Model,
		"input": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	err = retry.Do(ctx, o.RetryMax, o.RetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
		resp, err := o.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			delay := retryAfterDuration(resp.Header.Get("Retry-After"))
			return retry.MarkRetryable(retry.RetryAfterError{Err: fmt.Errorf("OpenAI API エラー: %s", resp.Status), Delay: delay})
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("OpenAI API エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, fmt.Errorf("Embedding 応答が空です")
	}
	return response.Data[0].Embedding, nil
}
