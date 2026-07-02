package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/mcp"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
)

func TestProbeEnabledPluginMCPsRecordsFailure(t *testing.T) {
	resource := pluginspkg.Resource{
		Type:       "mcp",
		Key:        "docs",
		ResourceID: "conn-1",
		Metadata:   map[string]any{"display_name": "Docs MCP"},
	}
	recorder := &pluginMCPProbeTestRecorder{}

	err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:        "install-1",
		Status:    pluginspkg.StatusReady,
		Enabled:   true,
		Resources: []pluginspkg.Resource{resource},
	}, pluginMCPProbeDeps{
		connections: pluginMCPProbeTestConnections{items: map[string]mcp.Connection{
			"conn-1": {ID: "conn-1", Type: "http", Active: true},
		}},
		gateway:  pluginMCPProbeTestGateway{err: errors.New("401 invalid or expired token")},
		recorder: recorder,
	})

	if err == nil || !strings.Contains(err.Error(), "Docs MCP") || !strings.Contains(err.Error(), "authorization failed") {
		t.Fatalf("error = %v, want resource label and probe failure", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("record calls = %d, want 1", len(recorder.calls))
	}
	call := recorder.calls[0]
	if call.status != "error" || call.message != "authorization failed" || len(call.tools) != 0 {
		t.Fatalf("recorded call = %+v, want error probe result", call)
	}
}

func TestProbeEnabledPluginMCPsRecordsSuccess(t *testing.T) {
	resource := pluginspkg.Resource{
		Type:       "mcp",
		Key:        "docs",
		ResourceID: "conn-1",
	}
	recorder := &pluginMCPProbeTestRecorder{}
	tools := []mcp.ToolDescriptor{{Name: "search", InputSchema: map[string]any{"type": "object"}}}

	if err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:        "install-1",
		Status:    pluginspkg.StatusReady,
		Enabled:   true,
		Resources: []pluginspkg.Resource{resource},
	}, pluginMCPProbeDeps{
		connections: pluginMCPProbeTestConnections{items: map[string]mcp.Connection{
			"conn-1": {ID: "conn-1", Type: "sse", Active: true},
		}},
		gateway:  pluginMCPProbeTestGateway{tools: tools},
		recorder: recorder,
	}); err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("record calls = %d, want 1", len(recorder.calls))
	}
	call := recorder.calls[0]
	if call.status != "connected" || call.message != "" || len(call.tools) != 1 || call.tools[0].Name != "search" {
		t.Fatalf("recorded call = %+v, want connected probe result", call)
	}
}

func TestProbeEnabledPluginMCPsRecordsOnlyAllowedTools(t *testing.T) {
	resource := pluginspkg.Resource{
		Type:       "mcp",
		Key:        "stripe",
		ResourceID: "conn-1",
	}
	recorder := &pluginMCPProbeTestRecorder{}

	if err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:        "install-1",
		Status:    pluginspkg.StatusReady,
		Enabled:   true,
		Resources: []pluginspkg.Resource{resource},
	}, pluginMCPProbeDeps{
		connections: pluginMCPProbeTestConnections{items: map[string]mcp.Connection{
			"conn-1": {
				ID:       "conn-1",
				Type:     "http",
				Active:   true,
				Metadata: map[string]any{"allowed_tools": []any{"stripe_api_read", "fetch_stripe_resources"}},
			},
		}},
		gateway: pluginMCPProbeTestGateway{tools: []mcp.ToolDescriptor{
			{Name: "stripe_api_read"},
			{Name: "stripe_api_write"},
			{Name: "fetch_stripe_resources"},
			{Name: "create_refund"},
		}},
		recorder: recorder,
	}); err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("record calls = %d, want 1", len(recorder.calls))
	}
	got := recorder.calls[0].tools
	if len(got) != 2 || got[0].Name != "stripe_api_read" || got[1].Name != "fetch_stripe_resources" {
		t.Fatalf("recorded tools = %#v, want only allowed Stripe tools", got)
	}
}

func TestProbeEnabledPluginMCPsFailsWhenResourceMissingConnection(t *testing.T) {
	err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:      "install-1",
		Status:  pluginspkg.StatusReady,
		Enabled: true,
		Resources: []pluginspkg.Resource{{
			Type:     "mcp",
			Key:      "docs",
			Metadata: map[string]any{"display_name": "Docs MCP"},
		}},
	}, pluginMCPProbeDeps{})

	if err == nil || !strings.Contains(err.Error(), "Docs MCP") || !strings.Contains(err.Error(), "missing a connection") {
		t.Fatalf("error = %v, want missing connection error with resource label", err)
	}
}

