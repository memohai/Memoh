package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Default DashScope API base URL and embedding path.
const (
	DefaultDashScopeBaseURL = "https://dashscope.aliyuncs.com"
	DashScopeEmbeddingPath  = "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding"
)

// DashScopeEmbedder calls Aliyun DashScope multimodal embedding API.
type DashScopeEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	logger  *slog.Logger
	http    *http.Client
}

// DashScopeUsage holds token/duration usage from DashScope API response.
// DashScopeUsage holds token/duration usage from DashScope API response.
type DashScopeUsage struct {
	InputTokens int `json:"input_tokens"`
	ImageTokens int `json:"image_tokens"`
	ImageCount  int `json:"image_count,omitempty"`
	Duration    int `json:"duration,omitempty"`
}

type dashScopeRequest struct {
	Model string                `json:"model"`
	Input dashScopeRequestInput `json:"input"`
}

type dashScopeRequestInput struct {
	Contents []map[string]string `json:"contents"`
}

type dashScopeResponse struct {
	Output struct {
		Embeddings []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
			Type      string    `json:"type"`
		} `json:"embeddings"`
	} `json:"output"`
	Usage     DashScopeUsage `json:"usage"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
}

// NewDashScopeEmbedder builds a DashScope embedder; baseURL defaults to DefaultDashScopeBaseURL if empty.
func NewDashScopeEmbedder(log *slog.Logger, apiKey, baseURL, model string, timeout time.Duration) *DashScopeEmbedder {
	if baseURL == "" {
		baseURL = DefaultDashScopeBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DashScopeEmbedder{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		logger:  log.With(slog.String("embedder", "dashscope")),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

// Embed returns the embedding vector and usage for text and/or image/video URLs via DashScope API.
func (e *DashScopeEmbedder) Embed(ctx context.Context, text, imageURL, videoURL string) ([]float32, DashScopeUsage, error) {
	contents := make([]map[string]string, 0, 3)
	if strings.TrimSpace(text) != "" {
		contents = append(contents, map[string]string{"text": text})
	}
	if strings.TrimSpace(imageURL) != "" {
		contents = append(contents, map[string]string{"image": imageURL})
	}
	if strings.TrimSpace(videoURL) != "" {
		contents = append(contents, map[string]string{"video": videoURL})
	}
	if len(contents) == 0 {
		return nil, DashScopeUsage{}, errors.New("dashscope input is required")
	}

	payload, err := json.Marshal(dashScopeRequest{
		Model: e.model,
		Input: dashScopeRequestInput{Contents: contents},
	})
	if err != nil {
		return nil, DashScopeUsage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+DashScopeEmbeddingPath, bytes.NewReader(payload))
	if err != nil {
		return nil, DashScopeUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, DashScopeUsage{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			e.logger.Warn("dashscope embeddings: close response body failed", slog.Any("error", err))
		}
	}()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, DashScopeUsage{}, fmt.Errorf("dashscope embeddings error: %s", strings.TrimSpace(string(body)))
	}

	var parsed dashScopeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, DashScopeUsage{}, err
	}
	if parsed.Code != "" {
		return nil, parsed.Usage, fmt.Errorf("dashscope embeddings error: %s", parsed.Message)
	}
	if len(parsed.Output.Embeddings) == 0 {
		return nil, parsed.Usage, errors.New("dashscope embeddings empty response")
	}

	var preferredType string
	switch {
	case strings.TrimSpace(text) != "":
		preferredType = "text"
	case strings.TrimSpace(imageURL) != "":
		preferredType = "image"
	case strings.TrimSpace(videoURL) != "":
		preferredType = "video"
	}

	if preferredType != "" {
		for _, item := range parsed.Output.Embeddings {
			if strings.EqualFold(item.Type, preferredType) && len(item.Embedding) > 0 {
				return item.Embedding, parsed.Usage, nil
			}
		}
	}

	return parsed.Output.Embeddings[0].Embedding, parsed.Usage, nil
}
