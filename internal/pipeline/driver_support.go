package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func latestRCEventAtMs(rc RenderedContext) int64 {
	var latest int64
	for _, seg := range rc {
		if eventAtMs := seg.eventAtMs(); eventAtMs > latest {
			latest = eventAtMs
		}
	}
	return latest
}

func usageInputTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var usage struct {
		InputTokens int `json:"inputTokens"`
	}
	if json.Unmarshal(raw, &usage) != nil {
		return 0
	}
	return usage.InputTokens
}

func (d *DiscussDriver) loadDiscussCursor(ctx context.Context, cfg DiscussSessionConfig, log *slog.Logger) int64 {
	if d.deps.CursorStore == nil {
		return 0
	}
	cursor, err := d.deps.CursorStore.GetDiscussConsumedCursor(ctx, cfg.SessionID, discussCursorScope(cfg))
	if err != nil {
		log.Warn("discuss cursor load failed", slog.Any("error", err))
		return 0
	}
	return cursor
}

func (d *DiscussDriver) advanceDiscussCursor(ctx context.Context, sess *discussSession, cfg DiscussSessionConfig, cursor int64, log *slog.Logger) {
	if cursor <= sess.lastProcessedMs {
		return
	}
	sess.lastProcessedMs = cursor
	if d.deps.CursorStore == nil {
		return
	}
	if err := d.deps.CursorStore.UpsertDiscussConsumedCursor(ctx,
		cfg.SessionID,
		discussCursorScope(cfg),
		strings.TrimSpace(cfg.RouteID),
		strings.TrimSpace(cfg.CurrentPlatform),
		cursor,
	); err != nil {
		log.Warn("discuss cursor persist failed", slog.Any("error", err), slog.Int64("cursor", cursor))
	}
}

func discussCursorScope(cfg DiscussSessionConfig) string {
	if routeID := strings.TrimSpace(cfg.RouteID); routeID != "" {
		return "route:" + routeID
	}
	platform := strings.TrimSpace(cfg.CurrentPlatform)
	identityID := strings.TrimSpace(cfg.ChannelIdentityID)
	switch {
	case platform != "" && identityID != "":
		return "source:" + platform + ":" + identityID
	case platform != "":
		return "source:" + platform
	default:
		return "default"
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func anchorFromTRs(trs []TurnResponseEntry) int64 {
	var latest int64
	for _, tr := range trs {
		if tr.RequestedAtMs > latest {
			latest = tr.RequestedAtMs
		}
	}
	return latest
}

func (d *DiscussDriver) broadcastDiscussEvent(botID string, event agentpkg.StreamEvent) {
	if d.deps.Broadcaster == nil {
		return
	}
	streamEvent, ok := agentEventToChannelEvent(event)
	if !ok {
		return
	}
	d.deps.Broadcaster.PublishEvent(botID, streamEvent)
}

func agentEventToChannelEvent(event agentpkg.StreamEvent) (channel.StreamEvent, bool) {
	switch event.Type {
	case agentpkg.EventAgentStart:
		return channel.StreamEvent{Type: channel.StreamEventAgentStart}, true
	case agentpkg.EventTextStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseText}, true
	case agentpkg.EventTextDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: event.Delta}, true
	case agentpkg.EventTextEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText}, true
	case agentpkg.EventReasoningStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventReasoningDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: event.Delta, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventReasoningEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventToolCallStart:
		return channel.StreamEvent{Type: channel.StreamEventToolCallStart, ToolCall: &channel.StreamToolCall{
			Name: event.ToolName, CallID: event.ToolCallID, Input: event.Input,
		}}, true
	case agentpkg.EventToolCallEnd:
		return channel.StreamEvent{Type: channel.StreamEventToolCallEnd, ToolCall: &channel.StreamToolCall{
			Name: event.ToolName, CallID: event.ToolCallID, Input: event.Input, Result: event.Result,
		}}, true
	case agentpkg.EventToolApprovalRequest:
		return channel.StreamEvent{Type: channel.StreamEventToolCallStart, ToolCall: &channel.StreamToolCall{
			Name:       strings.TrimSpace(event.ToolName),
			CallID:     strings.TrimSpace(event.ToolCallID),
			Input:      event.Input,
			ApprovalID: strings.TrimSpace(event.ApprovalID),
			ShortID:    event.ShortID,
			Actions: []channel.Action{
				{Type: "tool_approval", Label: "Approve", Value: "approve:" + strings.TrimSpace(event.ApprovalID)},
				{Type: "tool_approval", Label: "Reject", Value: "reject:" + strings.TrimSpace(event.ApprovalID)},
			},
		}}, true
	case agentpkg.EventUserInputRequest:
		userInputID := strings.TrimSpace(event.UserInputID)
		if userInputID == "" {
			userInputID = strings.TrimSpace(event.ApprovalID)
		}
		return channel.StreamEvent{Type: channel.StreamEventToolCallStart, ToolCall: &channel.StreamToolCall{
			Name:   strings.TrimSpace(event.ToolName),
			CallID: strings.TrimSpace(event.ToolCallID),
			Input: map[string]any{
				"user_input_id": userInputID,
				"short_id":      event.ShortID,
				"status":        strings.TrimSpace(event.Status),
				"payload":       event.Input,
			},
			ShortID: event.ShortID,
			Actions: []channel.Action{{Type: "user_input", Label: "Respond", Value: "respond:" + userInputID}},
		}}, true
	case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
		return channel.StreamEvent{Type: channel.StreamEventAgentEnd}, true
	case agentpkg.EventError:
		return channel.StreamEvent{Type: channel.StreamEventError, Error: event.Error}, true
	default:
		return channel.StreamEvent{}, false
	}
}

