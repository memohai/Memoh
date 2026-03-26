package models

import (
	"net/http"
	"time"

	googleembedding "github.com/memohai/twilight-ai/provider/google/embedding"
	openaiembedding "github.com/memohai/twilight-ai/provider/openai/embedding"
	sdk "github.com/memohai/twilight-ai/sdk"
)

// NewSDKEmbeddingModel creates a Twilight AI SDK EmbeddingModel for the given
// provider configuration. It dispatches to the native Google embedding provider
// when clientType is "google-generative-ai", and falls back to the
// OpenAI-compatible /embeddings endpoint for all other provider types.
func NewSDKEmbeddingModel(clientType, baseURL, apiKey, modelID string, timeout time.Duration, httpClient *http.Client) *sdk.EmbeddingModel {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	switch ClientType(clientType) {
	case ClientTypeGoogleGenerativeAI:
		opts := []googleembedding.Option{
			googleembedding.WithAPIKey(apiKey),
			googleembedding.WithHTTPClient(httpClient),
		}
		if baseURL != "" {
			opts = append(opts, googleembedding.WithBaseURL(baseURL))
		}
		p := googleembedding.New(opts...)
		return p.EmbeddingModel(modelID)
	default:
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
}
