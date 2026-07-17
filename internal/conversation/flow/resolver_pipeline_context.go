package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

type pipelineContextBuild struct {
	Messages        []conversation.ModelMessage
	HistoryRecords  []historyfrag.HistoryRecord
	EstimatedTokens int
}

type pipelineHistoryProjectionBuild struct {
	projection     pipelinepkg.ContextHistoryProjection
	summaryRecords []historyfrag.HistoryRecord
}

type composedPipelineMessage struct {
	message            conversation.ModelMessage
	renderedMessageIDs []string
	summaryRecord      historyfrag.HistoryRecord
	hasSummary         bool
	forceKeep          bool
}

func (r *Resolver) buildPipelineContext(
	ctx context.Context,
	req conversation.ChatRequest,
	contextTokenBudget int,
	currentQuery string,
) (pipelineContextBuild, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if r.pipeline == nil || sessionID == "" {
		return pipelineContextBuild{}, nil
	}
	rc := r.pipeline.GetRC(sessionID)
	scope := compactionSummaryScope(
		req.BotID,
		req.ChatID,
		req.SessionID,
		req.ConversationType,
		req.ConversationName,
		req.ReplyTarget,
	)
	history, err := r.loadPipelineHistoryProjection(ctx, scope, historyScopeFallbackFromChatRequest(req))
	if err != nil {
		return pipelineContextBuild{}, err
	}
	rc = pipelinepkg.RecentRenderedContext(rc, history.projection.WindowStartAtMs)
	rc = replaceCurrentRenderedQuery(rc, req.ExternalMessageID, currentQuery)
	composed := pipelinepkg.ComposeContextProjection(rc, history.projection)
	entries := composedPipelineMessages(composed, history.summaryRecords)
	entries = appendCurrentPipelineQueryIfMissing(
		entries,
		pipelinepkg.ActiveRenderedContext(rc, history.projection.CompactionArtifacts),
		req.ExternalMessageID,
		currentQuery,
	)
	return trimComposedPipelineMessages(r.logger, entries, contextTokenBudget), nil
}

func replaceCurrentRenderedQuery(
	rc pipelinepkg.RenderedContext,
	currentMessageID string,
	currentQuery string,
) pipelinepkg.RenderedContext {
	currentMessageID = strings.TrimSpace(currentMessageID)
	currentQuery = strings.TrimSpace(currentQuery)
	if currentMessageID == "" || currentQuery == "" {
		return rc
	}
	for i := range rc {
		if strings.TrimSpace(rc[i].MessageID) != currentMessageID {
			continue
		}
		updated := append(pipelinepkg.RenderedContext(nil), rc...)
		updated[i].Content = []pipelinepkg.RenderedContentPiece{{Type: "text", Text: currentQuery}}
		return updated
	}
	return rc
}

