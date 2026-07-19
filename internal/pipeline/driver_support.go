package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/historyfrag"
)

// SetResolver sets the RunConfigResolver after construction (breaks DI cycles).
func (d *DiscussDriver) SetResolver(r RunConfigResolver) {
	d.deps.Resolver = r
}

func (d *DiscussDriver) SetRuntimeStreamer(r discussRuntimeStreamer) {
	d.deps.RuntimeStreamer = r
}

// SetBroadcaster sets the stream broadcaster after construction so that
// discuss-mode agent events are forwarded to the Web UI in real time.
func (d *DiscussDriver) SetBroadcaster(b DiscussStreamBroadcaster) {
	d.deps.Broadcaster = b
}

func latestRCEventCursor(rc RenderedContext) int64 {
	var latest int64
	for _, seg := range rc {
		if cursor := seg.eventCursor(); cursor > latest {
			latest = cursor
		}
	}
	return latest
}

type discussCurrentTriggerSelection struct {
	exact   bool
	cursors map[int64]struct{}
}

func newDiscussCurrentTriggerSelection(deliveries []DiscussEventDelivery) discussCurrentTriggerSelection {
	selection := discussCurrentTriggerSelection{exact: len(deliveries) > 0}
	if !selection.exact {
		return selection
	}
	selection.cursors = make(map[int64]struct{}, len(deliveries))
	for _, delivery := range deliveries {
		if delivery.EventCursor > 0 {
			selection.cursors[delivery.EventCursor] = struct{}{}
		}
	}
	return selection
}

func eventDeliveriesFromConfig(cfg DiscussSessionConfig) []DiscussEventDelivery {
	if cfg.EventDelivery == nil {
		return nil
	}
	return []DiscussEventDelivery{*cfg.EventDelivery}
}

func (s discussCurrentTriggerSelection) contains(eventCursor, afterCursor int64) bool {
	if s.exact {
		_, ok := s.cursors[eventCursor]
		return ok
	}
	return eventCursor > afterCursor
}

func hasExternalCurrentTrigger(
	rc RenderedContext,
	selection discussCurrentTriggerSelection,
	afterCursor int64,
) bool {
	for _, segment := range rc {
		if !segment.IsMyself && !segment.IsSelfSent && selection.contains(segment.eventCursor(), afterCursor) {
			return true
		}
	}
	return false
}

func markCurrentTriggerSegments(
	rc RenderedContext,
	selection discussCurrentTriggerSelection,
) RenderedContext {
	if !selection.exact {
		return rc
	}
	marked := append(RenderedContext(nil), rc...)
	for i := range marked {
		marked[i].CurrentTriggerEvaluated = true
		marked[i].CurrentTrigger = !marked[i].IsMyself && !marked[i].IsSelfSent &&
			selection.contains(marked[i].eventCursor(), 0)
	}
	return marked
}

func recentRenderedContextWithCurrentTriggers(
	rc RenderedContext,
	windowStartAtMs int64,
) RenderedContext {
	if windowStartAtMs <= 0 {
		return RecentRenderedContext(rc, windowStartAtMs)
	}
	recent := make(RenderedContext, 0, len(rc))
	for _, segment := range rc {
		if segment.eventAtMs() >= windowStartAtMs ||
			(segment.CurrentTriggerEvaluated && segment.CurrentTrigger) {
			recent = append(recent, segment)
		}
	}
	return recent
}

func renderedContextDiscussCursor(rc RenderedContext) DiscussCursorPosition {
	position := DiscussCursorPosition{}
	for _, segment := range rc {
		if cursor := segment.eventCursor(); cursor > position.EventCursor {
			position.EventCursor = cursor
		}
		if cursor := segment.eventAtMs(); cursor > position.SourceCursor {
			position.SourceCursor = cursor
		}
	}
	return position
}

func discussCursorCommit(cfg DiscussSessionConfig, position DiscussCursorPosition) DiscussCursorCommit {
	return DiscussCursorCommit{
		ScopeKey: discussCursorScope(cfg),
		RouteID:  strings.TrimSpace(cfg.RouteID),
		Source:   strings.TrimSpace(cfg.CurrentPlatform),
		Position: position,
	}
}

