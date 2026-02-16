package healthcheck

import (
	"context"
	"testing"
)

type testChecker struct {
	items []CheckResult
}

func (c *testChecker) ListChecks(ctx context.Context, botID string) []CheckResult {
	return c.items
}

func TestRuntimeCheckerAdapterListChecks(t *testing.T) {
	t.Parallel()

	adapter := NewRuntimeCheckerAdapter(&testChecker{
		items: []CheckResult{
			{
				ID:       "mcp.connection.conn-1",
				Type:     "mcp.connection",
				TitleKey: "bots.checks.titles.mcpConnection",
				Subtitle: "demo",
				Status:   "ok",
				Summary:  "healthy",
				Detail:   "",
				Metadata: map[string]any{"tool_count": 2},
			},
		},
	})

	items := adapter.ListChecks(context.Background(), "bot-1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "mcp.connection.conn-1" {
		t.Fatalf("unexpected id: %s", items[0].ID)
	}
	if items[0].TitleKey != "bots.checks.titles.mcpConnection" {
		t.Fatalf("unexpected title key: %s", items[0].TitleKey)
	}
	if items[0].Status != "ok" {
		t.Fatalf("unexpected status: %s", items[0].Status)
	}
}

func TestRuntimeCheckerAdapterNilChecker(t *testing.T) {
	t.Parallel()

	var adapter *RuntimeCheckerAdapter
	items := adapter.ListChecks(context.Background(), "bot-1")
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %d", len(items))
	}
}
