package federation

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

type testConnectionLister struct {
	items        []mcpgw.Connection
	err          error
	probeResults []testProbeResult
}

func (l *testConnectionLister) ListActiveByBot(_ context.Context, _ string) ([]mcpgw.Connection, error) {
	if l.err != nil {
		return nil, l.err
	}
	return l.items, nil
}

func (l *testConnectionLister) UpdateProbeResult(_ context.Context, botID, id, status string, tools []mcpgw.ToolDescriptor, message string) error {
	l.probeResults = append(l.probeResults, testProbeResult{
		botID:   botID,
		id:      id,
		status:  status,
		tools:   tools,
		message: message,
	})
	return nil
}

type testProbeResult struct {
	botID   string
	id      string
	status  string
	tools   []mcpgw.ToolDescriptor
	message string
}

type testGateway struct {
	listHTTP  []mcpgw.ToolDescriptor
	listSSE   []mcpgw.ToolDescriptor
	listStdio []mcpgw.ToolDescriptor
	listErr   error

	lastCallType string
}

func (g *testGateway) ListHTTPConnectionTools(_ context.Context, _ mcpgw.Connection) ([]mcpgw.ToolDescriptor, error) {
	return g.listHTTP, g.listErr
}

func (g *testGateway) CallHTTPConnectionTool(_ context.Context, _ mcpgw.Connection, _ string, _ map[string]any) (map[string]any, error) {
	g.lastCallType = "http"
	return map[string]any{"result": map[string]any{"ok": true, "route": "http"}}, nil
}

func (g *testGateway) ListSSEConnectionTools(_ context.Context, _ mcpgw.Connection) ([]mcpgw.ToolDescriptor, error) {
	return g.listSSE, g.listErr
}

func (g *testGateway) CallSSEConnectionTool(_ context.Context, _ mcpgw.Connection, _ string, _ map[string]any) (map[string]any, error) {
	g.lastCallType = "sse"
	return map[string]any{"result": map[string]any{"ok": true, "route": "sse"}}, nil
}

func (g *testGateway) ListStdioConnectionTools(_ context.Context, _ string, _ mcpgw.Connection) ([]mcpgw.ToolDescriptor, error) {
	return g.listStdio, g.listErr
}

func (g *testGateway) CallStdioConnectionTool(_ context.Context, _ string, _ mcpgw.Connection, _ string, _ map[string]any) (map[string]any, error) {
	g.lastCallType = "stdio"
	return map[string]any{"result": map[string]any{"ok": true, "route": "stdio"}}, nil
}

func TestSourceListToolsIncludesSSETools(t *testing.T) {
	gateway := &testGateway{
		listSSE: []mcpgw.ToolDescriptor{
			{
				Name:        "search",
				Description: "search remote data",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "Remote SSE",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}

	source := NewSource(slog.Default(), gateway, lister)
	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "remote_sse_search" {
		t.Fatalf("unexpected tool alias: %s", tools[0].Name)
	}
	if len(lister.probeResults) != 1 || lister.probeResults[0].status != "connected" || len(lister.probeResults[0].tools) != 1 {
		t.Fatalf("probe results = %#v, want connected result with tools", lister.probeResults)
	}
}

func TestSourceListToolsRecordsProbeFailure(t *testing.T) {
	gateway := &testGateway{listErr: errors.New("401 invalid or expired token")}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "Remote SSE",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}

	source := NewSource(slog.Default(), gateway, lister)
	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want no tools after probe failure", tools)
	}
	if len(lister.probeResults) != 1 {
		t.Fatalf("probe results = %#v, want one error result", lister.probeResults)
	}
	result := lister.probeResults[0]
	if result.botID != "bot-1" || result.id != "conn-1" || result.status != "error" || result.message != "401 invalid or expired token" {
		t.Fatalf("probe result = %#v, want error result", result)
	}
}

func TestSourceCallToolRoutesToSSEConnection(t *testing.T) {
	gateway := &testGateway{
		listSSE: []mcpgw.ToolDescriptor{
			{
				Name:        "search",
				Description: "search remote data",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "Remote SSE",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}
	source := NewSource(slog.Default(), gateway, lister)

	result, err := source.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}, "remote_sse_search", map[string]any{"query": "hello"})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if gateway.lastCallType != "sse" {
		t.Fatalf("expected sse route, got: %s", gateway.lastCallType)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("expected ok=true in result")
	}
}

