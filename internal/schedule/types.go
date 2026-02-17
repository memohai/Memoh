package schedule

import (
	"encoding/json"
	"time"
)

// Schedule is a cron schedule attached to a bot (pattern, command, max calls, enabled).
type Schedule struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Pattern      string    `json:"pattern"`
	MaxCalls     *int      `json:"max_calls,omitempty"`
	CurrentCalls int       `json:"current_calls"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Enabled      bool      `json:"enabled"`
	Command      string    `json:"command"`
	BotID        string    `json:"bot_id"`
}

// NullableInt represents an optional int for JSON (null vs omitted).
type NullableInt struct {
	Value *int
	Set   bool
}

// IsZero reports whether the value was not set (omitempty semantics).
func (n NullableInt) IsZero() bool {
	return !n.Set
}

// MarshalJSON encodes as null when unset or value is nil, otherwise the int.
func (n NullableInt) MarshalJSON() ([]byte, error) {
	if !n.Set || n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.Value)
}

// UnmarshalJSON decodes null or an int and sets Set true.
func (n *NullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

// CreateRequest is the input for creating a schedule (name, description, cron pattern, command, etc.).
type CreateRequest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Pattern     string      `json:"pattern"`
	MaxCalls    NullableInt `json:"max_calls,omitzero"`
	Command     string      `json:"command"`
	Enabled     *bool       `json:"enabled,omitempty"`
}

// UpdateRequest is the input for updating a schedule (all fields optional).
type UpdateRequest struct {
	Name        *string     `json:"name,omitempty"`
	Description *string     `json:"description,omitempty"`
	Pattern     *string     `json:"pattern,omitempty"`
	MaxCalls    NullableInt `json:"max_calls,omitzero"`
	Command     *string     `json:"command,omitempty"`
	Enabled     *bool       `json:"enabled,omitempty"`
}

// ListResponse holds the list of schedules for list API.
type ListResponse struct {
	Items []Schedule `json:"items"`
}