func TestProbeEnabledPluginMCPsFailsWhenConnectionInactive(t *testing.T) {
	err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:      "install-1",
		Status:  pluginspkg.StatusReady,
		Enabled: true,
		Resources: []pluginspkg.Resource{{
			Type:       "mcp",
			Key:        "docs",
			ResourceID: "conn-1",
			Metadata:   map[string]any{"display_name": "Docs MCP"},
		}},
	}, pluginMCPProbeDeps{
		connections: pluginMCPProbeTestConnections{items: map[string]mcp.Connection{
			"conn-1": {ID: "conn-1", Type: "http", Active: false},
		}},
	})

	if err == nil || !strings.Contains(err.Error(), "Docs MCP") || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("error = %v, want inactive connection error with resource label", err)
	}
}

func TestProbeReadyPluginMCPsAllowsInactiveConnection(t *testing.T) {
	resource := pluginspkg.Resource{
		Type:       "mcp",
		Key:        "docs",
		ResourceID: "conn-1",
	}
	recorder := &pluginMCPProbeTestRecorder{}

	err := probePluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:        "install-1",
		Status:    pluginspkg.StatusReady,
		Enabled:   false,
		Resources: []pluginspkg.Resource{resource},
	}, pluginMCPProbeOptions{requireEnabled: false, requireActive: false}, pluginMCPProbeDeps{
		connections: pluginMCPProbeTestConnections{items: map[string]mcp.Connection{
			"conn-1": {ID: "conn-1", Type: "http", Active: false},
		}},
		gateway:  pluginMCPProbeTestGateway{tools: []mcp.ToolDescriptor{{Name: "search"}}},
		recorder: recorder,
	})
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if len(recorder.calls) != 1 || recorder.calls[0].status != "connected" {
		t.Fatalf("record calls = %+v, want connected result", recorder.calls)
	}
}

func TestProbeEnabledPluginMCPsFailsWhenDeclaredResourceNotInstalled(t *testing.T) {
	err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:      "install-1",
		Status:  pluginspkg.StatusReady,
		Enabled: true,
		Manifest: pluginspkg.Manifest{
			MCPs: []pluginspkg.MCPResource{{Key: "docs", DisplayName: "Docs MCP"}},
		},
	}, pluginMCPProbeDeps{})

	if err == nil || !strings.Contains(err.Error(), "declares MCP resources") {
		t.Fatalf("error = %v, want missing installed MCP resource error", err)
	}
}

func TestProbeEnabledPluginMCPsSkipsPluginsWithoutMCPResources(t *testing.T) {
	if err := probeEnabledPluginMCPs(context.Background(), "bot-1", pluginspkg.Installation{
		ID:      "install-1",
		Status:  pluginspkg.StatusReady,
		Enabled: true,
		Resources: []pluginspkg.Resource{{
			Type:       "skill",
			Key:        "review",
			ResourceID: "/data/skills/review/SKILL.md",
		}},
	}, pluginMCPProbeDeps{}); err != nil {
		t.Fatalf("probe returned error for plugin without MCP resources: %v", err)
	}
}

type pluginMCPProbeTestConnections struct {
	items map[string]mcp.Connection
}

func (c pluginMCPProbeTestConnections) Get(_ context.Context, _ string, id string) (mcp.Connection, error) {
	conn, ok := c.items[id]
	if !ok {
		return mcp.Connection{}, errors.New("not found")
	}
	return conn, nil
}

type pluginMCPProbeTestGateway struct {
	tools []mcp.ToolDescriptor
	err   error
}

func (g pluginMCPProbeTestGateway) ListHTTPConnectionTools(context.Context, mcp.Connection) ([]mcp.ToolDescriptor, error) {
	return g.tools, g.err
}

func (g pluginMCPProbeTestGateway) ListSSEConnectionTools(context.Context, mcp.Connection) ([]mcp.ToolDescriptor, error) {
	return g.tools, g.err
}

func (g pluginMCPProbeTestGateway) ListStdioConnectionTools(context.Context, string, mcp.Connection) ([]mcp.ToolDescriptor, error) {
	return g.tools, g.err
}

type pluginMCPProbeTestRecorder struct {
	calls []pluginMCPProbeTestRecord
}

type pluginMCPProbeTestRecord struct {
	botID          string
	installationID string
	resourceKey    string
	resourceID     string
	status         string
	tools          []mcp.ToolDescriptor
	message        string
}

func (r *pluginMCPProbeTestRecorder) RecordMCPResourceProbeResult(_ context.Context, botID, installationID, resourceKey, resourceID, status string, tools []mcp.ToolDescriptor, message string) error {
	r.calls = append(r.calls, pluginMCPProbeTestRecord{
		botID:          botID,
		installationID: installationID,
		resourceKey:    resourceKey,
		resourceID:     resourceID,
		status:         status,
		tools:          tools,
		message:        message,
	})
	return nil
}
