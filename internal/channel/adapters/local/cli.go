package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

type CLIAdapter struct {
	hub *channel.SessionHub
}

func NewCLIAdapter(hub *channel.SessionHub) *CLIAdapter {
	return &CLIAdapter{hub: hub}
}

func (a *CLIAdapter) Type() channel.ChannelType {
	return CLIType
}

func (a *CLIAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	if a.hub == nil {
		return fmt.Errorf("cli hub not configured")
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return fmt.Errorf("cli target is required")
	}
	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	a.hub.Publish(target, msg)
	return nil
}
