package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

func (d *DiscussDriver) newDirectDiscussPromptRecipe(
	cfg DiscussSessionConfig,
	rc RenderedContext,
	cursor int64,
	scope contextfrag.Scope,
	initial DirectDiscussPromptInput,
	lateBinding func(RenderedContext) string,
) DirectDiscussPromptRecipe {
	return DirectDiscussPromptRecipe{
		Initial: initial,
		Rebuild: func(ctx context.Context) (DirectDiscussPromptInput, error) {
			history, err := d.deps.Resolver.LoadContextHistoryProjection(ctx, cfg.BotID, cfg.SessionID)
			if err != nil {
				return DirectDiscussPromptInput{}, err
			}
			active := ActiveRenderedContext(rc, history.CompactionArtifacts)
			composed := composeContextWithArtifactsAtCursor(
				rc,
				history.TurnResponses,
				history.CompactionArtifacts,
				cursor,
			)
			if composed == nil {
				return DirectDiscussPromptInput{}, errors.New("direct discuss context disappeared during prompt rebuild")
			}
			return buildDirectDiscussPromptInput(
				composed.Messages,
				history.CompactionArtifacts,
				scope,
				lateBinding(active),
				cfg.UserID,
			), nil
		},
	}
}

func (d *DiscussDriver) streamDiscussACPRuntime(
	ctx context.Context,
	cfg DiscussSessionConfig,
	messages []sdk.Message,
	log *slog.Logger,
) (bool, bool) {
	if d.deps.RuntimeStreamer == nil {
		log.Error("discuss ACP runtime: streamer not configured")
		return false, false
	}
	prompt := discussACPFullContextPrompt(messages)
	if strings.TrimSpace(prompt) == "" {
		return false, false
	}
	chunks, errs := d.deps.RuntimeStreamer.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   cfg.BotID,
		ChatID:                  cfg.BotID,
		SessionID:               cfg.SessionID,
		RouteID:                 cfg.RouteID,
		SourceChannelIdentityID: cfg.ChannelIdentityID,
		CurrentChannel:          cfg.CurrentPlatform,
		ReplyTarget:             cfg.ReplyTarget,
		ConversationType:        cfg.ConversationType,
		Token:                   cfg.SessionToken,
		ChatToken:               cfg.ChatToken,
		ToolHTTPURL:             cfg.ToolHTTPURL,
		Query:                   prompt,
		RawQuery:                prompt,
		UserMessagePersisted:    true,
		SkipMemoryExtraction:    true,
		ForceFreshRuntime:       true,
	})
	streamed := false
	terminal := false
	failed := false
	for chunks != nil || errs != nil {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			var event agentpkg.StreamEvent
			if err := json.Unmarshal(chunk, &event); err != nil {
				log.Warn("discuss ACP runtime: decode stream event failed", slog.Any("error", err))
				failed = true
				continue
			}
			streamed = true
			if event.Type == agentpkg.EventError {
				failed = true
			}
			if event.Type == agentpkg.EventAgentEnd || event.Type == agentpkg.EventAgentAbort {
				terminal = true
			}
			d.broadcastDiscussEvent(cfg.BotID, event)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				log.Error("discuss ACP runtime failed", slog.Any("error", err))
				failed = true
			}
		case <-ctx.Done():
			log.Warn("discuss ACP runtime cancelled", slog.Any("error", ctx.Err()))
			return true, false
		}
	}
	return true, streamed && terminal && !failed
}

func discussACPFullContextPrompt(messages []sdk.Message) string {
	var b strings.Builder
	b.WriteString("You are replying in a discuss-mode conversation. The runtime is reset each turn, so use the complete context below as the source of truth.\n\n")
	for _, msg := range messages {
		role := strings.TrimSpace(string(msg.Role))
		if role == "" {
			role = "user"
		}
		content := discussACPMessageContent(msg)
		if content == "" {
			continue
		}
		b.WriteString("[")
		b.WriteString(role)
		b.WriteString("]\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func discussACPMessageContent(message sdk.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		switch value := part.(type) {
		case sdk.TextPart:
			if text := strings.TrimSpace(value.Text); text != "" {
				parts = append(parts, text)
			}
		case sdk.ToolCallPart:
			parts = append(parts, discussACPToolMarker("tool_call", value.ToolName, value.Input))
		case sdk.ToolResultPart:
			parts = append(parts, discussACPToolMarker("tool_result", value.ToolName, value.Result))
		case sdk.ImagePart:
			parts = append(parts, "[image]")
		case sdk.FilePart:
			parts = append(parts, "[file]")
		case sdk.ReasoningPart:
			continue
		default:
			if payload := boundedACPValue(value); payload != "" {
				parts = append(parts, payload)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func discussACPToolMarker(kind, name string, payload any) string {
	marker := "[" + kind
	if name = strings.TrimSpace(name); name != "" {
		marker += ": " + name
	}
	marker += "]"
	if rendered := boundedACPValue(payload); rendered != "" && rendered != "{}" && rendered != "null" {
		marker += " " + rendered
	}
	return marker
}

func boundedACPValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	const maxRunes = 4096
	runes := []rune(strings.TrimSpace(string(raw)))
	if len(runes) > maxRunes {
		runes = append(runes[:maxRunes], '…')
	}
	return string(runes)
}
