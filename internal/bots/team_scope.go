package bots

import (
	"context"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/teams"
)

func teamIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	return db.ParseUUID(teams.ScopeOrDefault(ctx).TeamID)
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
