// Package preauth provides pre-authentication token issuance and validation.
package preauth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// ErrKeyNotFound is returned when a preauth key lookup by token finds no row.
var ErrKeyNotFound = errors.New("preauth key not found")

// Service issues and validates preauth keys for bot access.
type Service struct {
	queries *sqlc.Queries
}

// NewService creates a preauth service using the given queries.
func NewService(queries *sqlc.Queries) *Service {
	return &Service{queries: queries}
}

// Issue creates a new preauth key for the given bot.
func (s *Service) Issue(ctx context.Context, botID, issuedByUserID string, ttl time.Duration) (Key, error) {
	if s.queries == nil {
		return Key{}, errors.New("preauth queries not configured")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Key{}, err
	}
	pgIssuedBy := pgtype.UUID{Valid: false}
	if strings.TrimSpace(issuedByUserID) != "" {
		parsed, err := db.ParseUUID(issuedByUserID)
		if err != nil {
			return Key{}, err
		}
		pgIssuedBy = parsed
	}
	token := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	expiresAt := time.Now().UTC().Add(ttl)
	row, err := s.queries.CreateBotPreauthKey(ctx, sqlc.CreateBotPreauthKeyParams{
		BotID:          pgBotID,
		Token:          token,
		IssuedByUserID: pgIssuedBy,
		ExpiresAt:      pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Key{}, err
	}
	return normalizeKey(row), nil
}

// Get returns the preauth key for the given token, or ErrKeyNotFound if not found.
func (s *Service) Get(ctx context.Context, token string) (Key, error) {
	if s.queries == nil {
		return Key{}, errors.New("preauth queries not configured")
	}
	row, err := s.queries.GetBotPreauthKey(ctx, strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Key{}, ErrKeyNotFound
		}
		return Key{}, err
	}
	return normalizeKey(row), nil
}

// MarkUsed marks the preauth key by ID as used and returns the updated key.
func (s *Service) MarkUsed(ctx context.Context, id string) (Key, error) {
	if s.queries == nil {
		return Key{}, errors.New("preauth queries not configured")
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Key{}, err
	}
	row, err := s.queries.MarkBotPreauthKeyUsed(ctx, pgID)
	if err != nil {
		return Key{}, err
	}
	return normalizeKey(row), nil
}

func normalizeKey(row sqlc.BotPreauthKey) Key {
	return Key{
		ID:             row.ID.String(),
		BotID:          row.BotID.String(),
		Token:          strings.TrimSpace(row.Token),
		IssuedByUserID: row.IssuedByUserID.String(),
		ExpiresAt:      timeFromPg(row.ExpiresAt),
		UsedAt:         timeFromPg(row.UsedAt),
		CreatedAt:      timeFromPg(row.CreatedAt),
	}
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
