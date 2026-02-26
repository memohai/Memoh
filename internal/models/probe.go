package models

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db"
)

const probeTimeout = 5 * time.Second

// Test probes a model's provider endpoint using the model's real model_id
// and client_type to verify that the configuration is valid.
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

	// Reachability check
	reachable, reachMsg := probeReachable(ctx, baseURL)
	if !reachable {
		return TestResponse{
			Status:  TestStatusError,
			Message: reachMsg,
		}, nil
	}

	// Select probe by client type (chat) or model type (embedding)
	var result probeResult
	if model.Type == string(ModelTypeEmbedding) {
		result = probeEmbedding(ctx, baseURL, apiKey, model.ModelID)
	} else {
		result = probeChatModel(ctx, baseURL, apiKey, model.ModelID, ClientType(model.ClientType.String))
	}

	return TestResponse{
		Status:    classifyProbe(result.statusCode),
		Reachable: true,
		LatencyMs: result.latencyMs,
		Message:   result.message,
	}, nil
}

type probeResult struct {
	statusCode int
	latencyMs  int64
	message    string
}

func probeChatModel(ctx context.Context, baseURL, apiKey, modelID string, clientType ClientType) probeResult {
	switch clientType {
	case ClientTypeOpenAIResponses:
		body := fmt.Sprintf(`{"model":%q,"input":"hi","max_output_tokens":1}`, modelID)
		return doProbe(ctx, http.MethodPost, baseURL+"/responses", openAIHeaders(apiKey), body)

	case ClientTypeOpenAICompletions:
		body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, modelID)
		return doProbe(ctx, http.MethodPost, baseURL+"/chat/completions", openAIHeaders(apiKey), body)

	case ClientTypeAnthropicMessages:
		body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, modelID)
		return doProbe(ctx, http.MethodPost, baseURL+"/messages", map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
			"Content-Type":      "application/json",
		}, body)

	case ClientTypeGoogleGenerativeAI:
		body := `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":1}}`
		url := fmt.Sprintf("%s/models/%s:generateContent", baseURL, modelID)
		return doProbe(ctx, http.MethodPost, url, map[string]string{
			"x-goog-api-key": apiKey,
			"Content-Type":   "application/json",
		}, body)

	default:
		// Fallback: treat as OpenAI completions compatible
		body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, modelID)
		return doProbe(ctx, http.MethodPost, baseURL+"/chat/completions", openAIHeaders(apiKey), body)
	}
}

func probeEmbedding(ctx context.Context, baseURL, apiKey, modelID string) probeResult {
	body := fmt.Sprintf(`{"model":%q,"input":"hello"}`, modelID)
	return doProbe(ctx, http.MethodPost, baseURL+"/embeddings", openAIHeaders(apiKey), body)
}

func openAIHeaders(apiKey string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}
}

func probeReachable(ctx context.Context, baseURL string) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return true, ""
}

func doProbe(ctx context.Context, method, url string, headers map[string]string, body string) probeResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return probeResult{message: err.Error()}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return probeResult{latencyMs: latency, message: err.Error()}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return probeResult{statusCode: resp.StatusCode, latencyMs: latency}
}

func classifyProbe(statusCode int) TestStatus {
	switch {
	case statusCode >= 200 && statusCode <= 299:
		return TestStatusOK
	case statusCode == 400 || statusCode == 422 || statusCode == 429:
		// 400/422 = endpoint works but request rejected; 429 = rate limited (model exists)
		return TestStatusOK
	case statusCode == 401 || statusCode == 403:
		return TestStatusAuthError
	default:
		return TestStatusError
	}
}
