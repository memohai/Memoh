package db

import (
	"os"
	"strings"
	"testing"
)

func TestUserInputPromptDeliveryMigration(t *testing.T) {
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0117_user_input_prompt_delivery.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0117_user_input_prompt_delivery.down.sql")

	for name, sql := range map[string]string{"baseline": baseline, "0117 up": up} {
		if !strings.Contains(sql, "prompt_delivered_at TIMESTAMPTZ") {
			t.Fatalf("%s is missing prompt_delivered_at", name)
		}
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS prompt_delivered_at") {
		t.Fatal("0117 down is missing prompt_delivered_at rollback")
	}

	queries, err := os.ReadFile("../../db/postgres/queries/user_input.sql")
	if err != nil {
		t.Fatalf("read user input queries: %v", err)
	}
	latestPending := querySection(t, string(queries), "GetLatestPendingUserInputBySession", "GetPendingUserInputByReplyMessage")
	if !strings.Contains(latestPending, "prompt_delivered_at IS NOT NULL OR prompt_external_message_id <> ''") {
		t.Fatal("latest pending prompt lookup does not accept external message delivery evidence")
	}
}

func querySection(t *testing.T, source, name, next string) string {
	t.Helper()
	start := strings.Index(source, "-- name: "+name+" ")
	end := strings.Index(source, "-- name: "+next+" ")
	if start < 0 || end <= start {
		t.Fatalf("query section %s was not found", name)
	}
	return strings.Join(strings.Fields(source[start:end]), " ")
}
