package message

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
)

type pendingDeliveryQueries interface {
	CompletePendingHistoryDelivery(ctx context.Context, messageID pgtype.UUID) (int64, error)
}

func (s *DBService) CompletePendingDelivery(ctx context.Context, messageID string) error {
	if s == nil || s.queries == nil {
		return errors.New("message service is not configured")
	}
	pendingQueries, ok := s.queries.(pendingDeliveryQueries)
	if !ok {
		return errors.New("message store does not support pending delivery completion")
	}
	parsedID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return fmt.Errorf("invalid pending delivery message id: %w", err)
	}
	updated, err := pendingQueries.CompletePendingHistoryDelivery(ctx, parsedID)
	if err != nil {
		return fmt.Errorf("complete pending history delivery: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("complete pending history delivery updated %d messages", updated)
	}
	return nil
}
