package local

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

const (
	CLIType channel.ChannelType = "cli"
	WebType channel.ChannelType = "web"
)

func init() {
	channel.MustRegisterChannel(channel.ChannelDescriptor{
		Type:                CLIType,
		DisplayName:         "CLI",
		NormalizeConfig:     normalizeEmpty,
		NormalizeUserConfig: normalizeEmpty,
		BuildUserConfig:     buildEmpty,
		Configless:          true,
		TargetSpec: channel.TargetSpec{
			Format: "session_id",
			Hints: []channel.TargetHint{
				{Label: "Session ID", Example: "cli:uuid"},
			},
		},
		NormalizeTarget: normalizeTarget,
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Reply:       true,
			Attachments: true,
		},
	})
	channel.MustRegisterChannel(channel.ChannelDescriptor{
		Type:                WebType,
		DisplayName:         "Web",
		NormalizeConfig:     normalizeEmpty,
		NormalizeUserConfig: normalizeEmpty,
		BuildUserConfig:     buildEmpty,
		Configless:          true,
		TargetSpec: channel.TargetSpec{
			Format: "session_id",
			Hints: []channel.TargetHint{
				{Label: "Session ID", Example: "web:uuid"},
			},
		},
		NormalizeTarget: normalizeTarget,
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Reply:       true,
			Attachments: true,
		},
	})
}

func normalizeTarget(raw string) string {
	return strings.TrimSpace(raw)
}

func normalizeEmpty(map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}

func buildEmpty(channel.Identity) map[string]any {
	return map[string]any{}
}
