package route

import (
	"context"
	"time"
)

// Route maps an external channel conversation/thread to a bot and its active
// internal thread. JSON names remain compatible with the existing channel API.
type Route struct {
	ID                     string         `json:"id"`
	BotID                  string         `json:"bot_id"`
	Platform               string         `json:"platform"`
	ChannelConfigID        string         `json:"channel_config_id,omitempty"`
	ExternalConversationID string         `json:"conversation_id"`
	ExternalThreadID       string         `json:"thread_id,omitempty"`
	ConversationType       string         `json:"conversation_type,omitempty"`
	ReplyTarget            string         `json:"reply_target,omitempty"`
	ActiveThreadID         string         `json:"active_session_id,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// ResolveConversationResult is returned by ResolveConversation.
type ResolveConversationResult struct {
	BotID   string
	RouteID string
	Created bool
}

// CreateInput is the input for creating a route.
type CreateInput struct {
	BotID                  string
	Platform               string
	ChannelConfigID        string
	ExternalConversationID string
	ExternalThreadID       string
	ConversationType       string
	ReplyTarget            string
	Metadata               map[string]any
}

// ResolveInput is the input for route-to-conversation resolution.
type ResolveInput struct {
	BotID                  string
	Platform               string
	ExternalConversationID string
	ExternalThreadID       string
	ConversationType       string
	ChannelConfigID        string
	ReplyTarget            string
	Metadata               map[string]any
}

// Resolver defines the route resolution behavior used by inbound routing.
type Resolver interface {
	ResolveConversation(ctx context.Context, input ResolveInput) (ResolveConversationResult, error)
}

// Service defines route management behavior.
type Service interface {
	Resolver
	Create(ctx context.Context, input CreateInput) (Route, error)
	Find(ctx context.Context, botID, platform, externalConversationID, externalThreadID string) (Route, error)
	GetByID(ctx context.Context, routeID string) (Route, error)
	List(ctx context.Context, botID string) ([]Route, error)
	Delete(ctx context.Context, routeID string) error
	UpdateReplyTarget(ctx context.Context, routeID, replyTarget string) error
	UpdateMetadata(ctx context.Context, routeID string, metadata map[string]any) error
}
