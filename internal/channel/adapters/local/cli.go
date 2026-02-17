package local

import (
	"context"
	"errors"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

// CLIAdapter implements channel.Sender for the local CLI channel.
type CLIAdapter struct {
	hub *RouteHub
}

// NewCLIAdapter creates a CLIAdapter backed by the given route hub.
func NewCLIAdapter(hub *RouteHub) *CLIAdapter {
	return &CLIAdapter{hub: hub}
}

// Type returns the CLI channel type.
func (a *CLIAdapter) Type() channel.Type {
	return CLIType
}

// Descriptor returns the CLI channel metadata.
func (a *CLIAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        CLIType,
		DisplayName: "CLI",
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

// Send publishes an outbound message to the CLI route hub.
func (a *CLIAdapter) Send(_ context.Context, _ channel.Config, msg channel.OutboundMessage) error {
	if a.hub == nil {
		return errors.New("cli hub not configured")
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return errors.New("cli target is required")
	}
	if msg.Message.IsEmpty() {
		return errors.New("message is required")
	}
	a.hub.Publish(target, msg)
	return nil
}

// OpenStream opens a local stream session bound to the target route.
func (a *CLIAdapter) OpenStream(ctx context.Context, _ channel.Config, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	if a.hub == nil {
		return nil, errors.New("cli hub not configured")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("cli target is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return newLocalOutboundStream(a.hub, target), nil
}
