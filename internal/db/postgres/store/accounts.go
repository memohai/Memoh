package postgresstore

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func (s *Store) CountAccounts(ctx context.Context) (int64, error) {
	return s.queries.CountAccounts(ctx)
}

func (s *Store) GetByUserID(ctx context.Context, userID string) (dbstore.AccountRecord, error) {
	id, err := db.ParseUUID(userID)
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	row, err := s.queries.GetAccountByUserID(ctx, id)
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return accountRecord(row), nil
}

func (s *Store) GetByIdentity(ctx context.Context, identity string) (dbstore.AccountRecord, error) {
	row, err := s.queries.GetAccountByIdentity(ctx, pgtype.Text{String: identity, Valid: identity != ""})
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return accountRecord(row), nil
}

func (s *Store) List(ctx context.Context) ([]dbstore.AccountRecord, error) {
	rows, err := s.queries.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return accountRecords(rows), nil
}

func (s *Store) Search(ctx context.Context, query string, limit int32) ([]dbstore.AccountRecord, error) {
	rows, err := s.queries.SearchAccounts(ctx, dbsqlc.SearchAccountsParams{
		Query:      query,
		LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	return accountRecords(rows), nil
}

func (s *Store) CreateUser(ctx context.Context, input dbstore.CreateUserInput) (dbstore.AccountRecord, error) {
	row, err := s.queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		IsActive: input.IsActive,
		Metadata: input.Metadata,
	})
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	return accountRecord(row), nil
}

func (s *Store) CreateAccount(ctx context.Context, input dbstore.CreateAccountInput) (dbstore.AccountRecord, error) {
	userID, err := db.ParseUUID(input.UserID)
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	row, err := s.queries.CreateAccount(ctx, dbsqlc.CreateAccountParams{
		UserID:       userID,
		Username:     text(input.Username),
		Email:        optionalText(input.Email),
		PasswordHash: text(input.PasswordHash),
		Role:         input.Role,
		DisplayName:  optionalText(input.DisplayName),
		AvatarUrl:    optionalText(input.AvatarURL),
		IsActive:     input.IsActive,
		DataRoot:     optionalText(input.DataRoot),
	})
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	return accountRecord(row), nil
}

func (s *Store) UpdateLastLogin(ctx context.Context, accountID string) error {
	id, err := db.ParseUUID(accountID)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateAccountLastLogin(ctx, id)
	return err
}

func (s *Store) UpdateAdmin(ctx context.Context, input dbstore.UpdateAccountAdminInput) (dbstore.AccountRecord, error) {
	userID, err := db.ParseUUID(input.UserID)
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	row, err := s.queries.UpdateAccountAdmin(ctx, dbsqlc.UpdateAccountAdminParams{
		UserID:      userID,
		Role:        input.Role,
		DisplayName: optionalText(input.DisplayName),
		AvatarUrl:   optionalText(input.AvatarURL),
		IsActive:    input.IsActive,
	})
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return accountRecord(row), nil
}

func (s *Store) UpdateProfile(ctx context.Context, input dbstore.UpdateAccountProfileInput) (dbstore.AccountRecord, error) {
	userID, err := db.ParseUUID(input.UserID)
	if err != nil {
		return dbstore.AccountRecord{}, err
	}
	row, err := s.queries.UpdateAccountProfile(ctx, dbsqlc.UpdateAccountProfileParams{
		ID:          userID,
		DisplayName: optionalText(input.DisplayName),
		AvatarUrl:   optionalText(input.AvatarURL),
		Timezone:    input.Timezone,
		IsActive:    input.IsActive,
	})
	if err != nil {
		return dbstore.AccountRecord{}, mapQueryErr(err)
	}
	return accountRecord(row), nil
}

func (s *Store) UpdatePassword(ctx context.Context, input dbstore.UpdateAccountPasswordInput) error {
	userID, err := db.ParseUUID(input.UserID)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateAccountPassword(ctx, dbsqlc.UpdateAccountPasswordParams{
		ID:           userID,
		PasswordHash: text(input.PasswordHash),
	})
	return mapQueryErr(err)
}

func mapQueryErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	return err
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func accountRecords(rows []dbsqlc.User) []dbstore.AccountRecord {
	items := make([]dbstore.AccountRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, accountRecord(row))
	}
	return items
}

func accountRecord(row dbsqlc.User) dbstore.AccountRecord {
	rec := dbstore.AccountRecord{
		ID:              row.ID.String(),
		Username:        row.Username.String,
		Email:           row.Email.String,
		Role:            row.Role,
		DisplayName:     row.DisplayName.String,
		AvatarURL:       row.AvatarUrl.String,
		Timezone:        row.Timezone,
		PasswordHash:    row.PasswordHash.String,
		HasPasswordHash: row.PasswordHash.Valid,
		IsActive:        row.IsActive,
	}
	if row.CreatedAt.Valid {
		rec.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		rec.UpdatedAt = row.UpdatedAt.Time
	}
	if row.LastLoginAt.Valid {
		rec.LastLoginAt = row.LastLoginAt.Time
	}
	return rec
}
