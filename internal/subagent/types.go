package subagent

import "time"

type Subagent struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	BotID       string           `json:"bot_id"`
	Messages    []map[string]any `json:"messages"`
	Metadata    map[string]any   `json:"metadata"`
	Skills      []string         `json:"skills"`
	Usage       map[string]any   `json:"usage"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Deleted     bool             `json:"deleted"`
	DeletedAt   *time.Time       `json:"deleted_at,omitempty"`
}

type CreateRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Messages    []map[string]any `json:"messages,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Skills      []string         `json:"skills,omitempty"`
}

type UpdateRequest struct {
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateContextRequest struct {
	Messages []map[string]any `json:"messages"`
	Usage    map[string]any   `json:"usage,omitempty"`
}

type UpdateSkillsRequest struct {
	Skills []string `json:"skills"`
}

type AddSkillsRequest struct {
	Skills []string `json:"skills"`
}

type ListResponse struct {
	Items []Subagent `json:"items"`
}

type ContextResponse struct {
	Messages []map[string]any `json:"messages"`
	Usage    map[string]any   `json:"usage"`
}

type SkillsResponse struct {
	Skills []string `json:"skills"`
}
