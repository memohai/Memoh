package subagent

import "time"

// Subagent is a bot sub-agent definition (name, description, messages, skills, metadata).
type Subagent struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	BotID       string           `json:"bot_id"`
	Messages    []map[string]any `json:"messages"`
	Metadata    map[string]any   `json:"metadata"`
	Skills      []string         `json:"skills"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Deleted     bool             `json:"deleted"`
	DeletedAt   *time.Time       `json:"deleted_at,omitempty"`
}

// CreateRequest is the input for creating a subagent.
type CreateRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Messages    []map[string]any `json:"messages,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Skills      []string         `json:"skills,omitempty"`
}

// UpdateRequest is the input for updating name, description, metadata (all optional).
type UpdateRequest struct {
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// UpdateContextRequest is the input for replacing system messages.
type UpdateContextRequest struct {
	Messages []map[string]any `json:"messages"`
}

// UpdateSkillsRequest is the input for replacing the skills list.
type UpdateSkillsRequest struct {
	Skills []string `json:"skills"`
}

// AddSkillsRequest is the input for appending skills.
type AddSkillsRequest struct {
	Skills []string `json:"skills"`
}

// ListResponse holds the list of subagents for list API.
type ListResponse struct {
	Items []Subagent `json:"items"`
}

// ContextResponse holds the subagent context (messages) for get-context API.
type ContextResponse struct {
	Messages []map[string]any `json:"messages"`
}

// SkillsResponse holds the subagent skills list for get-skills API.
type SkillsResponse struct {
	Skills []string `json:"skills"`
}
