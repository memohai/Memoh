package route

import (
	"context"
	"time"
)

// Route maps external channel conversations to an internal conversation.
type Route struct {
	ID               string         `json:"id"`
	ChatID           string         `json:"chat_id"`
	BotID            string         `json:"bot_id"`
	Platform         string         `json:"platform"`
	ChannelConfigID  string         `json:"channel_config_id,omitempty"`
	ConversationID   string         `json:"conversation_id"`
	ThreadID         string         `json:"thread_id,omitempty"`
	ConversationType string         `json:"conversation_type,omitempty"`
	ReplyTarget      string         `json:"reply_target,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// ResolveConversationResult is returned by ResolveConversation.
type ResolveConversationResult struct {
	ChatID  string
	RouteID string
	Created bool
}

// CreateInput is the input for creating a route.
type CreateInput struct {
	ChatID           string
	BotID            string
	Platform         string
	ChannelConfigID  string
	ConversationID   string
	ThreadID         string
	ConversationType string
	ReplyTarget      string
	Metadata         map[string]any
}

// ResolveInput is the input for route-to-conversation resolution.
type ResolveInput struct {
	BotID             string
	Platform          string
	ConversationID    string
	ThreadID          string
	ConversationType  string
	ChannelIdentityID string
	ChannelConfigID   string
	ReplyTarget       string
}

// Resolver defines the route resolution behavior used by inbound routing.
type Resolver interface {
	ResolveConversation(ctx context.Context, input ResolveInput) (ResolveConversationResult, error)
}

// Service defines route management behavior.
type Service interface {
	Resolver
	Create(ctx context.Context, input CreateInput) (Route, error)
	Find(ctx context.Context, botID, platform, conversationID, threadID string) (Route, error)
	GetByID(ctx context.Context, routeID string) (Route, error)
	List(ctx context.Context, chatID string) ([]Route, error)
	Delete(ctx context.Context, routeID string) error
	UpdateReplyTarget(ctx context.Context, routeID, replyTarget string) error
}
