package teams

import (
	"context"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
)

// TeamUUID resolves the active team scope from the context and parses it into a
// pgtype.UUID. It returns the db.ParseUUID error unmodified so callers can wrap
// it with their own context if desired.
func TeamUUID(ctx context.Context) (pgtype.UUID, error) {
	return db.ParseUUID(strings.TrimSpace(ScopeOrDefault(ctx).TeamID))
}

// TeamUUIDOrZero resolves the active team scope, returning an invalid (zero)
// UUID when the scope cannot be parsed.
func TeamUUIDOrZero(ctx context.Context) pgtype.UUID {
	id, err := TeamUUID(ctx)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}

// WithTeamID sets the TeamID field on a copy of params (if present) and returns
// the copy, so it can be used inline when building sqlc query parameters.
func WithTeamID[T any](params T, teamID pgtype.UUID) T {
	ApplyTeamID(&params, teamID)
	return params
}

// ApplyTeamID sets the TeamID field of the struct pointed to by ptr to teamID.
// It is a no-op when ptr is not a non-nil pointer to a struct with a settable
// pgtype.UUID TeamID field.
func ApplyTeamID(ptr any, teamID pgtype.UUID) {
	value := reflect.ValueOf(ptr)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field := elem.FieldByName("TeamID")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeOf(pgtype.UUID{}) {
		return
	}
	field.Set(reflect.ValueOf(teamID))
}
