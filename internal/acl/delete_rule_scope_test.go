package acl

import (
	"context"
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

// TestDeleteBotACLRuleByIDIsBotScoped guards the cross-bot ACL deletion fix.
//
// The reflective team-scope helper (callTeamScopedQuery) binds bot_id only when
// the dbstore.Queries method takes a Params struct: with a bare pgtype.UUID arg
// it silently takes the legacy branch and drops BotID, so the delete runs
// without the bot_id guard (and, once the query gained bot_id = sqlc.arg(bot_id),
// matches nothing). Lock the method signature so the guard cannot regress.
func TestDeleteBotACLRuleByIDIsBotScoped(t *testing.T) {
	m, ok := reflect.TypeOf((*dbstore.Queries)(nil)).Elem().MethodByName("DeleteBotACLRuleByID")
	if !ok {
		t.Fatal("dbstore.Queries has no DeleteBotACLRuleByID method")
	}
	// Interface method Type() has no receiver: In(0)=ctx, In(1)=arg.
	if got, want := m.Type.NumIn(), 2; got != want {
		t.Fatalf("DeleteBotACLRuleByID arity = %d, want %d", got, want)
	}
	if m.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Fatalf("first arg = %s, want context.Context", m.Type.In(0))
	}
	argType := m.Type.In(1)
	if argType != reflect.TypeOf(sqlc.DeleteBotACLRuleByIDParams{}) {
		t.Fatalf("arg type = %s, want sqlc.DeleteBotACLRuleByIDParams (a bare uuid drops bot_id)", argType)
	}
	if _, ok := argType.FieldByName("BotID"); !ok {
		t.Fatal("DeleteBotACLRuleByIDParams has no BotID field; bot-scoped delete cannot be enforced")
	}
}
