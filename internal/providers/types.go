package providers

import "time"

// CreateRequest represents a request to create a new LLM provider
type CreateRequest struct {
	Name     string         `json:"name" validate:"required"`
	BaseURL  string         `json:"base_url" validate:"required,url"`
	APIKey   string         `json:"api_key"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateRequest represents a request to update an existing LLM provider
type UpdateRequest struct {
	Name     *string        `json:"name,omitempty"`
	BaseURL  *string        `json:"base_url,omitempty"`
	APIKey   *string        `json:"api_key,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// GetResponse represents the response for getting a provider
type GetResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	BaseURL   string         `json:"base_url"`
	APIKey    string         `json:"api_key,omitempty"` // masked in response
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ListResponse represents the response for listing providers
type ListResponse struct {
	Providers []GetResponse `json:"providers"`
	Total     int64         `json:"total"`
}

// CountResponse represents the count response
type CountResponse struct {
	Count int64 `json:"count"`
}

// TestResponse is returned by POST /providers/:id/test.
type TestResponse struct {
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Message   string `json:"message,omitempty"`
}

// RemoteModel represents a model returned by the provider's /v1/models endpoint
type RemoteModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// FetchModelsResponse represents the response from the provider's /v1/models endpoint
type FetchModelsResponse struct {
	Object string        `json:"object"`
	Data   []RemoteModel `json:"data"`
}

// ImportModelsRequest represents a request to import models from a provider
type ImportModelsRequest struct {
	ClientType string `json:"client_type"`
}

// ImportModelsResponse represents the response for importing models
type ImportModelsResponse struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Models  []string `json:"models"`
}
