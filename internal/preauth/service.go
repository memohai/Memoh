package preauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

var ErrKeyNotFound = errors.New("preauth key not found")

type Service struct {
	queries *sqlc.Queries
}

func NewService(queries *sqlc.Queries) *Service {
	return &Service{queries: queries}
}

func (s *Service) Issue(ctx context.Context, botID, issuedByUserID string, ttl time.Duration) (Key, error) {
	if s.queries == nil {
		return Key{}, fmt.Errorf("preauth queries not configured")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Key{}, err
	}
	pgIssuedBy := pgtype.UUID{Valid: false}
	if strings.TrimSpace(issuedByUserID) != "" {
		parsed, err := parseUUID(issuedByUserID)
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

func (s *Service) Get(ctx context.Context, token string) (Key, error) {
	if s.queries == nil {
		return Key{}, fmt.Errorf("preauth queries not configured")
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

func (s *Service) MarkUsed(ctx context.Context, id string) (Key, error) {
	if s.queries == nil {
		return Key{}, fmt.Errorf("preauth queries not configured")
	}
	pgID, err := parseUUID(id)
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
		ID:             toUUIDString(row.ID),
		BotID:          toUUIDString(row.BotID),
		Token:          strings.TrimSpace(row.Token),
		IssuedByUserID: toUUIDString(row.IssuedByUserID),
		ExpiresAt:      timeFromPg(row.ExpiresAt),
		UsedAt:         timeFromPg(row.UsedAt),
		CreatedAt:      timeFromPg(row.CreatedAt),
	}
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

func toUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
