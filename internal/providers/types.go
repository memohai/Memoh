package providers

import "time"

// CreateRequest represents a request to create a new provider.
type CreateRequest struct {
	Name       string         `json:"name" validate:"required"`
	ClientType string         `json:"client_type" validate:"required"`
	Icon       string         `json:"icon,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// UpdateRequest represents a request to update an existing provider.
type UpdateRequest struct {
	Name       *string        `json:"name,omitempty"`
	ClientType *string        `json:"client_type,omitempty"`
	Icon       *string        `json:"icon,omitempty"`
	Enable     *bool          `json:"enable,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// GetResponse represents the response for getting a provider.
type GetResponse struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	ClientType string         `json:"client_type"`
	Icon       string         `json:"icon,omitempty"`
	Enable     bool           `json:"enable"`
	Config     map[string]any `json:"config,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// ListResponse represents the response for listing providers.
type ListResponse struct {
	Providers []GetResponse `json:"providers"`
	Total     int64         `json:"total"`
}

// CountResponse represents the count response.
type CountResponse struct {
	Count int64 `json:"count"`
}

// TestResponse is returned by POST /providers/:id/test.
type TestResponse struct {
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Message   string `json:"message,omitempty"`
}

// OAuthStatus is returned by GET /providers/:id/oauth/status.
type OAuthStatus struct {
	Configured  bool               `json:"configured"`
	Mode        string             `json:"mode,omitempty"`
	HasToken    bool               `json:"has_token"`
	Expired     bool               `json:"expired"`
	ExpiresAt   *time.Time         `json:"expires_at,omitempty"`
	CallbackURL string             `json:"callback_url"`
	Device      *OAuthDeviceStatus `json:"device,omitempty"`
	Account     *OAuthAccount      `json:"account,omitempty"`
}

type OAuthDeviceStatus struct {
	Pending         bool       `json:"pending"`
	UserCode        string     `json:"user_code,omitempty"`
	VerificationURI string     `json:"verification_uri,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	IntervalSeconds int64      `json:"interval_seconds,omitempty"`
}

type OAuthAccount struct {
	Label      string `json:"label,omitempty"`
	Login      string `json:"login,omitempty"`
	Name       string `json:"name,omitempty"`
	Email      string `json:"email,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
}

type OAuthAuthorizeResponse struct {
	Mode    string             `json:"mode,omitempty"`
	AuthURL string             `json:"auth_url,omitempty"`
	Device  *OAuthDeviceStatus `json:"device,omitempty"`
}

// RemoteModel represents a model returned by the provider's /v1/models endpoint.
type RemoteModel struct {
	ID               string   `json:"id"`
	Object           string   `json:"object"`
	Created          int64    `json:"created"`
	OwnedBy          string   `json:"owned_by"`
	Name             string   `json:"name,omitempty"`
	Type             string   `json:"type,omitempty"`
	Compatibilities  []string `json:"compatibilities,omitempty"`
	ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`
}

// FetchModelsResponse represents the response from the provider's /v1/models endpoint.
type FetchModelsResponse struct {
	Object string        `json:"object"`
	Data   []RemoteModel `json:"data"`
}

// ImportModelsResponse represents the response for importing models.
type ImportModelsResponse struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Models  []string `json:"models"`
}
