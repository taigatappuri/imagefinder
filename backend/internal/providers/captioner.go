package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"imagefinder/internal/retry"
)

type CaptionInput struct {
	ImageURL    string
	CreatedTime *time.Time
	Location    *string
	AccessToken string
}

type CaptionResult struct {
	Caption     string   `json:"caption"`
	PeopleCount int      `json:"people_count"`
	Labels      []string `json:"labels"`
}

type Captioner interface {
	Caption(ctx context.Context, input CaptionInput) (CaptionResult, error)
}

type MockCaptioner struct{}

func (m *MockCaptioner) Caption(ctx context.Context, input CaptionInput) (CaptionResult, error) {
	pieces := []string{}
	if input.CreatedTime != nil {
		pieces = append(pieces, input.CreatedTime.Format("2006-01-02"))
	}
	if input.Location != nil {
		pieces = append(pieces, "場所:"+*input.Location)
	}
	caption := "写真の説明"
	if len(pieces) > 0 {
		caption = strings.Join(pieces, " ") + " の写真"
	}
	return CaptionResult{Caption: caption, PeopleCount: 0, Labels: []string{"mock"}}, nil
}

type GeminiCaptioner struct {
	Endpoint   string
	APIKey     string
	Client     *http.Client
	RetryMax   int
	RetryDelay time.Duration
}

func (g *GeminiCaptioner) Caption(ctx context.Context, input CaptionInput) (CaptionResult, error) {
	imageData, mimeType, err := fetchImage(ctx, g.Client, input.ImageURL, input.AccessToken)
	if err != nil {
		return CaptionResult{}, err
	}
	prompt := "特徴や書いてある文字に触れながら、この写真を日本語で詳細に説明し、人物数と主要ラベルを抽出してください。JSONで {\"caption\":string,\"people_count\":number,\"labels\":[string]} の形式で返してください。"
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": prompt},
					{"inline_data": map[string]any{"mime_type": mimeType, "data": base64.StdEncoding.EncodeToString(imageData)}},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CaptionResult{}, err
	}
	endpoint := g.Endpoint
	if g.APIKey != "" && !strings.Contains(endpoint, "key=") {
		if strings.Contains(endpoint, "?") {
			endpoint += "&key=" + g.APIKey
		} else {
			endpoint += "?key=" + g.APIKey
		}
	}
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	err = retry.Do(ctx, g.RetryMax, g.RetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := g.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("Gemini API エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("Gemini API エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return CaptionResult{}, err
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return CaptionResult{}, fmt.Errorf("gemini 応答が空です")
	}
	text := response.Candidates[0].Content.Parts[0].Text
	if result, ok := parseGeminiText(text); ok {
		return result, nil
	}
	return CaptionResult{Caption: text, PeopleCount: 0, Labels: []string{}}, nil
}

func parseGeminiText(text string) (CaptionResult, bool) {
	var result CaptionResult
	if err := json.Unmarshal([]byte(text), &result); err == nil {
		return result, true
	}
	candidate := extractJSON(text)
	if candidate == "" {
		return CaptionResult{}, false
	}
	if err := json.Unmarshal([]byte(candidate), &result); err == nil {
		return result, true
	}
	return CaptionResult{}, false
}

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func fetchImage(ctx context.Context, client *http.Client, url string, accessToken string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("画像取得に失敗: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return data, mimeType, nil
}
