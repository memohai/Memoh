package models

import (
	"context"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/teams"
)

const OpenSourceDefaultTeamID = teams.DefaultTeamID

var teamContextKeys = []any{
	"memoh.team_id",
	"team_id",
	"teamID",
	"team",
}

// TeamIDFromContext resolves the active tenant scope, then falls back to the
// open-source default team used by bootstrapped provider templates.
func TeamIDFromContext(ctx context.Context) pgtype.UUID {
	if scope, err := teams.ScopeFromContext(ctx); err == nil {
		if teamID, ok := parseTeamID(scope.TeamID); ok {
			return teamID
		}
	}
	for _, key := range teamContextKeys {
		if teamID, ok := teamIDValue(ctx.Value(key)); ok {
			return teamID
		}
	}
	if teamID, ok := parseTeamID(OpenSourceDefaultTeamID); ok {
		return teamID
	}
	return pgtype.UUID{}
}

func teamIDValue(value any) (pgtype.UUID, bool) {
	switch v := value.(type) {
	case pgtype.UUID:
		return v, v.Valid
	case uuid.UUID:
		return pgtype.UUID{Bytes: v, Valid: true}, true
	case string:
		return parseTeamID(v)
	case interface{ String() string }:
		return parseTeamID(v.String())
	default:
		return pgtype.UUID{}, false
	}
}

func parseTeamID(raw string) (pgtype.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.UUID{}, false
	}
	teamID, err := db.ParseUUID(raw)
	return teamID, err == nil && teamID.Valid
}

func SetTeamIDParam(arg any, teamID pgtype.UUID) {
	value := reflect.ValueOf(arg)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	setStructField(elem, "TeamID", teamID)
}

func InvokeTeamQuery[T any](
	ctx context.Context,
	target any,
	method string,
	fields map[string]any,
	fallback func() (T, error),
) (T, error) {
	var zero T
	methodValue := reflect.ValueOf(target).MethodByName(method)
	if !methodValue.IsValid() {
		return fallback()
	}
	methodType := methodValue.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
		return fallback()
	}
	paramType := methodType.In(1)
	if paramType.Kind() != reflect.Struct {
		return fallback()
	}
	param := reflect.New(paramType).Elem()
	setStructField(param, "TeamID", TeamIDFromContext(ctx))
	for name, value := range fields {
		setStructField(param, name, value)
	}
	out := methodValue.Call([]reflect.Value{reflect.ValueOf(ctx), param})
	if !out[1].IsNil() {
		return zero, out[1].Interface().(error)
	}
	result, ok := out[0].Interface().(T)
	if !ok {
		return fallback()
	}
	return result, nil
}

func InvokeTeamExec(
	ctx context.Context,
	target any,
	method string,
	fields map[string]any,
	fallback func() error,
) error {
	methodValue := reflect.ValueOf(target).MethodByName(method)
	if !methodValue.IsValid() {
		return fallback()
	}
	methodType := methodValue.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		return fallback()
	}
	paramType := methodType.In(1)
	if paramType.Kind() != reflect.Struct {
		return fallback()
	}
	param := reflect.New(paramType).Elem()
	setStructField(param, "TeamID", TeamIDFromContext(ctx))
	for name, value := range fields {
		setStructField(param, name, value)
	}
	out := methodValue.Call([]reflect.Value{reflect.ValueOf(ctx), param})
	if out[0].IsNil() {
		return nil
	}
	return out[0].Interface().(error)
}

func setStructField(target reflect.Value, name string, value any) {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() || value == nil {
		return
	}
	source := reflect.ValueOf(value)
	if source.Type().AssignableTo(field.Type()) {
		field.Set(source)
		return
	}
	if source.Type().ConvertibleTo(field.Type()) {
		field.Set(source.Convert(field.Type()))
	}
}
