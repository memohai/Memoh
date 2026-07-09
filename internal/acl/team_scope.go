package acl

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func applyUUIDField(params any, name string, id pgtype.UUID) {
	value := reflect.ValueOf(params)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	elem := value.Elem()
	if elem.Kind() != reflect.Struct {
		return
	}
	field := elem.FieldByName(name)
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeOf(pgtype.UUID{}) {
		return
	}
	field.Set(reflect.ValueOf(id))
}

func getChannelIdentityByID(ctx context.Context, queries dbstore.Queries, teamID, id pgtype.UUID) (sqlc.ChannelIdentity, error) {
	values, err := callTeamScopedQuery(ctx, queries, "GetChannelIdentityByID", teamID, map[string]reflect.Value{
		"ID": reflect.ValueOf(id),
	}, reflect.ValueOf(id))
	if err != nil {
		return sqlc.ChannelIdentity{}, err
	}
	row, _ := values[0].Interface().(sqlc.ChannelIdentity)
	return row, errorFromValue(values[1])
}

func getBotACLDefaultEffect(ctx context.Context, queries dbstore.Queries, teamID, id pgtype.UUID) (string, error) {
	values, err := callTeamScopedQuery(ctx, queries, "GetBotACLDefaultEffect", teamID, map[string]reflect.Value{
		"ID": reflect.ValueOf(id),
	}, reflect.ValueOf(id))
	if err != nil {
		return "", err
	}
	effect, _ := values[0].Interface().(string)
	return effect, errorFromValue(values[1])
}

func listBotACLRules(ctx context.Context, queries dbstore.Queries, teamID, botID pgtype.UUID) ([]sqlc.ListBotACLRulesRow, error) {
	values, err := callTeamScopedQuery(ctx, queries, "ListBotACLRules", teamID, map[string]reflect.Value{
		"BotID": reflect.ValueOf(botID),
	}, reflect.ValueOf(botID))
	if err != nil {
		return nil, err
	}
	rows, _ := values[0].Interface().([]sqlc.ListBotACLRulesRow)
	return rows, errorFromValue(values[1])
}

func deleteBotACLRuleByID(ctx context.Context, queries dbstore.Queries, teamID, botID, id pgtype.UUID) error {
	values, err := callTeamScopedQuery(ctx, queries, "DeleteBotACLRuleByID", teamID, map[string]reflect.Value{
		"ID":    reflect.ValueOf(id),
		"BotID": reflect.ValueOf(botID),
	}, reflect.ValueOf(id))
	if err != nil {
		return err
	}
	return errorFromValue(values[0])
}

func callTeamScopedQuery(ctx context.Context, queries dbstore.Queries, methodName string, teamID pgtype.UUID, fields map[string]reflect.Value, legacyArg reflect.Value) ([]reflect.Value, error) {
	method := reflect.ValueOf(queries).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("query method %s not configured", methodName)
	}
	if method.Type().NumIn() != 2 {
		return nil, fmt.Errorf("query method %s has unexpected arity", methodName)
	}
	argType := method.Type().In(1)
	var arg reflect.Value
	if legacyArg.IsValid() && legacyArg.Type().AssignableTo(argType) {
		arg = legacyArg
	} else {
		arg = reflect.New(argType).Elem()
		if arg.Kind() != reflect.Struct {
			return nil, fmt.Errorf("query method %s has unsupported arg type %s", methodName, argType)
		}
		if field := arg.FieldByName("TeamID"); field.IsValid() && field.CanSet() && reflect.TypeOf(teamID).AssignableTo(field.Type()) {
			field.Set(reflect.ValueOf(teamID))
		}
		for name, value := range fields {
			field := arg.FieldByName(name)
			if field.IsValid() && field.CanSet() && value.Type().AssignableTo(field.Type()) {
				field.Set(value)
			}
		}
	}
	return method.Call([]reflect.Value{reflect.ValueOf(ctx), arg}), nil
}

func errorFromValue(value reflect.Value) error {
	if !value.IsValid() || value.IsNil() {
		return nil
	}
	err, _ := value.Interface().(error)
	return err
}
