package botbackup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const historyEventDedupMigrationMarker = "_migration_0116_history_event_dedup"

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

func stripRestoredHistoryEventDedupMarker(item sqlc.ListAllMessagesForBackupRow) ([]byte, error) {
	raw := item.Metadata
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(defaultJSONMap(raw), &metadata); err != nil || metadata == nil {
		return raw, nil
	}
	marker, exists := metadata[historyEventDedupMigrationMarker]
	if !exists {
		return raw, nil
	}
	if err := validateRestoredHistoryEventDedupMarker(item, marker); err != nil {
		return nil, fmt.Errorf(
			"reserved 0116 history event dedup metadata marker is invalid for message %s: %w",
			item.ID.String(),
			err,
		)
	}
	delete(metadata, historyEventDedupMigrationMarker)
	cleaned, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("rebuild history message metadata: %w", err)
	}
	return cleaned, nil
}

func validateRestoredHistoryEventDedupMarker(item sqlc.ListAllMessagesForBackupRow, raw json.RawMessage) error {
	if item.EventID.Valid {
		return errors.New("archived message still has an event link")
	}
	if !item.ID.Valid {
		return errors.New("archived message id is invalid")
	}
	var marker struct {
		Version   json.RawMessage `json:"version"`
		MessageID string          `json:"message_id"`
		EventID   string          `json:"event_id"`
	}
	if err := json.Unmarshal(raw, &marker); err != nil {
		return errors.New("marker must be an object")
	}
	version := strings.TrimSpace(string(marker.Version))
	if version != "1" && version != `"1"` {
		return errors.New("marker version must be 1")
	}
	if marker.MessageID != item.ID.String() {
		return errors.New("marker message_id does not match the archived message")
	}
	eventID, err := uuid.Parse(marker.EventID)
	if err != nil || !strings.EqualFold(eventID.String(), marker.EventID) {
		return errors.New("marker event_id is not a valid UUID")
	}
	return nil
}
