package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestByIDQueriesAreTeamScoped(t *testing.T) {
	// These by-id queries must have team_id filter (Phase 4 hardening, anti-regression).
	want := map[string][]string{
		"tool_approval.sql":    {"ApproveToolApprovalRequest", "RejectToolApprovalRequest", "UpdateToolApprovalPromptMessage"},
		"user_input.sql":       {"SubmitUserInputRequest", "CancelUserInputRequest", "FailUserInputRequest", "UpdateUserInputAssistantMessage", "UpdateUserInputPromptMessage", "UpdateUserInputToolResultMessage"},
		"models.sql":           {"GetModelByID", "DeleteModel", "GetProviderByID", "DeleteProvider", "GetSpeechModelWithProvider", "GetTranscriptionModelWithProvider", "GetVideoModelWithProvider"},
		"fetch_providers.sql":  {"GetFetchProviderByID", "DeleteFetchProvider"},
		"search_providers.sql": {"GetSearchProviderByID", "DeleteSearchProvider"},
		"settings.sql":         {"UpsertBotSettings"},
	}
	for filename, names := range want {
		path := filepath.Join("..", "..", "db", "postgres", "queries", filename)
		// #nosec G304 -- test reads fixed checked-in SQL files
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		content := string(data)
		blocks := strings.Split(content, "-- name: ")
		for _, n := range names {
			var found bool
			for _, b := range blocks[1:] {
				if strings.HasPrefix(b, n+" ") {
					found = true
					if !strings.Contains(b, "team_id") {
						t.Errorf("%s query %s missing team_id predicate", filename, n)
					}
				}
			}
			if !found {
				t.Errorf("%s query %s not found", filename, n)
			}
		}
	}
}