func historyDiscussCursor(rc RenderedContext, sourceBoundary int64) DiscussCursorPosition {
	position := DiscussCursorPosition{}
	if sourceBoundary <= 0 {
		return position
	}
	for _, segment := range rc {
		sourceCursor := segment.eventAtMs()
		if sourceCursor > sourceBoundary {
			continue
		}
		if eventCursor := segment.eventCursor(); eventCursor > position.EventCursor {
			position.EventCursor = eventCursor
		}
		if sourceCursor > position.SourceCursor {
			position.SourceCursor = sourceCursor
		}
	}
	return position
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

func (d *DiscussDriver) maybeCompactDiscussContext(
	ctx context.Context,
	cfg DiscussSessionConfig,
	inputTokens int,
	contextTokenBudget int,
) {
	if inputTokens <= 0 || d.deps.Resolver == nil {
		return
	}
	d.deps.Resolver.ScheduleCompaction(
		ctx,
		cfg.BotID,
		cfg.SessionID,
		cfg.UserID,
		inputTokens,
		contextTokenBudget,
	)
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

func extractNewImageRefs(rc RenderedContext, afterCursor int64) []ImageAttachmentRef {
	var refs []ImageAttachmentRef
	for _, target := range extractNewImageTargets(rc, discussCurrentTriggerSelection{}, afterCursor) {
		refs = append(refs, target.refs...)
	}
	return refs
}

type newImageTarget struct {
	messageID   string
	eventCursor int64
	refs        []ImageAttachmentRef
}

func extractNewImageTargets(
	rc RenderedContext,
	selection discussCurrentTriggerSelection,
	afterCursor int64,
) []newImageTarget {
	var targets []newImageTarget
	for _, segment := range rc {
		if !selection.contains(segment.eventCursor(), afterCursor) || segment.IsMyself || segment.IsSelfSent || len(segment.ImageRefs) == 0 {
			continue
		}
		targets = append(targets, newImageTarget{
			messageID:   strings.TrimSpace(segment.MessageID),
			eventCursor: segment.eventCursor(),
			refs:        append([]ImageAttachmentRef(nil), segment.ImageRefs...),
		})
	}
	return targets
}

func injectImagePartsIntoRenderedUserMessage(
	entries []sdkContextMessage,
	messageID string,
	eventCursor int64,
	parts []sdk.ImagePart,
) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" && eventCursor <= 0 {
		return
	}
	for i := range entries {
		if entries[i].Message.Role != sdk.MessageRoleUser {
			continue
		}
		matched := messageID != "" && containsRenderedMessageID(entries[i].RenderedMessageIDs, messageID)
		if !matched && messageID == "" {
			matched = containsEventCursor(entries[i].ExternalEventCursors, eventCursor)
		}
		if !matched {
			continue
		}
		for _, part := range parts {
			if strings.TrimSpace(part.Image) != "" {
				entries[i].Message.Content = append(entries[i].Message.Content, part)
			}
		}
		return
	}
}

func containsEventCursor(cursors []int64, target int64) bool {
	for _, cursor := range cursors {
		if cursor == target {
			return true
		}
	}
	return false
}

func containsRenderedMessageID(ids []string, target string) bool {
	for _, id := range ids {
		if strings.TrimSpace(id) == target {
			return true
		}
	}
	return false
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

func wasRecentlyMentioned(rc RenderedContext, afterCursor int64) bool {
	return wasRecentlyMentionedWithTriggers(rc, discussCurrentTriggerSelection{}, afterCursor)
}

func wasRecentlyMentionedWithTriggers(
	rc RenderedContext,
	selection discussCurrentTriggerSelection,
	afterCursor int64,
) bool {
	for _, segment := range rc {
		if selection.contains(segment.eventCursor(), afterCursor) &&
			!segment.IsMyself && !segment.IsSelfSent && (segment.MentionsMe || segment.RepliesToMe) {
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
		entry := sdkContextMessage{
			CompactionArtifactID:      strings.TrimSpace(message.CompactionArtifactID),
			RenderedMessageIDs:        append([]string(nil), message.RenderedMessageIDs...),
			ExternalEventCursors:      append([]int64(nil), message.ExternalEventCursors...),
			LatestExternalEventCursor: message.LatestExternalEventCursor,
		}
		decodedRawContent := false
		if len(message.RawContent) > 0 {
			raw, err := json.Marshal(struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}{Role: message.Role, Content: message.RawContent})
			if err == nil {
				var decoded sdk.Message
				if json.Unmarshal(raw, &decoded) == nil {
					entry.Message = decoded
					decodedRawContent = true
				}
			}
		}
		if !decodedRawContent {
			switch {
			case strings.EqualFold(message.Role, "assistant"):
				entry.Message = sdk.AssistantMessage(message.Content)
			case strings.EqualFold(message.Role, "system"):
				entry.Message = sdk.SystemMessage(message.Content)
			default:
				entry.Message = sdk.UserMessage(message.Content)
			}
		}
		if message.CurrentTrigger && entry.Message.Role == sdk.MessageRoleUser {
			entry.Message.Content = append(
				[]sdk.MessagePart{sdk.TextPart{Text: "[user; current-trigger]"}},
				entry.Message.Content...,
			)
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