func (d *DiscussDriver) loadTurnResponses(ctx context.Context, sessionID string) ([]TurnResponseEntry, []messagepkg.Message, error) {
	if d.deps.MessageService == nil {
		return nil, nil, nil
	}
	messages, err := d.deps.MessageService.ListActiveSinceBySession(ctx, sessionID, time.Unix(0, 0).UTC())
	if err != nil {
		d.logger.Warn("load TRs failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil, nil, err
	}
	responses := make([]TurnResponseEntry, 0, len(messages))
	for _, message := range messages {
		if entry, ok := DecodeTurnResponseEntry(message); ok {
			responses = append(responses, entry)
		}
	}
	return responses, messages, nil
}

func extractNewImageRefs(rc RenderedContext, afterMs int64) []ImageAttachmentRef {
	var refs []ImageAttachmentRef
	for _, segment := range rc {
		if segment.eventAtMs() > afterMs && !segment.IsMyself && !segment.IsSelfSent {
			refs = append(refs, segment.ImageRefs...)
		}
	}
	return refs
}

func injectImagePartsIntoLastUserMessage(messages []sdk.Message, parts []sdk.ImagePart) {
	if len(parts) == 0 {
		return
	}
	extra := make([]sdk.MessagePart, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Image) != "" {
			extra = append(extra, part)
		}
	}
	if len(extra) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == sdk.MessageRoleUser {
			messages[i].Content = append(messages[i].Content, extra...)
			return
		}
	}
}

func wasRecentlyMentioned(rc RenderedContext, afterMs int64) bool {
	for _, segment := range rc {
		if segment.eventAtMs() > afterMs && !segment.IsMyself && !segment.IsSelfSent && (segment.MentionsMe || segment.RepliesToMe) {
			return true
		}
	}
	return false
}

func buildLateBindingPrompt(isMentioned bool) string {
	now := time.Now().Format(time.RFC3339)
	var prompt strings.Builder
	prompt.WriteString("Current time: ")
	prompt.WriteString(now)
	prompt.WriteString("\n\n")
	prompt.WriteString("IMPORTANT: You MUST use the `send` tool to speak. Your text output is invisible to everyone — it is only internal monologue. ")
	prompt.WriteString("If you want to say something, you MUST call the `send` tool. Writing text without a tool call means absolute silence — no one will see it.")
	if isMentioned {
		prompt.WriteString("\n\nYou are being addressed directly. You should respond by calling the `send` tool now.")
	}
	return prompt.String()
}

func contextMessagesToSDKEntries(messages []ContextMessage) []sdkContextMessage {
	result := make([]sdkContextMessage, 0, len(messages))
	for _, message := range messages {
		entry := sdkContextMessage{CompactionArtifactID: strings.TrimSpace(message.CompactionArtifactID)}
		if len(message.RawContent) > 0 {
			raw, err := json.Marshal(struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}{Role: message.Role, Content: message.RawContent})
			if err == nil {
				var decoded sdk.Message
				if json.Unmarshal(raw, &decoded) == nil {
					entry.Message = decoded
					result = append(result, entry)
					continue
				}
			}
		}
		if message.Role == "assistant" {
			entry.Message = sdk.AssistantMessage(message.Content)
		} else {
			entry.Message = sdk.UserMessage(message.Content)
		}
		result = append(result, entry)
	}
	return result
}

func sdkMessagesFromContextEntries(entries []sdkContextMessage) []sdk.Message {
	messages := make([]sdk.Message, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	return messages
}

func compactionSummaryContextFrags(
	entries []sdkContextMessage,
	artifacts []CompactionArtifact,
	scope contextfrag.Scope,
) []contextfrag.ContextFrag {
	artifactsByID := make(map[string]CompactionArtifact, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			artifactsByID[id] = artifact
		}
	}

	frags := make([]contextfrag.ContextFrag, 0, len(artifactsByID))
	for index, entry := range entries {
		artifactID := strings.TrimSpace(entry.CompactionArtifactID)
		artifact, ok := artifactsByID[artifactID]
		if !ok {
			continue
		}
		coveredRefs := make([]contextfrag.ContextRef, 0, len(artifact.Sources))
		for _, source := range artifact.Sources {
			if contextfrag.ValidateContextRef(source.Ref) == nil {
				coveredRefs = append(coveredRefs, source.Ref)
			}
		}
		record := historyfrag.SummaryRecord(artifactID, artifact.Summary, coveredRefs, scope)
		frag := historyfrag.ToFrag(record)
		frag.ID = fmt.Sprintf("message.%03d", index)
		frag.Provenance.Index = index
		frags = append(frags, frag)
	}
	return frags
}
