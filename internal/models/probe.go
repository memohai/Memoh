package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	anthropicmessages "github.com/memohai/twilight-ai/provider/anthropic/messages"
	googlegenerative "github.com/memohai/twilight-ai/provider/google/generativeai"
	openaicompletions "github.com/memohai/twilight-ai/provider/openai/completions"
	openairesponses "github.com/memohai/twilight-ai/provider/openai/responses"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db"
)

const probeTimeout = 15 * time.Second

// Test probes a model's provider endpoint using the Twilight AI SDK
// to verify connectivity, authentication, and model availability.
func (s *Service) Test(ctx context.Context, id string) (TestResponse, error) {
	modelID, err := db.ParseUUID(id)
	if err != nil {
		return TestResponse{}, fmt.Errorf("invalid model id: %w", err)
	}

	model, err := s.queries.GetModelByID(ctx, modelID)
	if err != nil {
		return TestResponse{}, fmt.Errorf("get model: %w", err)
	}

	provider, err := s.queries.GetLlmProviderByID(ctx, model.LlmProviderID)
	if err != nil {
		return TestResponse{}, fmt.Errorf("get provider: %w", err)
	}

	baseURL := strings.TrimRight(provider.BaseUrl, "/")
	apiKey := provider.ApiKey
	clientType := ClientType(model.ClientType.String)

	// Embedding models don't have a chat Provider in the SDK — probe
	// the /embeddings endpoint directly.
	if model.Type == string(ModelTypeEmbedding) {
		return testEmbeddingModel(ctx, baseURL, apiKey, model.ModelID)
	}

	sdkProvider := NewSDKProvider(baseURL, apiKey, clientType, probeTimeout)

	start := time.Now()

	providerResult := sdkProvider.Test(ctx)
	switch providerResult.Status {
	case sdk.ProviderStatusUnreachable:
		return TestResponse{
			Status:    TestStatusError,
			Reachable: false,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   providerResult.Message,
		}, nil
	case sdk.ProviderStatusUnhealthy:
		return TestResponse{
			Status:    TestStatusAuthError,
			Reachable: true,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   providerResult.Message,
		}, nil
	}

	modelResult, err := sdkProvider.TestModel(ctx, model.ModelID)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return TestResponse{
			Status:    TestStatusError,
			Reachable: true,
			LatencyMs: latency,
			Message:   err.Error(),
		}, nil
	}

	if !modelResult.Supported {
		return TestResponse{
			Status:    TestStatusModelNotSupported,
			Reachable: true,
			LatencyMs: latency,
			Message:   modelResult.Message,
		}, nil
	}

	return TestResponse{
		Status:    TestStatusOK,
		Reachable: true,
		LatencyMs: latency,
		Message:   modelResult.Message,
	}, nil
}

// testEmbeddingModel probes an embedding model by sending a minimal
// request to the /embeddings endpoint.
func testEmbeddingModel(ctx context.Context, baseURL, apiKey, modelID string) (TestResponse, error) {
	body, _ := json.Marshal(map[string]any{"model": modelID, "input": "hello"})

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return TestResponse{Status: TestStatusError, Message: err.Error()}, nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL comes from operator-configured provider base URL
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return TestResponse{
			Status:    TestStatusError,
			Reachable: false,
			LatencyMs: latency,
			Message:   err.Error(),
		}, nil
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	result, classifyErr := sdk.ClassifyProbeStatus(resp.StatusCode)
	if classifyErr != nil {
		return TestResponse{
			Status:    TestStatusError,
			Reachable: true,
			LatencyMs: latency,
			Message:   classifyErr.Error(),
		}, nil
	}

	tr := TestResponse{
		Reachable: true,
		LatencyMs: latency,
		Message:   result.Message,
	}
	if result.Supported {
		tr.Status = TestStatusOK
	} else {
		tr.Status = TestStatusModelNotSupported
	}
	return tr, nil
}

// NewSDKProvider creates a Twilight AI SDK Provider for the given client type.
// It is exported so that other packages (e.g. providers) can reuse it for testing.
func NewSDKProvider(baseURL, apiKey string, clientType ClientType, timeout time.Duration) sdk.Provider {
	httpClient := &http.Client{Timeout: timeout}

	switch clientType {
	case ClientTypeOpenAIResponses:
		opts := []openairesponses.Option{
			openairesponses.WithAPIKey(apiKey),
			openairesponses.WithHTTPClient(httpClient),
		}
		if baseURL != "" {
			opts = append(opts, openairesponses.WithBaseURL(baseURL))
		}
		return openairesponses.New(opts...)

	case ClientTypeAnthropicMessages:
		opts := []anthropicmessages.Option{
			anthropicmessages.WithAPIKey(apiKey),
			anthropicmessages.WithHTTPClient(httpClient),
		}
		if baseURL != "" {
			opts = append(opts, anthropicmessages.WithBaseURL(baseURL))
		}
		return anthropicmessages.New(opts...)

	case ClientTypeGoogleGenerativeAI:
		opts := []googlegenerative.Option{
			googlegenerative.WithAPIKey(apiKey),
			googlegenerative.WithHTTPClient(httpClient),
		}
		if baseURL != "" {
			opts = append(opts, googlegenerative.WithBaseURL(baseURL))
		}
		return googlegenerative.New(opts...)

	default:
		opts := []openaicompletions.Option{
			openaicompletions.WithAPIKey(apiKey),
			openaicompletions.WithHTTPClient(httpClient),
		}
		if baseURL != "" {
			opts = append(opts, openaicompletions.WithBaseURL(baseURL))
		}
		return openaicompletions.New(opts...)
	}
}