func TestSourceRenamesReservedToolAliases(t *testing.T) {
	gateway := &testGateway{
		listSSE: []mcpgw.ToolDescriptor{
			{
				Name:        "observe",
				Description: "observe browser state",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "browser",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}
	source := NewSource(slog.Default(), gateway, lister, WithReservedToolName(func(name string) bool {
		return name == "browser_observe"
	}))

	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "browser_observe_2" {
		t.Fatalf("tools = %#v, want reserved alias renamed to browser_observe_2", tools)
	}

	result, err := source.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}, "browser_observe_2", map[string]any{})
	if err != nil {
		t.Fatalf("call renamed tool failed: %v", err)
	}
	if route, _ := result["route"].(string); route != "sse" {
		t.Fatalf("result = %#v, want sse route", result)
	}
}

func TestSourceFiltersToolsByConnectionAllowlist(t *testing.T) {
	gateway := &testGateway{
		listHTTP: []mcpgw.ToolDescriptor{
			{Name: "stripe_api_read", Description: "read stripe data", InputSchema: map[string]any{"type": "object"}},
			{Name: "stripe_api_search", Description: "search stripe api methods", InputSchema: map[string]any{"type": "object"}},
			{Name: "stripe_api_details", Description: "inspect stripe api method details", InputSchema: map[string]any{"type": "object"}},
			{Name: "search_stripe_resources", Description: "search stripe resources", InputSchema: map[string]any{"type": "object"}},
			{Name: "fetch_stripe_resources", Description: "fetch stripe resources", InputSchema: map[string]any{"type": "object"}},
			{Name: "get_stripe_account_info", Description: "get stripe account info", InputSchema: map[string]any{"type": "object"}},
			{Name: "stripe_api_write", Description: "write stripe data", InputSchema: map[string]any{"type": "object"}},
			{Name: "create_refund", Description: "create refund", InputSchema: map[string]any{"type": "object"}},
			{Name: "stripe_report", Description: "create or retrieve reports", InputSchema: map[string]any{"type": "object"}},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "stripe_stripe",
				Type:   "http",
				Active: true,
				Config: map[string]any{"url": "https://mcp.stripe.com"},
				Metadata: map[string]any{"allowed_tools": []any{
					"stripe_api_read",
					"stripe_api_search",
					"stripe_api_details",
					"search_stripe_resources",
					"fetch_stripe_resources",
					"get_stripe_account_info",
				}},
			},
		},
	}
	source := NewSource(slog.Default(), gateway, lister)

	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 6 {
		t.Fatalf("tools = %#v, want 6 read-only tools", tools)
	}
	gotNames := map[string]bool{}
	for _, tool := range tools {
		gotNames[tool.Name] = true
	}
	for _, name := range []string{
		"stripe_stripe_stripe_api_read",
		"stripe_stripe_stripe_api_search",
		"stripe_stripe_stripe_api_details",
		"stripe_stripe_search_stripe_resources",
		"stripe_stripe_fetch_stripe_resources",
		"stripe_stripe_get_stripe_account_info",
	} {
		if !gotNames[name] {
			t.Fatalf("tools = %#v, missing %s", tools, name)
		}
	}
	if _, err := source.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}, "stripe_stripe_stripe_api_write", map[string]any{}); !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Fatalf("write tool call err = %v, want ErrToolNotFound", err)
	}
	if _, err := source.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}, "stripe_stripe_create_refund", map[string]any{}); !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Fatalf("create refund tool call err = %v, want ErrToolNotFound", err)
	}
	if _, err := source.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}, "stripe_stripe_stripe_report", map[string]any{}); !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Fatalf("report tool call err = %v, want ErrToolNotFound", err)
	}
}

func TestSourceInvalidateBotClearsToolCache(t *testing.T) {
	gateway := &testGateway{
		listSSE: []mcpgw.ToolDescriptor{
			{
				Name:        "search",
				Description: "search remote data",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "Remote SSE",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}
	source := NewSource(slog.Default(), gateway, lister)

	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected cached source to list 1 tool, got %d", len(tools))
	}

	lister.items = nil
	tools, err = source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools from cache failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected stale cache before invalidation, got %d tools", len(tools))
	}

	source.InvalidateBot("bot-1")
	tools, err = source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools after invalidation failed: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("expected no tools after invalidation, got %#v", tools)
	}
}

func TestSourceInvalidateAllClearsToolCache(t *testing.T) {
	gateway := &testGateway{
		listSSE: []mcpgw.ToolDescriptor{
			{
				Name:        "search",
				Description: "search remote data",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	lister := &testConnectionLister{
		items: []mcpgw.Connection{
			{
				ID:     "conn-1",
				Name:   "Remote SSE",
				Type:   "sse",
				Active: true,
				Config: map[string]any{"url": "http://example.com/sse"},
			},
		},
	}
	source := NewSource(slog.Default(), gateway, lister)

	if _, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"}); err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	lister.items = nil
	source.InvalidateAll()
	tools, err := source.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools after invalidating all failed: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("expected no tools after invalidating all, got %#v", tools)
	}
}
