package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type DBAuthSessionStore struct {
	queries dbstore.Queries
}

func NewDBAuthSessionStore(queries dbstore.Queries) *DBAuthSessionStore {
	return &DBAuthSessionStore{queries: queries}
}

func (s *DBAuthSessionStore) CreateSession(ctx context.Context, input AuthSessionInput) (AuthSession, error) {
	userID, err := db.ParseUUID(input.UserID)
	if err != nil {
		return AuthSession{}, err
	}
	var identityID pgtype.UUID
	if input.IdentityID != "" {
		identityID, err = db.ParseUUID(input.IdentityID)
		if err != nil {
			return AuthSession{}, err
		}
	}
	row, err := s.queries.CreateIAMSession(ctx, sqlc.CreateIAMSessionParams{
		UserID:     userID,
		IdentityID: identityID,
		ExpiresAt:  pgtype.Timestamptz{Time: input.ExpiresAt.UTC(), Valid: true},
		IpAddress:  pgtype.Text{String: input.IPAddress, Valid: input.IPAddress != ""},
		UserAgent:  pgtype.Text{String: input.UserAgent, Valid: input.UserAgent != ""},
		Metadata:   []byte("{}"),
	})
	if err != nil {
		return AuthSession{}, err
	}
	return AuthSession{
		ID:        row.ID.String(),
		UserID:    row.UserID.String(),
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

func (s *DBAuthSessionStore) ValidateSession(ctx context.Context, userID string, sessionID string) error {
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return err
	}
	row, err := s.queries.GetIAMSessionByID(ctx, pgSessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrNotFound) {
			return db.ErrNotFound
		}
		return err
	}
	if row.UserID.String() != userID {
		return db.ErrNotFound
	}
	if !row.UserIsActive || row.RevokedAt.Valid || !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now().UTC()) {
		return db.ErrNotFound
	}
	return nil
}

func (s *DBAuthSessionStore) RevokeSession(ctx context.Context, sessionID string) error {
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return err
	}
	return s.queries.RevokeIAMSession(ctx, pgSessionID)
}
