package botbackup

import (
	"bytes"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const historyEventDedupMigrationMarker = "_migration_0115_history_event_dedup"

func restoredHistoryEventLinkWinners(messages []sqlc.ListAllMessagesForBackupRow, eventMap map[string]pgtype.UUID) []bool {
	winnerByEvent := make(map[string]int)
	for i, item := range messages {
		if !item.EventID.Valid {
			continue
		}
		eventID := eventMap[item.EventID.String()]
		if !eventID.Valid {
			continue
		}
		key := eventID.String()
		winner, exists := winnerByEvent[key]
		if !exists || restoredHistoryMessagePrecedes(item, messages[winner]) {
			winnerByEvent[key] = i
		}
	}
	winners := make([]bool, len(messages))
	for _, winner := range winnerByEvent {
		winners[winner] = true
	}
	return winners
}

func restoredHistoryMessagePrecedes(left, right sqlc.ListAllMessagesForBackupRow) bool {
	if left.CreatedAt.Valid != right.CreatedAt.Valid {
		return left.CreatedAt.Valid
	}
	if left.CreatedAt.Valid && !left.CreatedAt.Time.Equal(right.CreatedAt.Time) {
		return left.CreatedAt.Time.Before(right.CreatedAt.Time)
	}
	if left.ID.Valid != right.ID.Valid {
		return left.ID.Valid
	}
	return bytes.Compare(left.ID.Bytes[:], right.ID.Bytes[:]) < 0
}

func stripRestoredHistoryEventDedupMarker(raw []byte) []byte {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(defaultJSONMap(raw), &metadata); err != nil || metadata == nil {
		return raw
	}
	if _, exists := metadata[historyEventDedupMigrationMarker]; !exists {
		return raw
	}
	delete(metadata, historyEventDedupMigrationMarker)
	cleaned, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return cleaned
}
