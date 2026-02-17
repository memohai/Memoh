// Package bind provides bind-code issuance and consumption for channel linking.
package bind

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

const (
	defaultTTL      = 24 * time.Hour
	maxTokenRetries = 5
)

// Service manages channel identity->user bind code lifecycle.
type Service struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a bind code service.
func NewService(log *slog.Logger, pool *pgxpool.Pool, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:    pool,
		queries: queries,
		logger:  log.With(slog.String("service", "bind")),
	}
}

// Issue creates a new bind code issued by the given user.
// Platform is optional; when provided, bind consume must happen on the same channel platform.
func (s *Service) Issue(ctx context.Context, issuedByUserID, platform string, ttl time.Duration) (Code, error) {
	if s.queries == nil {
		return Code{}, errors.New("bind queries not configured")
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}

	pgUserID, err := db.ParseUUID(issuedByUserID)
	if err != nil {
		return Code{}, fmt.Errorf("invalid user id: %w", err)
	}
	normalizedPlatform := normalizePlatform(platform)

	expiresAt := time.Now().UTC().Add(ttl)
	for range maxTokenRetries {
		token := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
		row, err := s.queries.CreateBindCode(ctx, sqlc.CreateBindCodeParams{
			Token:          token,
			IssuedByUserID: pgUserID,
			ChannelType: pgtype.Text{
				String: normalizedPlatform,
				Valid:  normalizedPlatform != "",
			},
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err == nil {
			return toCode(row), nil
		}
		if isUniqueViolation(err) {
			continue
		}
		return Code{}, fmt.Errorf("create bind code: %w", err)
	}
	return Code{}, errors.New("create bind code: token collision after retries")
}

// Get looks up a bind code by token.
func (s *Service) Get(ctx context.Context, token string) (Code, error) {
	if s.queries == nil {
		return Code{}, errors.New("bind queries not configured")
	}
	row, err := s.queries.GetBindCode(ctx, strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Code{}, ErrCodeNotFound
		}
		return Code{}, err
	}
	return toCode(row), nil
}

// Consume validates and consumes a bind code and links the channel identity to issuer user.
func (s *Service) Consume(ctx context.Context, code Code, channelIdentityID string) error {
	if s.queries == nil || s.pool == nil {
		return errors.New("bind service not configured")
	}

	// Fast-fail based on caller snapshot before opening a transaction.
	if !code.UsedAt.IsZero() {
		return ErrCodeUsed
	}
	if !code.ExpiresAt.IsZero() && time.Now().UTC().After(code.ExpiresAt) {
		return ErrCodeExpired
	}
	token := strings.TrimSpace(code.Token)
	if token == "" {
		return ErrCodeNotFound
	}
	sourceIdentityID := strings.TrimSpace(channelIdentityID)
	if sourceIdentityID == "" {
		return errors.New("channel identity id is required")
	}
	pgSourceIdentityID, err := db.ParseUUID(sourceIdentityID)
	if err != nil {
		return err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin bind consume tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	lockedCodeRow, err := qtx.GetBindCodeForUpdate(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCodeNotFound
		}
		return fmt.Errorf("lock bind code: %w", err)
	}
	lockedCode := toCode(lockedCodeRow)
	if !lockedCode.UsedAt.IsZero() {
		return ErrCodeUsed
	}
	if !lockedCode.ExpiresAt.IsZero() && time.Now().UTC().After(lockedCode.ExpiresAt) {
		return ErrCodeExpired
	}
	if strings.TrimSpace(code.Platform) != "" && !strings.EqualFold(lockedCode.Platform, strings.TrimSpace(code.Platform)) {
		return ErrCodeMismatch
	}

	targetUserID := strings.TrimSpace(lockedCode.IssuedByUserID)
	if targetUserID == "" {
		return errors.New("bind code issuer user is missing")
	}
	pgTargetUserID, err := db.ParseUUID(targetUserID)
	if err != nil {
		return err
	}

	if _, err := qtx.GetChannelIdentityByIDForUpdate(ctx, pgSourceIdentityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("channel identity not found")
		}
		return fmt.Errorf("lock source identity: %w", err)
	}
	sourceIdentity, err := qtx.GetChannelIdentityByIDForUpdate(ctx, pgSourceIdentityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("channel identity not found")
		}
		return fmt.Errorf("reload source identity: %w", err)
	}
	if sourceIdentity.UserID.Valid && sourceIdentity.UserID.String() != targetUserID {
		return ErrLinkConflict
	}
	if !sourceIdentity.UserID.Valid {
		if _, err := qtx.SetChannelIdentityLinkedUser(ctx, sqlc.SetChannelIdentityLinkedUserParams{
			ID:     pgSourceIdentityID,
			UserID: pgTargetUserID,
		}); err != nil {
			return fmt.Errorf("link channel identity user: %w", err)
		}
	}

	if _, err := qtx.MarkBindCodeUsed(ctx, sqlc.MarkBindCodeUsedParams{
		ID:                      lockedCodeRow.ID,
		UsedByChannelIdentityID: pgSourceIdentityID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCodeUsed
		}
		return fmt.Errorf("mark bind code used: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bind consume tx: %w", err)
	}

	s.logger.Info("bind code consumed",
		slog.String("code_id", lockedCode.ID),
		slog.String("platform", lockedCode.Platform),
		slog.String("channel_identity", sourceIdentityID),
		slog.String("target_user", targetUserID),
	)
	return nil
}

func toCode(row sqlc.ChannelIdentityBindCode) Code {
	c := Code{
		ID:             row.ID.String(),
		Token:          row.Token,
		IssuedByUserID: row.IssuedByUserID.String(),
		CreatedAt:      row.CreatedAt.Time,
	}
	if row.ChannelType.Valid {
		c.Platform = normalizePlatform(row.ChannelType.String)
	}
	if row.ExpiresAt.Valid {
		c.ExpiresAt = row.ExpiresAt.Time
	}
	if row.UsedAt.Valid {
		c.UsedAt = row.UsedAt.Time
	}
	if row.UsedByChannelIdentityID.Valid {
		c.UsedByChannelIdentityID = row.UsedByChannelIdentityID.String()
	}
	return c
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == "" || pgErr.ConstraintName == "channel_identity_bind_codes_token_unique"
}

func normalizePlatform(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
