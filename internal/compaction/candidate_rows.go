package compaction

import (
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// itemsFromRows classifies each uncompacted row into a typed CompactionCandidate.
// A row that cannot be classified remains as a must-keep barrier rather than
// aborting the whole compaction. Keeping its position prevents compact spans on
// either side from sharing an ID and being reordered by the read path.
func itemsFromRows(rows []sqlc.ListUncompactedMessagesBySessionRow) ([]CompactionCandidate, int) {
	items := make([]CompactionCandidate, 0, len(rows))
	barrierCount := 0
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(rowToMessage(row), rowScopeFallback(row))
		if err != nil {
			barrierCount++
			preserveToolClosure, toolResult := rawToolShape(row)
			policies := []CompactPolicy{CompactPolicyMustKeep}
			if preserveToolClosure {
				policies = appendPolicy(policies, CompactPolicyPreserveToolClosure)
			}
			items = append(items, CompactionCandidate{
				ID:           row.ID,
				RawContent:   row.Content,
				RawUsage:     row.Usage,
				Policies:     policies,
				IsToolResult: toolResult,
			})
			continue
		}
		items = append(items, CompactionCandidate{
			ID:           row.ID,
			RawContent:   row.Content,
			RawUsage:     row.Usage,
			Record:       record,
			Policies:     candidatePolicies(record),
			IsToolResult: strings.EqualFold(strings.TrimSpace(record.ModelMessage.Role), "tool"),
		})
	}
	if len(items) > 0 {
		propagateMustKeepAcrossToolExchanges(items)
		markSelectionPolicies(items)
	}
	return items, barrierCount
}

func rawToolShape(row sqlc.ListUncompactedMessagesBySessionRow) (preserveToolClosure, toolResult bool) {
	toolResult = strings.EqualFold(strings.TrimSpace(row.Role), "tool")
	if toolResult {
		return true, true
	}

	content := row.Content
	var modelMessage conversation.ModelMessage
	if json.Unmarshal(row.Content, &modelMessage) == nil {
		if len(modelMessage.ToolCalls) > 0 || strings.TrimSpace(modelMessage.ToolCallID) != "" {
			return true, false
		}
		if modelMessage.HasContent() {
			content = modelMessage.Content
		}
	}
	var barePart entryPart
	if json.Unmarshal(content, &barePart) == nil && isToolPartType(barePart.Type) {
		return true, false
	}
	for _, part := range parseEntryParts(content) {
		if isToolPartType(part.Type) {
			return true, false
		}
	}
	return false, false
}

func isToolPartType(value string) bool {
	return strings.Contains(value, "tool-call") || strings.Contains(value, "tool_call") ||
		strings.Contains(value, "tool-result") || strings.Contains(value, "tool_result")
}

func rowToMessage(row sqlc.ListUncompactedMessagesBySessionRow) messagepkg.Message {
	return messagepkg.Message{
		ID:                      formatUUID(row.ID),
		BotID:                   formatUUID(row.BotID),
		SessionID:               formatUUID(row.SessionID),
		SenderChannelIdentityID: formatUUID(row.SenderChannelIdentityID),
		SenderUserID:            formatUUID(row.SenderUserID),
		SenderDisplayName:       textValue(row.SenderDisplayName),
		SenderAvatarURL:         textValue(row.SenderAvatarUrl),
		Platform:                textValue(row.Platform),
		ExternalMessageID:       textValue(row.ExternalMessageID),
		SourceReplyToMessageID:  textValue(row.SourceReplyToMessageID),
		Role:                    row.Role,
		Content:                 row.Content,
		Metadata:                metadataMap(row.Metadata),
		Usage:                   row.Usage,
		CompactID:               formatUUID(row.CompactID),
		EventID:                 formatUUID(row.EventID),
		DisplayContent:          textValue(row.DisplayText),
		CreatedAt:               row.CreatedAt.Time,
	}
}

func rowScopeFallback(row sqlc.ListUncompactedMessagesBySessionRow) historyfrag.ScopeFallback {
	return historyfrag.ScopeFallback{
		ConversationType: textValue(row.ConversationType),
		ConversationName: strings.TrimSpace(row.ConversationName),
		ReplyTarget:      textValue(row.ReplyTarget),
	}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func metadataMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
