package userinput

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	ToolNameAskUser = "ask_user"

	StatusPending   = "pending"
	StatusSubmitted = "submitted"
	StatusCanceled  = "canceled"
	StatusExpired   = "expired"
	StatusFailed    = "failed"

	DeferredKind = "user_input"

	ActionTypeUserInput = "user_input"
	ActionSubmit        = "submit"
	ActionCancel        = "cancel"
)

var (
	ErrNotFound       = errors.New("user input request not found")
	ErrAlreadyDecided = errors.New("user input request already decided")
)

type CreatePendingInput struct {
	BotID                        string
	SessionID                    string
	RouteID                      string
	ChannelIdentityID            string
	RequestedByChannelIdentityID string
	ToolCallID                   string
	ToolName                     string
	Input                        any
	ProviderMetadata             map[string]any
	SourcePlatform               string
	ReplyTarget                  string
	ConversationType             string
	ExpiresAt                    *time.Time
}

type ResolveInput struct {
	BotID                  string
	SessionID              string
	ExplicitID             string
	ReplyExternalMessageID string
}

type SubmitInput struct {
	RequestID              string
	ActorChannelIdentityID string
	Answer                 any
	OptionID               string
	OptionValue            any
	RawUserResponse        map[string]any
}

type CancelInput struct {
	RequestID              string
	ActorChannelIdentityID string
	Reason                 string
}

type Request struct {
	ID                      string         `json:"id"`
	BotID                   string         `json:"bot_id"`
	SessionID               string         `json:"session_id"`
	RouteID                 string         `json:"route_id,omitempty"`
	ChannelIdentityID       string         `json:"channel_identity_id,omitempty"`
	ToolCallID              string         `json:"tool_call_id"`
	ToolName                string         `json:"tool_name"`
	ShortID                 int            `json:"short_id"`
	Status                  string         `json:"status"`
	Input                   map[string]any `json:"input,omitempty"`
	UIPayload               UIPayload      `json:"ui_payload"`
	Result                  map[string]any `json:"result,omitempty"`
	ProviderMetadata        map[string]any `json:"-"`
	PromptExternalMessageID string         `json:"prompt_external_message_id,omitempty"`
	SourcePlatform          string         `json:"source_platform,omitempty"`
	ReplyTarget             string         `json:"reply_target,omitempty"`
	ConversationType        string         `json:"conversation_type,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	RespondedAt             *time.Time     `json:"responded_at,omitempty"`
	CanceledAt              *time.Time     `json:"canceled_at,omitempty"`
}

type UIPayload struct {
	Question    string     `json:"question"`
	Options     []UIOption `json:"options,omitempty"`
	AllowCustom bool       `json:"allow_custom,omitempty"`
	InputType   string     `json:"input_type,omitempty"`
	Placeholder string     `json:"placeholder,omitempty"`
}

type UIOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       any    `json:"value,omitempty"`
	InputType   string `json:"input_type,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

func ResultBytes(req Request) []byte {
	if len(req.Result) == 0 {
		return []byte("{}")
	}
	data, err := json.Marshal(req.Result)
	if err != nil {
		return []byte("{}")
	}
	return data
}
