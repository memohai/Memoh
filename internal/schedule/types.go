package schedule

import (
	"encoding/json"
	"time"
)

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
	UserID       string    `json:"user_id"`
}

type NullableInt struct {
	Value *int
	Set   bool
}

func (n NullableInt) IsZero() bool {
	return !n.Set
}

func (n NullableInt) MarshalJSON() ([]byte, error) {
	if !n.Set || n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.Value)
}

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

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	MaxCalls    NullableInt `json:"max_calls,omitempty"`
	Command     string `json:"command"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type UpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Pattern     *string `json:"pattern,omitempty"`
	MaxCalls    NullableInt `json:"max_calls,omitempty"`
	Command     *string `json:"command,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

type ListResponse struct {
	Items []Schedule `json:"items"`
}

