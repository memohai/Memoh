package compaction

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// compactionItem is the typed view of one uncompacted history row used during
// candidate selection. content and usage retain the raw row payload so token
// estimation stays byte-identical to the legacy path; record carries the typed
// classifier output used for tool-aware boundaries and summarizer rendering.
type compactionItem struct {
	id      pgtype.UUID
	content []byte
	usage   []byte
	record  historyfrag.HistoryRecord
}

func itemsFromRows(rows []sqlc.ListUncompactedMessagesBySessionRow) ([]compactionItem, error) {
	items := make([]compactionItem, 0, len(rows))
	for _, row := range rows {
		record, err := historyfrag.FromDBMessage(rowToMessage(row), historyfrag.ScopeFallback{})
		if err != nil {
			return nil, err
		}
		items = append(items, compactionItem{
			id:      row.ID,
			content: row.Content,
			usage:   row.Usage,
			record:  record,
		})
	}
	return items, nil
}

func rowToMessage(row sqlc.ListUncompactedMessagesBySessionRow) messagepkg.Message {
	return messagepkg.Message{
		ID:                      formatUUID(row.ID),
		BotID:                   formatUUID(row.BotID),
		SessionID:               formatUUID(row.SessionID),
		SenderChannelIdentityID: formatUUID(row.SenderChannelIdentityID),
		Role:                    row.Role,
		Content:                 row.Content,
		Usage:                   row.Usage,
		CompactID:               formatUUID(row.CompactID),
		CreatedAt:               row.CreatedAt.Time,
	}
}

type usagePayload struct {
	OutputTokens *int `json:"output_tokens"`
}

func estimateItemTokens(item compactionItem) int {
	if len(item.usage) > 0 {
		var u usagePayload
		if json.Unmarshal(item.usage, &u) == nil && u.OutputTokens != nil && *u.OutputTokens > 0 {
			return *u.OutputTokens
		}
	}
	return len(item.content) / 4
}

// splitByRatio splits items so that roughly the first ratio% (by token weight)
// are returned for compaction, and the rest are kept as-is.
func splitByRatio(items []compactionItem, totalInputTokens, ratio int) []compactionItem {
	if ratio <= 0 || totalInputTokens <= 0 || len(items) == 0 {
		return nil
	}
	if ratio >= 100 {
		return items
	}

	keepTokens := totalInputTokens * (100 - ratio) / 100
	if keepTokens <= 0 {
		return items
	}

	accumulated := 0
	cutoff := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateItemTokens(items[i])
		if accumulated >= keepTokens {
			cutoff = i + 1
			break
		}
	}

	if cutoff <= 0 {
		return nil
	}
	if cutoff >= len(items) {
		return items
	}
	return items[:cutoff]
}

// splitByTarget returns the oldest items to compact so that the remaining newest
// items fit within targetTokens. Used by synchronous compaction.
func splitByTarget(items []compactionItem, targetTokens int) []compactionItem {
	if targetTokens <= 0 || len(items) == 0 {
		return nil
	}
	accumulated := 0
	cutoff := 0
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateItemTokens(items[i])
		if accumulated > targetTokens {
			cutoff = i + 1
			break
		}
	}
	if cutoff <= 0 {
		return nil
	}
	return items[:cutoff]
}

// trimCompactMessages trims the compaction input from the tail (oldest) so the
// total estimated tokens stay within maxTokens.
func trimCompactMessages(items []compactionItem, maxTokens int) []compactionItem {
	if len(items) == 0 || maxTokens <= 0 {
		return items
	}
	total := 0
	for _, it := range items {
		total += estimateItemTokens(it)
	}
	if total <= maxTokens {
		return items
	}
	accumulated := 0
	cutoff := len(items)
	for i := len(items) - 1; i >= 0; i-- {
		accumulated += estimateItemTokens(items[i])
		if accumulated > maxTokens {
			cutoff = i + 1
			break
		}
	}
	if cutoff >= len(items) {
		return items
	}
	return items[cutoff:]
}
