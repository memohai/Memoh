package tts

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/settings"
	ttspkg "github.com/memohai/memoh/internal/tts"
)

const (
	toolSpeak  = "speak"
	maxTextLen = 500
)

// Sender sends outbound messages through the channel manager.
type Sender interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// ChannelTypeResolver parses platform name to channel type.
type ChannelTypeResolver interface {
	ParseChannelType(raw string) (channel.ChannelType, error)
}

type Executor struct {
	logger   *slog.Logger
	settings *settings.Service
	tts      *ttspkg.Service
	sender   Sender
	resolver ChannelTypeResolver
}

func NewExecutor(log *slog.Logger, settingsSvc *settings.Service, ttsSvc *ttspkg.Service, sender Sender, resolver ChannelTypeResolver) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:   log.With(slog.String("provider", "speak_tool")),
		settings: settingsSvc,
		tts:      ttsSvc,
		sender:   sender,
		resolver: resolver,
	}
}

func (e *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if e.settings == nil || e.tts == nil || e.sender == nil || e.resolver == nil {
		return nil, nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, nil
	}
	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, nil
	}
	if strings.TrimSpace(botSettings.TtsModelID) == "" {
		return nil, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolSpeak,
			Description: "Send a voice message to a DIFFERENT channel or person. Synthesizes text to speech and delivers as audio. Do NOT use this for the current conversation — use <speech> block instead.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The text to convert to speech (max 500 characters)",
					},
					"platform": map[string]any{
						"type":        "string",
						"description": "Channel platform name. Defaults to current session platform.",
					},
					"target": map[string]any{
						"type":        "string",
						"description": "Channel target (chat/group/thread ID). Use get_contacts to find available targets.",
					},
					"reply_to": map[string]any{
						"type":        "string",
						"description": "Message ID to reply to. The voice message will reference this message on the platform.",
					},
				},
				"required": []string{"text"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolSpeak {
		return nil, mcpgw.ErrToolNotFound
	}

	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	text := strings.TrimSpace(mcpgw.StringArg(arguments, "text"))
	if text == "" {
		return mcpgw.BuildToolErrorResult("text is required"), nil
	}
	if len([]rune(text)) > maxTextLen {
		return mcpgw.BuildToolErrorResult("text too long, max 500 characters"), nil
	}

	channelType, err := e.resolvePlatform(arguments, session)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	target := mcpgw.FirstStringArg(arguments, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return mcpgw.BuildToolErrorResult("target is required"), nil
	}

	// Reject when destination matches the current conversation.
	if strings.EqualFold(channelType.String(), strings.TrimSpace(session.CurrentPlatform)) &&
		target == strings.TrimSpace(session.ReplyTarget) {
		return mcpgw.BuildToolErrorResult(
			"You are trying to speak in the SAME conversation you are already in. " +
				"Do NOT use the speak tool for this. Instead, use the <speech> block in your response " +
				"(e.g. <speech>Hello world</speech>).",
		), nil
	}

	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		e.logger.Error("failed to load bot settings", slog.String("bot_id", botID), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult("failed to load bot settings"), nil
	}
	if botSettings.TtsModelID == "" {
		return mcpgw.BuildToolErrorResult("bot has no TTS model configured"), nil
	}

	audioData, contentType, synthErr := e.tts.Synthesize(ctx, botSettings.TtsModelID, text, nil)
	if synthErr != nil {
		e.logger.Error("tts synthesis failed", slog.String("bot_id", botID), slog.Any("error", synthErr))
		return mcpgw.BuildToolErrorResult("speech synthesis failed: " + synthErr.Error()), nil
	}

	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(audioData))

	msg := channel.Message{
		Attachments: []channel.Attachment{
			{
				Type: channel.AttachmentVoice,
				URL:  dataURL,
				Mime: contentType,
				Size: int64(len(audioData)),
			},
		},
	}

	if replyTo := mcpgw.FirstStringArg(arguments, "reply_to"); replyTo != "" {
		msg.Reply = &channel.ReplyRef{MessageID: replyTo}
	}

	sendReq := channel.SendRequest{
		Target:  target,
		Message: msg,
	}
	if err := e.sender.Send(ctx, botID, channelType, sendReq); err != nil {
		e.logger.Warn("speak send failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	payload := map[string]any{
		"ok":          true,
		"bot_id":      botID,
		"platform":    channelType.String(),
		"target":      target,
		"instruction": "Voice message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
	}
	return mcpgw.BuildToolSuccessResult(payload), nil
}

func (e *Executor) resolvePlatform(arguments map[string]any, session mcpgw.ToolSessionContext) (channel.ChannelType, error) {
	platform := mcpgw.FirstStringArg(arguments, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", errors.New("platform is required")
	}
	return e.resolver.ParseChannelType(platform)
}
