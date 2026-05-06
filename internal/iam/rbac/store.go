package rbac

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type SQLStore struct {
	queries dbstore.Queries
}

func NewSQLStore(queries dbstore.Queries) *SQLStore {
	return &SQLStore{queries: queries}
}

func (s *SQLStore) HasPermission(ctx context.Context, check Check) (bool, error) {
	userID, err := db.ParseUUID(check.UserID)
	if err != nil {
		return false, err
	}
	var resourceID pgtype.UUID
	if check.ResourceType != ResourceSystem && check.ResourceID != "" {
		resourceID, err = db.ParseUUID(check.ResourceID)
		if err != nil {
			return false, err
		}
	}
	return s.queries.HasPermission(ctx, sqlc.HasPermissionParams{
		PermissionKey: string(check.PermissionKey),
		ResourceType:  string(check.ResourceType),
		ResourceID:    resourceID,
		UserID:        userID,
	})
}

func (s *SQLStore) HasSystemAdmin(ctx context.Context, userID string) (bool, error) {
	return s.HasPermission(ctx, Check{
		UserID:        userID,
		PermissionKey: PermissionSystemAdmin,
		ResourceType:  ResourceSystem,
	})
}
