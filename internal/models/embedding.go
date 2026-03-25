package models

import (
	"net/http"
	"time"

	openaiembedding "github.com/memohai/twilight-ai/provider/openai/embedding"
	sdk "github.com/memohai/twilight-ai/sdk"
)

// NewSDKEmbeddingModel creates a Twilight AI SDK EmbeddingModel for the given
// provider configuration. Currently all embedding providers use the
// OpenAI-compatible /embeddings endpoint (including Google-hosted models that
// expose the same wire format), so we route everything through the OpenAI
// embedding provider. If a future provider requires a different wire protocol,
// add a branch here.
func NewSDKEmbeddingModel(baseURL, apiKey, modelID string, timeout time.Duration) *sdk.EmbeddingModel {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	opts := []openaiembedding.Option{
		openaiembedding.WithAPIKey(apiKey),
		openaiembedding.WithHTTPClient(httpClient),
	}
	if baseURL != "" {
		opts = append(opts, openaiembedding.WithBaseURL(baseURL))
	}

	p := openaiembedding.New(opts...)
	return p.EmbeddingModel(modelID)
}
