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

// Embedder produces vector embeddings for text (or other input) and reports dimension.
type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
	Dimensions() int
}

// OpenAIEmbedder calls OpenAI-compatible embedding API (e.g. OpenAI or local) for text.
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

// NewOpenAIEmbedder builds an OpenAIEmbedder; baseURL, apiKey, model required; dims must be positive.
func NewOpenAIEmbedder(log *slog.Logger, apiKey, baseURL, model string, dims int, timeout time.Duration) (*OpenAIEmbedder, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("openai embedder: base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("openai embedder: api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("openai embedder: model is required")
	}
	if dims <= 0 {
		return nil, errors.New("openai embedder: dimensions must be positive")
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
	}, nil
}

// Dimensions returns the embedding dimension configured for this embedder.
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dims
}

// Embed returns the embedding vector for the given text via the OpenAI-compatible API.
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			e.logger.Warn("embeddings: close response body failed", slog.Any("error", err))
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai embeddings error: %s", strings.TrimSpace(string(body)))
	}

	var parsed openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) == 0 {
		return nil, errors.New("openai embeddings empty response")
	}
	return parsed.Data[0].Embedding, nil
}
