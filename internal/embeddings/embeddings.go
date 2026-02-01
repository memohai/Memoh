package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
	Dimensions() int
}

type OpenAIEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	dims    int
	logger  *slog.Logger
	http    *http.Client
}

type openAIEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func NewOpenAIEmbedder(log *slog.Logger, apiKey, baseURL, model string, dims int, timeout time.Duration) *OpenAIEmbedder {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	if dims <= 0 {
		dims = 1536
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &OpenAIEmbedder{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		dims:    dims,
		logger:  log.With(slog.String("embedder", "openai")),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (e *OpenAIEmbedder) Dimensions() int {
	return e.dims
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, input string) ([]float32, error) {
	payload, err := json.Marshal(openAIEmbeddingRequest{
		Input: input,
		Model: e.model,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings error: %s", strings.TrimSpace(string(body)))
	}

	var parsed openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("openai embeddings empty response")
	}
	return parsed.Data[0].Embedding, nil
}
