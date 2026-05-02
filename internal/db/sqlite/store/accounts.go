package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/db"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func (s *Store) CountAccounts(ctx context.Context) (int64, error) {
	return s.queries.CountAccounts(ctx)
}

func (s *Store) GetByUserID(ctx context.Context, userID string) (dbstore.AccountRecord, error) {
	row, err := s.queries.GetAccountByUserID(ctx, userID)
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return s.accountRecord(ctx, row), nil
}

func (s *Store) GetByIdentity(ctx context.Context, identity string) (dbstore.AccountRecord, error) {
	row, err := s.queries.GetAccountByIdentity(ctx, identity)
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return s.accountRecord(ctx, row), nil
}

func (s *Store) List(ctx context.Context) ([]dbstore.AccountRecord, error) {
	rows, err := s.queries.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return s.accountRecords(ctx, rows), nil
}

func (s *Store) Search(ctx context.Context, query string, limit int32) ([]dbstore.AccountRecord, error) {
	rows, err := s.queries.SearchAccounts(ctx, sqlitesqlc.SearchAccountsParams{
		Query:      query,
		LimitCount: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	return s.accountRecords(ctx, rows), nil
}

func (s *Store) CreateUser(ctx context.Context, input dbstore.CreateUserInput) (dbstore.AccountRecord, error) {
	row, err := s.queries.CreateUser(ctx, sqlitesqlc.CreateUserParams{
		IsActive: boolInt(input.IsActive),
		Metadata: string(input.Metadata),
	})
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	return baseAccountRecord(row), nil
}

func (s *Store) CreateAccount(ctx context.Context, input dbstore.CreateAccountInput) (dbstore.AccountRecord, error) {
	row, err := s.queries.CreateAccount(ctx, sqlitesqlc.CreateAccountParams{
		UserID:      input.UserID,
		Username:    nullable(input.Username),
		Email:       nullable(input.Email),
		DisplayName: nullable(input.DisplayName),
		AvatarUrl:   nullable(input.AvatarURL),
		IsActive:    boolInt(input.IsActive),
		DataRoot:    nullable(input.DataRoot),
	})
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	_, err = s.queries.CreatePasswordIdentity(ctx, sqlitesqlc.CreatePasswordIdentityParams{
		UserID:           input.UserID,
		Subject:          input.Username,
		CredentialSecret: nullable(input.PasswordHash),
		Email:            nullable(input.Email),
		Username:         nullable(input.Username),
		DisplayName:      nullable(input.DisplayName),
		AvatarUrl:        nullable(input.AvatarURL),
	})
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	return s.accountRecord(ctx, row), nil
}

func (s *Store) UpdateLastLogin(ctx context.Context, accountID string) error {
	_, err := s.queries.UpdateAccountLastLogin(ctx, accountID)
	return mapQueryErr(err)
}

func (s *Store) UpdateAdmin(ctx context.Context, input dbstore.UpdateAccountAdminInput) (dbstore.AccountRecord, error) {
	row, err := s.queries.UpdateAccountAdmin(ctx, sqlitesqlc.UpdateAccountAdminParams{
		UserID:      input.UserID,
		DisplayName: nullable(input.DisplayName),
		AvatarUrl:   nullable(input.AvatarURL),
		IsActive:    boolInt(input.IsActive),
	})
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return s.accountRecord(ctx, row), nil
}

func (s *Store) UpdateProfile(ctx context.Context, input dbstore.UpdateAccountProfileInput) (dbstore.AccountRecord, error) {
	row, err := s.queries.UpdateAccountProfile(ctx, sqlitesqlc.UpdateAccountProfileParams{
		ID:          input.UserID,
		DisplayName: nullable(input.DisplayName),
		AvatarUrl:   nullable(input.AvatarURL),
		Timezone:    input.Timezone,
		IsActive:    boolInt(input.IsActive),
	})
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return s.accountRecord(ctx, row), nil
}

func (s *Store) UpdatePassword(ctx context.Context, input dbstore.UpdateAccountPasswordInput) error {
	_, err := s.queries.UpdateAccountPassword(ctx, sqlitesqlc.UpdateAccountPasswordParams{
		UserID:       input.UserID,
		PasswordHash: nullable(input.PasswordHash),
	})
	return mapQueryErr(err)
}

func mapQueryErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Join(db.ErrNotFound, pgx.ErrNoRows)
	}
	return err
}

func nullable(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (s *Store) accountRecords(ctx context.Context, rows []sqlitesqlc.IamUser) []dbstore.AccountRecord {
	items := make([]dbstore.AccountRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.accountRecord(ctx, row))
	}
	return items
}

func (s *Store) accountRecord(ctx context.Context, row sqlitesqlc.IamUser) dbstore.AccountRecord {
	rec := baseAccountRecord(row)
	if row.Username.Valid {
		identity, err := s.queries.GetPasswordIdentityBySubject(ctx, row.Username.String)
		if err == nil {
			rec.PasswordHash = identity.CredentialSecret.String
			rec.HasPasswordHash = identity.CredentialSecret.Valid
		}
	}
	return rec
}

func baseAccountRecord(row sqlitesqlc.IamUser) dbstore.AccountRecord {
	return dbstore.AccountRecord{
		ID:          row.ID,
		Username:    row.Username.String,
		Email:       row.Email.String,
		DisplayName: row.DisplayName.String,
		AvatarURL:   row.AvatarUrl.String,
		Timezone:    row.Timezone,
		IsActive:    row.IsActive != 0,
		CreatedAt:   parseTime(row.CreatedAt),
		UpdatedAt:   parseTime(row.UpdatedAt),
		LastLoginAt: parseTime(row.LastLoginAt.String),
	}
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