func (r *Resolver) loadPipelineHistoryProjection(
	ctx context.Context,
	scope contextfrag.Scope,
	fallback historyfrag.ScopeFallback,
) (pipelineHistoryProjectionBuild, error) {
	since := time.Now().UTC().Add(-time.Duration(defaultMaxContextMinutes) * time.Minute)
	if r.messageService == nil {
		artifacts, summaries, err := r.loadPipelineCompactionArtifacts(ctx, scope, nil)
		return pipelineHistoryProjectionBuild{
			projection: pipelinepkg.ContextHistoryProjection{
				CompactionArtifacts: artifacts,
				WindowStartAtMs:     since.UnixMilli(),
			},
			summaryRecords: summaries,
		}, err
	}
	messages, err := r.messageService.ListActiveSinceBySession(ctx, scope.SessionID, since)
	if err != nil {
		return pipelineHistoryProjectionBuild{}, err
	}
	records := make([]historyfrag.HistoryRecord, 0, len(messages))
	for _, message := range messages {
		record, err := historyfrag.FromDBMessageWithLogger(r.logger, message, fallback)
		if err != nil {
			return pipelineHistoryProjectionBuild{}, err
		}
		records = append(records, record)
	}
	artifacts, summaries, err := r.loadPipelineCompactionArtifacts(ctx, scope, records)
	if err != nil {
		return pipelineHistoryProjectionBuild{}, err
	}
	historyPositions, externalPositions := pipelineHistoryPositions(messages)
	if positionReader, ok := r.messageService.(messagepkg.ExternalMessagePositionReader); ok {
		positions, readErr := positionReader.ListExternalMessagePositionsBySession(
			ctx,
			scope.SessionID,
			pipelineRenderedMessageIDs(r.pipeline, scope.SessionID, since.UnixMilli()),
		)
		if readErr != nil {
			return pipelineHistoryProjectionBuild{}, readErr
		}
		for _, position := range positions {
			externalID := strings.TrimSpace(position.ExternalMessageID)
			if externalID == "" || position.TurnPosition <= 0 || position.TurnMessageSequence <= 0 {
				continue
			}
			externalPositions[externalID] = pipelinepkg.HistoryPosition{
				TurnPosition:    position.TurnPosition,
				MessageSequence: position.TurnMessageSequence,
			}
		}
	}
	turnResponseMessages := messages
	if historyReader, ok := r.messageService.(messagepkg.TurnResponseHistoryReader); ok {
		turnResponseMessages, err = historyReader.ListUncoveredTurnResponsesBySession(
			ctx,
			scope.SessionID,
			since,
			coveredHistoryMessageIDs(artifacts),
		)
		if err != nil {
			return pipelineHistoryProjectionBuild{}, err
		}
	}
	turnResponses := pipelineTurnResponses(turnResponseMessages, historyPositions)
	latestTurnResponseAtMs := int64(0)
	for _, response := range turnResponses {
		if response.Role != "user" && response.RequestedAtMs > latestTurnResponseAtMs {
			latestTurnResponseAtMs = response.RequestedAtMs
		}
	}
	if cursorReader, ok := r.messageService.(messagepkg.TurnResponseCursorReader); ok {
		latest, err := cursorReader.LatestTurnResponseAtBySession(ctx, scope.SessionID)
		if err != nil {
			return pipelineHistoryProjectionBuild{}, err
		}
		if latestMs := latest.UnixMilli(); !latest.IsZero() && latestMs > latestTurnResponseAtMs {
			latestTurnResponseAtMs = latestMs
		}
	}
	return pipelineHistoryProjectionBuild{
		projection: pipelinepkg.ContextHistoryProjection{
			TurnResponses:            turnResponses,
			CompactionArtifacts:      artifacts,
			HistoryMessagePositions:  historyPositions,
			ExternalMessagePositions: externalPositions,
			LatestTurnResponseAtMs:   latestTurnResponseAtMs,
			WindowStartAtMs:          since.UnixMilli(),
		},
		summaryRecords: summaries,
	}, nil
}

