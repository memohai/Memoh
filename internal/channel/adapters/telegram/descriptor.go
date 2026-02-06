package telegram

import "github.com/memohai/memoh/internal/channel"

const Type channel.ChannelType = "telegram"

func init() {
	channel.MustRegisterChannel(channel.ChannelDescriptor{
		Type:                Type,
		DisplayName:         "Telegram",
		NormalizeConfig:     NormalizeConfig,
		NormalizeUserConfig: NormalizeUserConfig,
		ResolveTarget:       ResolveTarget,
		MatchBinding:        MatchBinding,
		BuildUserConfig:     BuildUserConfig,
		TargetSpec: channel.TargetSpec{
			Format: "chat_id | @username",
			Hints: []channel.TargetHint{
				{Label: "Chat ID", Example: "123456789"},
				{Label: "Username", Example: "@alice"},
			},
		},
		NormalizeTarget: normalizeTarget,
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Markdown:    true,
			Reply:       true,
			Attachments: true,
			Media:       true,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"botToken": {
					Type:     channel.FieldSecret,
					Required: true,
					Title:    "Bot Token",
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"username": {Type: channel.FieldString},
				"user_id":  {Type: channel.FieldString},
				"chat_id":  {Type: channel.FieldString},
			},
		},
	})
}
