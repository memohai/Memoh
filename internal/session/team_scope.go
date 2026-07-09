package session

import (
	"context"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/teams"
)

func teamIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	return dbpkg.ParseUUID(teams.ScopeOrDefault(ctx).TeamID)
}

func withTeamID[T any](params T, teamID pgtype.UUID) T {
	applyTeamID(&params, teamID)
	return params
}

func applyTeamID(params any, teamID pgtype.UUID) {
	value := reflect.ValueOf(params)
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
