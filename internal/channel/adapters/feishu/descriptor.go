package feishu

import "github.com/memohai/memoh/internal/channel"

const Type channel.ChannelType = "feishu"

func init() {
	channel.MustRegisterChannel(channel.ChannelDescriptor{
		Type:                Type,
		DisplayName:         "Feishu",
		NormalizeConfig:     NormalizeConfig,
		NormalizeUserConfig: NormalizeUserConfig,
		ResolveTarget:       ResolveTarget,
		MatchBinding:        MatchBinding,
		BuildUserConfig:     BuildUserConfig,
		TargetSpec: channel.TargetSpec{
			Format: "open_id:xxx | user_id:xxx | chat_id:xxx",
			Hints: []channel.TargetHint{
				{Label: "Open ID", Example: "open_id:ou_xxx"},
				{Label: "User ID", Example: "user_id:ou_xxx"},
				{Label: "Chat ID", Example: "chat_id:oc_xxx"},
			},
		},
		NormalizeTarget: normalizeTarget,
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			RichText:    true,
			Attachments: true,
			Reply:       true,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"appId":     {Type: channel.FieldString, Required: true, Title: "App ID"},
				"appSecret": {Type: channel.FieldSecret, Required: true, Title: "App Secret"},
				"encryptKey": {
					Type:  channel.FieldSecret,
					Title: "Encrypt Key",
				},
				"verificationToken": {
					Type:  channel.FieldSecret,
					Title: "Verification Token",
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"open_id": {Type: channel.FieldString},
				"user_id": {Type: channel.FieldString},
			},
		},
	})
}
