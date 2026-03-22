package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/settings"
	ttspkg "github.com/memohai/memoh/internal/tts"
)

const ttsMaxTextLen = 500

// TTSSender sends outbound messages through the channel manager.
type TTSSender interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// TTSChannelResolver parses platform name to channel type.
type TTSChannelResolver interface {
	ParseChannelType(raw string) (channel.ChannelType, error)
}

type TTSProvider struct {
	logger   *slog.Logger
	settings *settings.Service
	tts      *ttspkg.Service
	sender   TTSSender
	resolver TTSChannelResolver
}

func NewTTSProvider(log *slog.Logger, settingsSvc *settings.Service, ttsSvc *ttspkg.Service, sender TTSSender, resolver TTSChannelResolver) *TTSProvider {
	if log == nil {
		log = slog.Default()
	}
	return &TTSProvider{
		logger:   log.With(slog.String("tool", "tts")),
		settings: settingsSvc,
		tts:      ttsSvc,
		sender:   sender,
		resolver: resolver,
	}
}

func (p *TTSProvider) Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.settings == nil || p.tts == nil || p.sender == nil || p.resolver == nil {
		return nil, nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, nil
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, nil
	}
	if strings.TrimSpace(botSettings.TtsModelID) == "" {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "speak",
			Description: "Send a voice message to a DIFFERENT channel or person. Synthesizes text to speech and delivers as audio. Do NOT use this for the current conversation — use <speech> block instead.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":     map[string]any{"type": "string", "description": "The text to convert to speech (max 500 characters)"},
					"platform": map[string]any{"type": "string", "description": "Channel platform name. Defaults to current session platform."},
					"target":   map[string]any{"type": "string", "description": "Channel target (chat/group/thread ID). Use get_contacts to find available targets."},
					"reply_to": map[string]any{"type": "string", "description": "Message ID to reply to. The voice message will reference this message on the platform."},
				},
				"required": []string{"text"},
			},
			Execute: func(execCtx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSpeak(execCtx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *TTSProvider) execSpeak(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	text := strings.TrimSpace(StringArg(args, "text"))
	if text == "" {
		return nil, errors.New("text is required")
	}
	if len([]rune(text)) > ttsMaxTextLen {
		return nil, errors.New("text too long, max 500 characters")
	}
	channelType, err := p.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	target := FirstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return nil, errors.New("target is required")
	}
	if strings.EqualFold(channelType.String(), strings.TrimSpace(session.CurrentPlatform)) &&
		target == strings.TrimSpace(session.ReplyTarget) {
		return nil, errors.New("you are trying to speak in the same conversation you are already in. " +
			"Do not use the speak tool for this. Instead, use the <speech> block in your response " +
			"(e.g. <speech>Hello world</speech>)")
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, errors.New("failed to load bot settings")
	}
	if botSettings.TtsModelID == "" {
		return nil, errors.New("bot has no TTS model configured")
	}
	audioData, contentType, synthErr := p.tts.Synthesize(ctx, botSettings.TtsModelID, text, nil)
	if synthErr != nil {
		return nil, fmt.Errorf("speech synthesis failed: %s", synthErr.Error())
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(audioData))
	msg := channel.Message{
		Attachments: []channel.Attachment{{Type: channel.AttachmentVoice, URL: dataURL, Mime: contentType, Size: int64(len(audioData))}},
	}
	if replyTo := FirstStringArg(args, "reply_to"); replyTo != "" {
		msg.Reply = &channel.ReplyRef{MessageID: replyTo}
	}
	if err := p.sender.Send(ctx, botID, channelType, channel.SendRequest{Target: target, Message: msg}); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "bot_id": botID, "platform": channelType.String(), "target": target,
		"instruction": "Voice message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
	}, nil
}

func (p *TTSProvider) resolvePlatform(args map[string]any, session SessionContext) (channel.ChannelType, error) {
	platform := FirstStringArg(args, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", errors.New("platform is required")
	}
	return p.resolver.ParseChannelType(platform)
}
