package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

type WebAdapter struct {
	hub *channel.SessionHub
}

func NewWebAdapter(hub *channel.SessionHub) *WebAdapter {
	return &WebAdapter{hub: hub}
}

func (a *WebAdapter) Type() channel.ChannelType {
	return WebType
}

func (a *WebAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	if a.hub == nil {
		return fmt.Errorf("web hub not configured")
	}
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return fmt.Errorf("web target is required")
	}
	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	a.hub.Publish(target, msg)
	return nil
}
