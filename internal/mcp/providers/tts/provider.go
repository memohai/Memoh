package tts

import (
	"context"
	"log/slog"
	"strings"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/settings"
	ttspkg "github.com/memohai/memoh/internal/tts"
)

const (
	toolTextToSpeech = "text_to_speech"
	maxTextLen       = 500
)

type Executor struct {
	logger    *slog.Logger
	settings  *settings.Service
	tts       *ttspkg.Service
	tempStore *ttspkg.TempStore
}

func NewExecutor(log *slog.Logger, settingsSvc *settings.Service, ttsSvc *ttspkg.Service, tempStore *ttspkg.TempStore) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:    log.With(slog.String("provider", "tts_tool")),
		settings:  settingsSvc,
		tts:       ttsSvc,
		tempStore: tempStore,
	}
}

func (e *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if e.settings == nil || e.tts == nil || e.tempStore == nil {
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
			Name:        toolTextToSpeech,
			Description: "Convert text to speech audio. Use this when the user asks you to speak, read aloud, or generate voice/audio.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The text to convert to speech (max 500 characters)",
					},
				},
				"required": []string{"text"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolTextToSpeech {
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

	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		e.logger.Error("failed to load bot settings", slog.String("bot_id", botID), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult("failed to load bot settings"), nil
	}
	if botSettings.TtsModelID == "" {
		return mcpgw.BuildToolErrorResult("bot has no TTS model configured"), nil
	}

	tempID, f, err := e.tempStore.Create()
	if err != nil {
		e.logger.Error("failed to create temp file", slog.Any("error", err))
		return mcpgw.BuildToolErrorResult("failed to create temp file"), nil
	}

	contentType, streamErr := e.tts.StreamToFile(ctx, botSettings.TtsModelID, text, f)
	closeErr := f.Close()
	if streamErr != nil {
		e.logger.Error("tts synthesis failed", slog.String("bot_id", botID), slog.Any("error", streamErr))
		e.tempStore.Delete(tempID)
		return mcpgw.BuildToolErrorResult("speech synthesis failed: " + streamErr.Error()), nil
	}
	if closeErr != nil {
		e.logger.Error("failed to finalize audio file", slog.String("bot_id", botID), slog.Any("error", closeErr))
		e.tempStore.Delete(tempID)
		return mcpgw.BuildToolErrorResult("failed to finalize audio file"), nil
	}

	size, _ := e.tempStore.FileSize(tempID)

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"type":         "tts_audio",
		"temp_id":      tempID,
		"content_type": contentType,
		"size":         size,
	}), nil
}
