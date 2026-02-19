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

// CheckStatus represents the result status of a single probe check.
type CheckStatus string

const (
	CheckStatusSupported   CheckStatus = "supported"
	CheckStatusAuthError   CheckStatus = "auth_error"
	CheckStatusUnsupported CheckStatus = "unsupported"
	CheckStatusError       CheckStatus = "error"
)

// CheckResult holds the outcome of probing a single endpoint.
type CheckResult struct {
	Status     CheckStatus `json:"status"`
	StatusCode int         `json:"status_code,omitempty"`
	LatencyMs  int64       `json:"latency_ms,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// TestResponse is returned by POST /providers/:id/test.
type TestResponse struct {
	Reachable bool                   `json:"reachable"`
	LatencyMs int64                  `json:"latency_ms,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Checks    map[string]CheckResult `json:"checks"`
}
