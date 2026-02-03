package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
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
	Endpoint string
	APIKey   string
	Model    string
	Client   *http.Client
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI API エラー: %s", resp.Status)
	}
	var response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, fmt.Errorf("Embedding 応答が空です")
	}
	return response.Data[0].Embedding, nil
}
