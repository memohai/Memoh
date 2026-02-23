package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service handles provider operations
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a new provider service
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "providers")),
	}
}

// Create creates a new LLM provider
func (s *Service) Create(ctx context.Context, req CreateRequest) (GetResponse, error) {
	// Marshal metadata
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}

	// Create provider
	provider, err := s.queries.CreateLlmProvider(ctx, sqlc.CreateLlmProviderParams{
		Name:     req.Name,
		BaseUrl:  req.BaseURL,
		ApiKey:   req.APIKey,
		Metadata: metadataJSON,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("create provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// Get retrieves a provider by ID
func (s *Service) Get(ctx context.Context, id string) (GetResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// GetByName retrieves a provider by name
func (s *Service) GetByName(ctx context.Context, name string) (GetResponse, error) {
	provider, err := s.queries.GetLlmProviderByName(ctx, name)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider by name: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// List retrieves all providers
func (s *Service) List(ctx context.Context) ([]GetResponse, error) {
	providers, err := s.queries.ListLlmProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}

	results := make([]GetResponse, 0, len(providers))
	for _, p := range providers {
		results = append(results, s.toGetResponse(p))
	}
	return results, nil
}

// Update updates an existing provider
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}

	// Get existing provider
	existing, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	// Apply updates
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}

	baseURL := existing.BaseUrl
	if req.BaseURL != nil {
		baseURL = *req.BaseURL
	}

	apiKey := resolveUpdatedAPIKey(existing.ApiKey, req.APIKey)

	metadata := existing.Metadata
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
		}
		metadata = metadataJSON
	}

	// Update provider
	updated, err := s.queries.UpdateLlmProvider(ctx, sqlc.UpdateLlmProviderParams{
		ID:       providerID,
		Name:     name,
		BaseUrl:  baseURL,
		ApiKey:   apiKey,
		Metadata: metadata,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("update provider: %w", err)
	}

	return s.toGetResponse(updated), nil
}

// Delete deletes a provider by ID
func (s *Service) Delete(ctx context.Context, id string) error {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}

	if err := s.queries.DeleteLlmProvider(ctx, providerID); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// Count returns the total count of providers
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountLlmProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("count providers: %w", err)
	}
	return count, nil
}

const probeTimeout = 5 * time.Second

// Test probes the provider's base URL to check connectivity, supported
// client types, and embedding support. All probes run concurrently.
func (s *Service) Test(ctx context.Context, id string) (TestResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return TestResponse{}, err
	}

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return TestResponse{}, fmt.Errorf("get provider: %w", err)
	}

	baseURL := strings.TrimRight(provider.BaseUrl, "/")
	apiKey := provider.ApiKey

	resp := TestResponse{Checks: make(map[string]CheckResult, 5)}

	// Connectivity check
	start := time.Now()
	reachable, reachMsg := probeReachable(ctx, baseURL)
	resp.Reachable = reachable
	resp.LatencyMs = time.Since(start).Milliseconds()
	if !reachable {
		resp.Message = reachMsg
		return resp, nil
	}

	type namedResult struct {
		name   string
		result CheckResult
	}

	probes := []struct {
		name string
		fn   func() CheckResult
	}{
		{"openai-completions", func() CheckResult {
			return probeOpenAICompletions(ctx, baseURL, apiKey)
		}},
		{"openai-responses", func() CheckResult {
			return probeOpenAIResponses(ctx, baseURL, apiKey)
		}},
		{"anthropic-messages", func() CheckResult {
			return probeAnthropicMessages(ctx, baseURL, apiKey)
		}},
		{"google-generative-ai", func() CheckResult {
			return probeGoogleGenerativeAI(ctx, baseURL, apiKey)
		}},
		{"embedding", func() CheckResult {
			return probeEmbedding(ctx, baseURL, apiKey)
		}},
	}

	results := make([]namedResult, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(idx int, name string, fn func() CheckResult) {
			defer wg.Done()
			results[idx] = namedResult{name: name, result: fn()}
		}(i, p.name, p.fn)
	}
	wg.Wait()

	for _, nr := range results {
		resp.Checks[nr.name] = nr.result
	}
	return resp, nil
}

