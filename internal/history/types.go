package history

import "time"

type Record struct {
	ID        string           `json:"id"`
	Messages  []map[string]any `json:"messages"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Skills    []string         `json:"skills"`
	Timestamp time.Time        `json:"timestamp"`
	BotID     string           `json:"bot_id"`
	SessionID string           `json:"session_id"`
}

type CreateRequest struct {
	Messages []map[string]any `json:"messages"`
	Metadata map[string]any   `json:"metadata,omitempty"`
	Skills   []string         `json:"skills,omitempty"`
}

type ListResponse struct {
	Items []Record `json:"items"`
}
