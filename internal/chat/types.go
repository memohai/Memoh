package chat

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GatewayMessage map[string]any

type ChatRequest struct {
	BotID              string           `json:"-"`
	SessionID          string           `json:"-"`
	Token              string           `json:"-"`
	UserID             string           `json:"-"`
	ContainerID        string           `json:"-"`
	ContactID          string           `json:"-"`
	ContactName        string           `json:"-"`
	ContactAlias       string           `json:"-"`
	ReplyTarget        string           `json:"-"`
	SessionToken       string           `json:"-"`
	Query              string           `json:"query"`
	Model              string           `json:"model,omitempty"`
	Provider           string           `json:"provider,omitempty"`
	MaxContextLoadTime int              `json:"max_context_load_time,omitempty"`
	Language           string           `json:"language,omitempty"`
	Platforms          []string         `json:"platforms,omitempty"`
	CurrentPlatform    string           `json:"current_platform,omitempty"`
	Messages           []GatewayMessage `json:"messages,omitempty"`
	Skills             []string         `json:"skills,omitempty"`
	AllowedActions     []string         `json:"allowed_actions,omitempty"`
}

type ChatResponse struct {
	Messages []GatewayMessage `json:"messages"`
	Skills   []string         `json:"skills,omitempty"`
	Model    string           `json:"model,omitempty"`
	Provider string           `json:"provider,omitempty"`
}

type StreamChunk = json.RawMessage

type SchedulePayload struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	MaxCalls    *int   `json:"maxCalls,omitempty"`
	Command     string `json:"command"`
}

// NormalizedMessage is the internal unified message structure.
type NormalizedMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	Parts      []ContentPart `json:"parts,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type ContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	URL      string         `json:"url,omitempty"`
	Styles   []string       `json:"styles,omitempty"`
	Language string         `json:"language,omitempty"`
	UserID   string         `json:"user_id,omitempty"`
	Emoji    string         `json:"emoji,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
