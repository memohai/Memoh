package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

func (d *DiscussDriver) streamDiscussACPRuntime(
	ctx context.Context,
	cfg DiscussSessionConfig,
	composed *ComposeContextResult,
	afterCursor int64,
	isMentioned bool,
	cursor DiscussCursorCommit,
	log *slog.Logger,
) (bool, bool, bool) {
	if d.deps.RuntimeStreamer == nil {
		log.Error("discuss ACP runtime: streamer not configured")
		return false, false, false
	}
	prompt := discussACPFullContextPrompt(composed.Messages, afterCursor, buildLateBindingPrompt(isMentioned))
	if strings.TrimSpace(prompt) == "" {
		return false, false, false
	}
	claims := make([]conversation.DeliveryClaim, len(cursor.DeliveryClaims))
	for i, claim := range cursor.DeliveryClaims {
		claims[i] = conversation.DeliveryClaim{
			EventID:    claim.EventID,
			ClaimToken: claim.ClaimToken,
		}
	}
	chunks, errs := d.deps.RuntimeStreamer.StreamChat(ctx, conversation.ChatRequest{
		BotID:                      cfg.BotID,
		ChatID:                     cfg.BotID,
		SessionID:                  cfg.SessionID,
		UserID:                     cfg.UserID,
		RouteID:                    cfg.RouteID,
		SourceChannelIdentityID:    cfg.ChannelIdentityID,
		CurrentChannel:             cfg.CurrentPlatform,
		ReplyTarget:                cfg.ReplyTarget,
		ConversationType:           cfg.ConversationType,
		ConversationName:           cfg.ConversationName,
		Token:                      cfg.SessionToken,
		ChatToken:                  cfg.ChatToken,
		ToolHTTPURL:                cfg.ToolHTTPURL,
		Query:                      prompt,
		RawQuery:                   prompt,
		UserMessagePersisted:       true,
		PersistedUserMessageID:     cfg.PersistedUserMessageID,
		SkipMemoryExtraction:       true,
		ForceFreshRuntime:          true,
		DiscussCursorScope:         cursor.ScopeKey,
		DiscussConsumedCursor:      cursor.Position.SourceCursor,
		DiscussConsumedEventCursor: cursor.Position.EventCursor,
		DiscussDeliveryClaims:      claims,
	})
	streamed := false
	terminal := false
	cursorCommitted := false
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
				cursorCommitted = event.Metadata != nil && event.Metadata[agentpkg.MetadataKeyDiscussCursorCommitted] == true
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
			return true, false, false
		}
	}
	return true, streamed && terminal && (!failed || cursorCommitted), cursorCommitted
}

func discussACPFullContextPrompt(messages []ContextMessage, afterCursor int64, lateBinding string) string {
	var b strings.Builder
	b.WriteString("You are replying in a discuss-mode conversation. The runtime is reset each turn, so use the complete context below as the source of truth.\n\n")
	hasCurrentTrigger := false
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		currentTrigger := strings.EqualFold(role, "user") &&
			strings.TrimSpace(msg.CompactionArtifactID) == "" &&
			(msg.CurrentTrigger || (!msg.CurrentTriggerEvaluated && msg.LatestExternalEventCursor > afterCursor))
		hasCurrentTrigger = hasCurrentTrigger || currentTrigger
		content := msg.Content
		if len(msg.RawContent) > 0 {
			content = string(msg.RawContent)
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		b.WriteString("[")
		b.WriteString(role)
		if currentTrigger {
			b.WriteString("; current-trigger")
		}
		b.WriteString("]\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	if hasCurrentTrigger {
		b.WriteString("Reply to the user-visible message or messages marked as current-trigger when a response is appropriate.")
	} else {
		b.WriteString("Reply when a response is appropriate based on the conversation context.")
	}
	if strings.TrimSpace(lateBinding) != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(lateBinding))
	}
	return strings.TrimSpace(b.String())
}
