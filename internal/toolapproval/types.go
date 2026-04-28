package toolapproval

import (
	"errors"
	"time"
)

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"

	DecisionBypass        = "bypass"
	DecisionNeedsApproval = "needs_approval"
)

var (
	ErrNotFound       = errors.New("tool approval request not found")
	ErrAlreadyDecided = errors.New("tool approval request already decided")
	ErrForbidden      = errors.New("tool approval forbidden")
	ErrAmbiguous      = errors.New("tool approval request is ambiguous")
)

type CreatePendingInput struct {
	BotID                        string
	SessionID                    string
	RouteID                      string
	ChannelIdentityID            string
	RequestedByChannelIdentityID string
	RequestedMessageID           string
	ToolCallID                   string
	ToolName                     string
	ToolInput                    any
	SourcePlatform               string
	ReplyTarget                  string
	ConversationType             string
}

type Evaluation struct {
	Decision string
	Request  Request
}

type ResolveInput struct {
	BotID                  string
	SessionID              string
	ExplicitID             string
	ReplyExternalMessageID string
}

type Request struct {
	ID                      string         `json:"id"`
	BotID                   string         `json:"bot_id"`
	SessionID               string         `json:"session_id"`
	RouteID                 string         `json:"route_id,omitempty"`
	ChannelIdentityID       string         `json:"channel_identity_id,omitempty"`
	ToolCallID              string         `json:"tool_call_id"`
	ToolName                string         `json:"tool_name"`
	ToolInput               map[string]any `json:"tool_input,omitempty"`
	ShortID                 int            `json:"short_id"`
	Status                  string         `json:"status"`
	DecisionReason          string         `json:"decision_reason,omitempty"`
	PromptExternalMessageID string         `json:"prompt_external_message_id,omitempty"`
	SourcePlatform          string         `json:"source_platform,omitempty"`
	ReplyTarget             string         `json:"reply_target,omitempty"`
	ConversationType        string         `json:"conversation_type,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	DecidedAt               *time.Time     `json:"decided_at,omitempty"`
}