func probeReachable(ctx context.Context, baseURL string) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return false, err.Error()
	}
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	io.Copy(io.Discard, httpResp.Body)
	httpResp.Body.Close()
	return true, ""
}

func probeOpenAICompletions(ctx context.Context, baseURL, apiKey string) CheckResult {
	return probeEndpoint(ctx, http.MethodGet, baseURL+"/models",
		map[string]string{
			"Authorization": "Bearer " + apiKey,
		}, "")
}

func probeOpenAIResponses(ctx context.Context, baseURL, apiKey string) CheckResult {
	body := `{"model":"probe-test","input":"hi","max_output_tokens":1}`
	return probeEndpoint(ctx, http.MethodPost, baseURL+"/responses",
		map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, body)
}

func probeAnthropicMessages(ctx context.Context, baseURL, apiKey string) CheckResult {
	body := `{"model":"probe-test","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`
	return probeEndpoint(ctx, http.MethodPost, baseURL+"/messages",
		map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
			"Content-Type":      "application/json",
		}, body)
}

func probeGoogleGenerativeAI(ctx context.Context, baseURL, apiKey string) CheckResult {
	return probeEndpoint(ctx, http.MethodGet, baseURL+"/models",
		map[string]string{
			"x-goog-api-key": apiKey,
		}, "")
}

func probeEmbedding(ctx context.Context, baseURL, apiKey string) CheckResult {
	body := `{"model":"probe-test","input":"hello"}`
	return probeEndpoint(ctx, http.MethodPost, baseURL+"/embeddings",
		map[string]string{
			"Authorization": "Bearer " + apiKey,
			"Content-Type":  "application/json",
		}, body)
}

func probeEndpoint(ctx context.Context, method, url string, headers map[string]string, body string) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return CheckResult{Status: CheckStatusError, Message: err.Error()}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return CheckResult{Status: CheckStatusError, LatencyMs: latency, Message: err.Error()}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return classifyResponse(resp.StatusCode, latency)
}

func classifyResponse(statusCode int, latencyMs int64) CheckResult {
	r := CheckResult{StatusCode: statusCode, LatencyMs: latencyMs}
	switch {
	case statusCode >= 200 && statusCode <= 299,
		statusCode == 400, statusCode == 422, statusCode == 429:
		r.Status = CheckStatusSupported
	case statusCode == 401 || statusCode == 403:
		r.Status = CheckStatusAuthError
	case statusCode == 404 || statusCode == 405:
		r.Status = CheckStatusUnsupported
	default:
		r.Status = CheckStatusError
		r.Message = fmt.Sprintf("unexpected status %d", statusCode)
	}
	return r
}

// toGetResponse converts a database provider to a response
func (s *Service) toGetResponse(provider sqlc.LlmProvider) GetResponse {
	var metadata map[string]any
	if len(provider.Metadata) > 0 {
		if err := json.Unmarshal(provider.Metadata, &metadata); err != nil {
			slog.Warn("provider metadata unmarshal failed", slog.String("id", provider.ID.String()), slog.Any("error", err))
		}
	}

	// Mask API key (show only first 8 characters)
	maskedAPIKey := maskAPIKey(provider.ApiKey)

	return GetResponse{
		ID:        provider.ID.String(),
		Name:      provider.Name,
		BaseURL:   provider.BaseUrl,
		APIKey:    maskedAPIKey,
		Metadata:  metadata,
		CreatedAt: provider.CreatedAt.Time,
		UpdatedAt: provider.UpdatedAt.Time,
	}
}

// maskAPIKey masks an API key for security
func maskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:8] + strings.Repeat("*", len(apiKey)-8)
}

// resolveUpdatedAPIKey keeps the original key when the request value matches the masked version.
// This prevents masked placeholder values from overwriting the real stored credential.
func resolveUpdatedAPIKey(existing string, updated *string) string {
	if updated == nil {
		return existing
	}
	if *updated == maskAPIKey(existing) {
		return existing
	}
	return *updated
}
