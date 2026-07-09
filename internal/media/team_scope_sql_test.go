package media

import (
	"os"
	"strings"
	"testing"
)

func TestMediaQueriesScopeBotOwnedRecordsByTeam(t *testing.T) {
	body, err := os.ReadFile("../../db/postgres/queries/media.sql")
	if err != nil {
		t.Fatalf("read media queries: %v", err)
	}
	sql := strings.ToLower(string(body))

	required := []string{
		"storage_providers",
		"bot_storage_bindings",
		"bot_history_messages",
		"sqlc.arg(team_id)",
		"bots",
		"team_id",
	}
	for _, token := range required {
		if !strings.Contains(sql, strings.ToLower(token)) {
			t.Fatalf("media.sql is missing team-scope token %q", token)
		}
	}

	if strings.Contains(sql, "insert into storage_providers (team_id") ||
		strings.Contains(sql, "where team_id = sqlc.arg(team_id)") && strings.Contains(sql, "from storage_providers") {
		t.Fatal("storage_providers should remain operator-global, not directly team-scoped")
	}
}
