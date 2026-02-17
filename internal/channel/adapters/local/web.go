package local

import (
	"context"
	"errors"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

// WebAdapter implements channel.Sender for the local Web channel.
type WebAdapter struct {
	hub *RouteHub
}

// NewWebAdapter creates a WebAdapter backed by the given route hub.
func NewWebAdapter(hub *RouteHub) *WebAdapter {
	return &WebAdapter{hub: hub}
}

// Type returns the Web channel type.
func (a *WebAdapter) Type() channel.Type {
	return WebType
}

// Descriptor returns the Web channel metadata.
func (a *WebAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        WebType,
		DisplayName: "Web",
		Configless:  true,
		Capabilities: channel.Capabilities{
			Text:           true,
			Reply:          true,
			Attachments:    true,
			Streaming:      true,
			BlockStreaming: true,
		},
		TargetSpec: channel.TargetSpec{
			Format: "bot_id",
			Hints: []channel.TargetHint{
				{Label: "Bot ID", Example: "bot_123"},
			},
		},
	}
}

// Send publishes an outbound message to the Web route hub.
func (a *WebAdapter) Send(_ context.Context, _ channel.Config, msg channel.OutboundMessage) error {
	if a.hub == nil {
		return errors.New("web hub not configured")
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return errors.New("web target is required")
	}
	if msg.Message.IsEmpty() {
		return errors.New("message is required")
	}
	a.hub.Publish(target, msg)
	return nil
}

// OpenStream opens a local stream session bound to the target route.
func (a *WebAdapter) OpenStream(ctx context.Context, _ channel.Config, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	if a.hub == nil {
		return nil, errors.New("web hub not configured")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("web target is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return newLocalOutboundStream(a.hub, target), nil
}
