package compaction

import (
	"net/http"
	"time"
)

// Log represents a compaction log entry.
type Log struct {
	ID           string     `json:"id"`
	BotID        string     `json:"bot_id"`
	SessionID    string     `json:"session_id,omitempty"`
	Status       string     `json:"status"`
	Summary      string     `json:"summary"`
	MessageCount int        `json:"message_count"`
	ErrorMessage string     `json:"error_message"`
	Usage        any        `json:"usage,omitempty"`
	ModelID      string     `json:"model_id,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// ListLogsResponse is the API response for listing compaction logs.
type ListLogsResponse struct {
	Items []Log `json:"items"`
}

// TriggerConfig holds the parameters needed to trigger a compaction.
type TriggerConfig struct {
	BotID          string
	SessionID      string
	ModelID        string
	ClientType     string
	APIKey         string //nolint:gosec // runtime credential, not a hardcoded secret
	CodexAccountID string
	BaseURL        string
	HTTPClient     *http.Client
}