func pipelineRenderedMessageIDs(pipeline *pipelinepkg.Pipeline, sessionID string, windowStartAtMs int64) []string {
	if pipeline == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	rendered := pipelinepkg.RecentRenderedContext(pipeline.GetRC(sessionID), windowStartAtMs)
	ids := make([]string, 0, len(rendered))
	seen := make(map[string]struct{}, len(rendered))
	for _, segment := range rendered {
		id := strings.TrimSpace(segment.MessageID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func coveredHistoryMessageIDs(artifacts []pipelinepkg.CompactionArtifact) []string {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, artifact := range artifacts {
		for _, source := range artifact.Sources {
			id := strings.TrimSpace(source.HistoryMessageID)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func pipelineHistoryPositions(messages []messagepkg.Message) (map[string]pipelinepkg.HistoryPosition, map[string]pipelinepkg.HistoryPosition) {
	historyPositions := make(map[string]pipelinepkg.HistoryPosition)
	externalPositions := make(map[string]pipelinepkg.HistoryPosition)
	for _, message := range messages {
		position := pipelinepkg.HistoryPosition{
			TurnPosition:    message.TurnPosition,
			MessageSequence: message.TurnMessageSequence,
		}
		if position.TurnPosition <= 0 || position.MessageSequence <= 0 {
			continue
		}
		if id := strings.TrimSpace(message.ID); id != "" {
			historyPositions[id] = position
		}
		if id := strings.TrimSpace(message.ExternalMessageID); id != "" {
			if _, exists := externalPositions[id]; !exists {
				externalPositions[id] = position
			}
		}
	}
	return historyPositions, externalPositions
}

func pipelineTurnResponses(
	messages []messagepkg.Message,
	historyPositions map[string]pipelinepkg.HistoryPosition,
) []pipelinepkg.TurnResponseEntry {
	responses := make([]pipelinepkg.TurnResponseEntry, 0, len(messages))
	for _, message := range messages {
		entry, ok := pipelinepkg.DecodeTurnResponseEntry(message)
		if ok {
			position := pipelinepkg.HistoryPosition{
				TurnPosition:    message.TurnPosition,
				MessageSequence: message.TurnMessageSequence,
			}
			if position.TurnPosition <= 0 || position.MessageSequence <= 0 {
				position = historyPositions[strings.TrimSpace(message.ID)]
			}
			if entry.Role == "user" && position.MessageSequence <= 1 {
				continue
			}
			entry.HistoryPosition = position
			responses = append(responses, entry)
		}
	}
	return responses
}

func composedPipelineMessages(
	composed *pipelinepkg.ComposeContextResult,
	summaryRecords []historyfrag.HistoryRecord,
) []composedPipelineMessage {
	if composed == nil {
		return nil
	}
	summaries := make(map[string]historyfrag.HistoryRecord, len(summaryRecords))
	for _, record := range summaryRecords {
		summaries[record.Ref.ID] = record
	}
	entries := make([]composedPipelineMessage, 0, len(composed.Messages))
	for _, message := range composed.Messages {
		content := message.RawContent
		if len(content) == 0 {
			content, _ = json.Marshal(message.Content)
		}
		entry := composedPipelineMessage{
			message:            conversation.ModelMessage{Role: message.Role, Content: content},
			renderedMessageIDs: append([]string(nil), message.RenderedMessageIDs...),
		}
		if summary, ok := summaries[message.CompactionArtifactID]; ok {
			entry.summaryRecord = summary
			entry.hasSummary = true
		}
		entries = append(entries, entry)
	}
	return entries
}

func appendCurrentPipelineQueryIfMissing(
	entries []composedPipelineMessage,
	rc pipelinepkg.RenderedContext,
	currentMessageID string,
	currentQuery string,
) []composedPipelineMessage {
	query := strings.TrimSpace(currentQuery)
	if query == "" {
		return entries
	}
	currentMessageID = strings.TrimSpace(currentMessageID)
	if currentMessageID != "" && renderedContextHasMessageID(rc, currentMessageID) {
		for i := len(entries) - 1; i >= 0; i-- {
			if containsString(entries[i].renderedMessageIDs, currentMessageID) {
				entries[i].forceKeep = true
				return entries
			}
		}
	}
	return append(entries, composedPipelineMessage{message: conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(query),
	}, forceKeep: true})
}

func renderedContextHasMessageID(rc pipelinepkg.RenderedContext, messageID string) bool {
	for _, segment := range rc {
		if strings.TrimSpace(segment.MessageID) == messageID {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type composedTrimEntry struct {
	tokens int
	role   string
	keep   bool
}

// composedTrimCutoff finds how many oldest entries exceed maxTokens, keeping
// the newest ones whole and never cutting directly before a tool-result row
// so a tool exchange is not split at the boundary.
func composedTrimCutoff(entries []composedTrimEntry, maxTokens int) int {
	cutoff := 0
	if maxTokens > 0 {
		tokens := 0
		for i := len(entries) - 1; i >= 0; i-- {
			tokens += entries[i].tokens
			if tokens > maxTokens {
				cutoff = i + 1
				break
			}
		}
		for cutoff < len(entries) && strings.EqualFold(strings.TrimSpace(entries[cutoff].role), "tool") {
			cutoff++
		}
	}
	return cutoff
}

// TrimDiscussContext bounds a discuss-composed context to the chat model's
// window with the same semantics as the flow pipeline branch: newest entries
// are kept, artifact summaries survive the dropped prefix, user messages
// selected by the current exact delivery are pinned as triggers, and a
// truncation notice marks the cut. Entries are metered with the flow path's
// estimator so the returned estimate shares its unit.
func (r *Resolver) TrimDiscussContext(
	messages []pipelinepkg.ContextMessage,
	contextTokenBudget int,
	afterCursor int64,
) ([]pipelinepkg.ContextMessage, int) {
	triggered := make([]bool, len(messages))
	hasTrigger := false
	hasEvaluatedUser := false
	for i, message := range messages {
		isUser := strings.TrimSpace(message.CompactionArtifactID) == "" &&
			strings.EqualFold(strings.TrimSpace(message.Role), "user")
		hasEvaluatedUser = hasEvaluatedUser || (isUser && message.CurrentTriggerEvaluated)
		triggered[i] = isUser &&
			(message.CurrentTrigger || (!message.CurrentTriggerEvaluated && message.LatestExternalEventCursor > afterCursor))
		hasTrigger = hasTrigger || triggered[i]
	}
	if !hasTrigger && !hasEvaluatedUser {
		for i := len(messages) - 1; i >= 0; i-- {
			if strings.TrimSpace(messages[i].CompactionArtifactID) == "" &&
				strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
				triggered[i] = true
				break
			}
		}
	}
	entries := make([]composedTrimEntry, len(messages))
	total := 0
	for i, message := range messages {
		entries[i] = composedTrimEntry{
			tokens: estimateMessageTokens(contextMessageForMetering(message)),
			role:   message.Role,
			keep:   strings.TrimSpace(message.CompactionArtifactID) != "" || triggered[i],
		}
		total += entries[i].tokens
	}
	cutoff := composedTrimCutoff(entries, contextTokenBudget)
	if cutoff == 0 {
		return messages, total
	}
	if r.logger != nil {
		r.logger.Info("trim discuss context",
			slog.Int("total_messages", len(messages)),
			slog.Int("max_tokens", contextTokenBudget),
			slog.Int("kept_messages", len(messages)-cutoff),
		)
	}
	notice := historyTruncationNotice()
	out := make([]pipelinepkg.ContextMessage, 0, len(messages)-cutoff+1)
	out = append(out, pipelinepkg.ContextMessage{Role: notice.Role, Content: notice.TextContent()})
	estimated := estimateMessageTokens(notice)
	for i, message := range messages {
		if i < cutoff && !entries[i].keep {
			continue
		}
		out = append(out, message)
		estimated += entries[i].tokens
	}
	return out, estimated
}

func contextMessageForMetering(message pipelinepkg.ContextMessage) conversation.ModelMessage {
	if len(message.RawContent) > 0 {
		return conversation.ModelMessage{Role: message.Role, Content: message.RawContent}
	}
	return conversation.ModelMessage{Role: message.Role, Content: conversation.NewTextContent(message.Content)}
}

func trimComposedPipelineMessages(
	log *slog.Logger,
	entries []composedPipelineMessage,
	maxTokens int,
) pipelineContextBuild {
	trimEntries := make([]composedTrimEntry, len(entries))
	for i, entry := range entries {
		trimEntries[i] = composedTrimEntry{
			tokens: estimateMessageTokens(entry.message),
			role:   entry.message.Role,
			keep:   entry.hasSummary || entry.forceKeep,
		}
	}
	cutoff := composedTrimCutoff(trimEntries, maxTokens)
	if cutoff > 0 && log != nil {
		log.Info("trim pipeline context",
			slog.Int("total_messages", len(entries)),
			slog.Int("max_tokens", maxTokens),
			slog.Int("kept_messages", len(entries)-cutoff),
		)
	}
	retained := make([]composedPipelineMessage, 0, len(entries)-cutoff)
	for _, entry := range entries[:cutoff] {
		if entry.hasSummary || entry.forceKeep {
			retained = append(retained, entry)
		}
	}
	retained = append(retained, entries[cutoff:]...)
	build := pipelineContextBuild{
		Messages:       make([]conversation.ModelMessage, 0, len(retained)+1),
		HistoryRecords: make([]historyfrag.HistoryRecord, 0),
	}
	if cutoff > 0 {
		notice := historyTruncationNotice()
		build.Messages = append(build.Messages, notice)
		build.EstimatedTokens += estimateMessageTokens(notice)
	}
	for _, entry := range retained {
		build.Messages = append(build.Messages, entry.message)
		build.EstimatedTokens += estimateMessageTokens(entry.message)
		if entry.hasSummary {
			build.HistoryRecords = append(build.HistoryRecords, entry.summaryRecord)
		}
	}
	return build
}
