package channel

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrStopNotSupported is returned when a connection does not support graceful shutdown.
var ErrStopNotSupported = errors.New("channel connection stop not supported")

// InboundHandler is a callback invoked when a message arrives from a channel.
type InboundHandler func(ctx context.Context, cfg Config, msg InboundMessage) error

// StreamReplySender sends replies within a single inbound-processing scope.
// It supports both one-shot delivery and streaming sessions.
type StreamReplySender interface {
	Send(ctx context.Context, msg OutboundMessage) error
	OpenStream(ctx context.Context, target string, opts StreamOptions) (OutboundStream, error)
}

// OutboundStream is a live stream session for emitting outbound events.
type OutboundStream interface {
	Push(ctx context.Context, event StreamEvent) error
	Close(ctx context.Context) error
}

// ProcessingStatusInfo carries context for channel-level processing status updates.
type ProcessingStatusInfo struct {
	BotID             string
	ChatID            string
	RouteID           string
	ChannelIdentityID string
	UserID            string
	Query             string
	ReplyTarget       string
	SourceMessageID   string
}

// ProcessingStatusHandle stores channel-specific state between status callbacks.
type ProcessingStatusHandle struct {
	Token string
}

// ProcessingStatusNotifier reports processing lifecycle updates to channel platforms.
// Implementations should be best-effort and idempotent.
type ProcessingStatusNotifier interface {
	ProcessingStarted(ctx context.Context, cfg Config, msg InboundMessage, info ProcessingStatusInfo) (ProcessingStatusHandle, error)
	ProcessingCompleted(ctx context.Context, cfg Config, msg InboundMessage, info ProcessingStatusInfo, handle ProcessingStatusHandle) error
	ProcessingFailed(ctx context.Context, cfg Config, msg InboundMessage, info ProcessingStatusInfo, handle ProcessingStatusHandle, cause error) error
}

// Adapter is the base interface every channel adapter must implement.
type Adapter interface {
	Type() Type
	Descriptor() Descriptor
}

// Descriptor holds read-only metadata for a registered channel type.
// It contains no behavior — all behavior is expressed through optional interfaces.
type Descriptor struct {
	Type             Type
	DisplayName      string
	Configless       bool
	Capabilities     Capabilities
	OutboundPolicy   OutboundPolicy
	ConfigSchema     ConfigSchema
	UserConfigSchema ConfigSchema
	TargetSpec       TargetSpec
}

// ConfigNormalizer validates and normalizes channel and user-binding configurations.
type ConfigNormalizer interface {
	NormalizeConfig(raw map[string]any) (map[string]any, error)
	NormalizeUserConfig(raw map[string]any) (map[string]any, error)
}

// TargetResolver handles delivery target normalization and resolution from user bindings.
type TargetResolver interface {
	NormalizeTarget(raw string) string
	ResolveTarget(userConfig map[string]any) (string, error)
}

// BindingMatcher matches user-channel bindings and constructs binding configs from identities.
type BindingMatcher interface {
	MatchBinding(config map[string]any, criteria BindingCriteria) bool
	BuildUserConfig(identity Identity) map[string]any
}

// Sender is an adapter capable of sending outbound messages.
type Sender interface {
	Send(ctx context.Context, cfg Config, msg OutboundMessage) error
}

// StreamSender is an adapter capable of opening outbound stream sessions.
type StreamSender interface {
	OpenStream(ctx context.Context, cfg Config, target string, opts StreamOptions) (OutboundStream, error)
}

// MessageEditor updates and deletes already-sent messages when supported.
type MessageEditor interface {
	Update(ctx context.Context, cfg Config, target, messageID string, msg Message) error
	Unsend(ctx context.Context, cfg Config, target, messageID string) error
}

// Reactor adds or removes emoji reactions on messages.
type Reactor interface {
	React(ctx context.Context, cfg Config, target, messageID, emoji string) error
	Unreact(ctx context.Context, cfg Config, target, messageID, emoji string) error
}

// SelfDiscoverer retrieves the adapter bot's own identity from the platform.
// The returned map is merged into Config.SelfIdentity and persisted.
type SelfDiscoverer interface {
	DiscoverSelf(ctx context.Context, credentials map[string]any) (identity map[string]any, externalID string, err error)
}

// Receiver is an adapter capable of establishing a long-lived connection to receive messages.
type Receiver interface {
	Connect(ctx context.Context, cfg Config, handler InboundHandler) (Connection, error)
}

// Connection represents an active, long-lived link to a channel platform.
type Connection interface {
	ConfigID() string
	BotID() string
	Type() Type
	Stop(ctx context.Context) error
	Running() bool
}

// BaseConnection is a default Connection implementation backed by a stop function.
type BaseConnection struct {
	configID    string
	botID       string
	channelType Type
	stop        func(ctx context.Context) error
	running     atomic.Bool
}

// NewConnection creates a BaseConnection for the given config and stop function.
func NewConnection(cfg Config, stop func(ctx context.Context) error) *BaseConnection {
	conn := &BaseConnection{
		configID:    cfg.ID,
		botID:       cfg.BotID,
		channelType: cfg.Type,
		stop:        stop,
	}
	conn.running.Store(true)
	return conn
}

// ConfigID returns the channel configuration identifier.
func (c *BaseConnection) ConfigID() string {
	return c.configID
}

// BotID returns the bot identifier that owns this connection.
func (c *BaseConnection) BotID() string {
	return c.botID
}

// Type returns the type of channel this connection serves.
func (c *BaseConnection) Type() Type {
	return c.channelType
}

// Stop gracefully shuts down the connection.
func (c *BaseConnection) Stop(ctx context.Context) error {
	if c.stop == nil {
		return ErrStopNotSupported
	}
	c.running.Store(false)
	return c.stop(ctx)
}

// Running reports whether the connection is still active.
func (c *BaseConnection) Running() bool {
	return c.running.Load()
}
