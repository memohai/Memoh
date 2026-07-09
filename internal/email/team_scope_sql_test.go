package email

import (
	"os"
	"strings"
	"testing"
)

func TestEmailQueriesAreTeamScoped(t *testing.T) {
	files := map[string][]string{
		"../../db/postgres/queries/email_providers.sql": {
			"sqlc.arg(team_id)",
			"team_id",
		},
		"../../db/postgres/queries/email_oauth_tokens.sql": {
			"sqlc.arg(team_id)",
			"email_providers",
			"team_id",
		},
		"../../db/postgres/queries/email_bindings.sql": {
			"sqlc.arg(team_id)",
			"bots",
			"email_providers",
			"team_id",
		},
		"../../db/postgres/queries/email_outbox.sql": {
			"sqlc.arg(team_id)",
			"bots",
			"team_id",
		},
	}

	for file, required := range files {
		// #nosec G304 -- tests read fixed checked-in SQL query files.
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		sql := strings.ToLower(string(body))
		for _, token := range required {
			if !strings.Contains(sql, strings.ToLower(token)) {
				t.Fatalf("%s is missing team-scope token %q", file, token)
			}
		}
	}
}
