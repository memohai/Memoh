package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestMissingRequiredVariablesTreatsSelfTemplateDefaultAsMissing(t *testing.T) {
	manifest := Manifest{
		Variables: []ConfigVar{
			{Key: "NOTION_TOKEN", Required: true, Secret: true},
		},
	}
	resource := MCPResource{
		Env: []ConfigVar{
			{Key: "NOTION_TOKEN", DefaultValue: "${NOTION_TOKEN}", Required: true, Secret: true},
		},
	}
	authReq := AuthRequirement{
		Type:      "user_secret",
		Variables: []string{"NOTION_TOKEN"},
	}

	if !missingRequiredVariables(manifest, resource, authReq, map[string]string{}) {
		t.Fatal("expected missing user secret when the only value is a self-template default")
	}
	if !missingResourceConfig(manifest, resource, map[string]string{}) {
		t.Fatal("expected missing resource config when the only value is a self-template default")
	}
}

func TestResolveConfigValueExpandsTemplateWhenVariableIsProvided(t *testing.T) {
	manifest := Manifest{
		Variables: []ConfigVar{
			{Key: "TOKEN", Required: true, Secret: true},
		},
	}
	resource := MCPResource{
		Headers: []ConfigVar{
			{Key: "Authorization", DefaultValue: "Bearer ${TOKEN}", Required: true, Secret: true},
		},
	}
	resolved := resolveVariables(manifest, resource, map[string]string{"TOKEN": "abc123"})

	if got := resolveConfigValue(resource.Headers[0], resolved); got != "Bearer abc123" {
		t.Fatalf("expected expanded authorization header, got %q", got)
	}
	if missingResourceConfig(manifest, resource, map[string]string{"TOKEN": "abc123"}) {
		t.Fatal("expected resource config to be present when template variable is provided")
	}
}

func TestResolveConfigValueDropsUnresolvedTemplate(t *testing.T) {
	item := ConfigVar{Key: "Authorization", DefaultValue: "Bearer ${TOKEN}", Required: true}

	if got := resolveConfigValue(item, map[string]string{}); got != "" {
		t.Fatalf("expected unresolved template to resolve to empty string, got %q", got)
	}
}

func TestVariablesFromConfigRestoresSavedInstallVariables(t *testing.T) {
	variables, err := variablesFromConfig([]byte(`{"variables":{"TOKEN":"abc123","COUNT":7,"EMPTY":null}}`))
	if err != nil {
		t.Fatalf("variablesFromConfig: %v", err)
	}
	if variables["TOKEN"] != "abc123" {
		t.Fatalf("TOKEN = %q, want abc123", variables["TOKEN"])
	}
	if variables["COUNT"] != "7" {
		t.Fatalf("COUNT = %q, want 7", variables["COUNT"])
	}
	if _, ok := variables["EMPTY"]; ok {
		t.Fatal("nil variables should be omitted")
	}
}

func TestRedactConfigIncludesResourceLevelVariables(t *testing.T) {
	manifest := Manifest{
		Variables: []ConfigVar{
			{Key: "TOKEN", Required: true, Secret: true},
			{Key: "DOMAIN", Required: false, Secret: false},
		},
		MCPs: []MCPResource{{
			Key: "docs",
			Headers: []ConfigVar{{
				Key:          "Authorization",
				DefaultValue: "Bearer ${TOKEN}",
				Required:     true,
				Secret:       true,
			}},
			Env: []ConfigVar{{
				Key:      "DIRECT_SECRET",
				Required: true,
				Secret:   true,
			}},
		}},
	}

	redacted := redactConfig(manifest, map[string]any{
		"variables": map[string]any{
			"TOKEN":         "abc123",
			"DOMAIN":        "https://example.test",
			"DIRECT_SECRET": "hidden",
		},
	})
	variables, ok := redacted["variables"].(map[string]any)
	if !ok {
		t.Fatalf("redacted variables = %#v", redacted["variables"])
	}
	if status, ok := variables["TOKEN"].(map[string]any); !ok || status["configured"] != true {
		t.Fatalf("TOKEN status = %#v, want configured", variables["TOKEN"])
	}
	if value := variables["TOKEN"].(map[string]any)["value"]; value != nil {
		t.Fatalf("secret TOKEN value = %#v, want redacted", value)
	}
	if status, ok := variables["DOMAIN"].(map[string]any); !ok || status["configured"] != true || status["value"] != "https://example.test" {
		t.Fatalf("DOMAIN status = %#v, want visible non-secret value", variables["DOMAIN"])
	}
	if _, ok := variables["Authorization"]; ok {
		t.Fatalf("templated Authorization header should be hidden when TOKEN is rendered: %#v", variables)
	}
	if status, ok := variables["DIRECT_SECRET"].(map[string]any); !ok || status["configured"] != true {
		t.Fatalf("DIRECT_SECRET status = %#v, want configured", variables["DIRECT_SECRET"])
	}
}

func TestUpdateConfigClearsStoredVariable(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000103"
	manifest := Manifest{
		ID:   "docs",
		Name: "Docs",
		Variables: []ConfigVar{
			{Key: "TOKEN", Required: true, Secret: true},
			{Key: "HOST"},
		},
		MCPs: []MCPResource{{
			Key:     "docs",
			Command: "docs-mcp",
			Env:     []ConfigVar{{Key: "TOKEN", DefaultValue: "${TOKEN}", Required: true, Secret: true}},
		}},
		AuthRequirements: []AuthRequirement{{
			Key:       "token",
			Type:      "user_secret",
			Variables: []string{"TOKEN"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{
		"TOKEN": "old-secret",
		"HOST":  "example.test",
	})

	updated, err := svc.UpdateConfig(ctx, botID, installationID, UpdateConfigRequest{
		Variables: map[string]string{"TOKEN": ""},
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.Status != StatusNeedsConfig {
		t.Fatalf("status = %q, want %q", updated.Status, StatusNeedsConfig)
	}
	if updated.Enabled {
		t.Fatal("plugin should be disabled when clearing required config")
	}
	row, err := svc.getRow(ctx, botID, installationID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	var stored struct {
		Variables map[string]string `json:"variables"`
	}
	if err := json.Unmarshal(row.Config, &stored); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if _, ok := stored.Variables["TOKEN"]; ok {
		t.Fatalf("TOKEN should be removed from stored variables: %#v", stored.Variables)
	}
	if stored.Variables["HOST"] != "example.test" {
		t.Fatalf("HOST = %q, want preserved value", stored.Variables["HOST"])
	}
}

func TestInstallRejectsInvalidConfigOption(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	manifest := Manifest{
		ID:   "lark",
		Name: "Lark",
		Variables: []ConfigVar{{
			Key:          "LARK_DOMAIN",
			DefaultValue: "https://open.larksuite.com",
			Options: []ConfigVarOption{
				{Label: "Lark", Value: "https://open.larksuite.com"},
				{Label: "Feishu", Value: "https://open.feishu.cn"},
			},
		}},
		MCPs: []MCPResource{{
			Key:     "lark",
			Command: "lark-mcp",
			Args:    []string{"--domain", "${LARK_DOMAIN}"},
		}},
	}

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "unsupported", value: "https://example.com"},
		{name: "leading space", value: " https://open.feishu.cn"},
		{name: "trailing space", value: "https://open.feishu.cn "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Install(ctx, botID, InstallRequest{
				Manifest:  manifest,
				Variables: map[string]string{"LARK_DOMAIN": tc.value},
			})
			if err == nil {
				t.Fatal("Install() error = nil, want invalid option error")
			}
		})
	}
}

func TestInstallRejectsInvalidConfigOptionDefault(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	_, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "lark",
			Name: "Lark",
			Variables: []ConfigVar{{
				Key:          "LARK_DOMAIN",
				DefaultValue: "https://example.com",
				Options: []ConfigVarOption{
					{Label: "Lark", Value: "https://open.larksuite.com"},
					{Label: "Feishu", Value: "https://open.feishu.cn"},
				},
			}},
			MCPs: []MCPResource{{
				Key:     "lark",
				Command: "lark-mcp",
				Args:    []string{"--domain", "${LARK_DOMAIN}"},
			}},
		},
	})
	if err == nil {
		t.Fatal("Install() error = nil, want invalid default option error")
	}
}

func TestInstallRejectsTemplatedConfigOptionDefaultOutsideAllowList(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	_, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "lark",
			Name: "Lark",
			Variables: []ConfigVar{
				{Key: "BASE_URL", DefaultValue: "https://example.com"},
				{
					Key:          "LARK_DOMAIN",
					DefaultValue: "${BASE_URL}",
					Options: []ConfigVarOption{
						{Label: "Lark", Value: "https://open.larksuite.com"},
						{Label: "Feishu", Value: "https://open.feishu.cn"},
					},
				},
			},
			MCPs: []MCPResource{{
				Key:     "lark",
				Command: "lark-mcp",
				Args:    []string{"--domain", "${LARK_DOMAIN}"},
			}},
		},
	})
	if err == nil {
		t.Fatal("Install() error = nil, want templated default option error")
	}
}

func TestInstallAllowsValidConfigOption(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installed, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "lark",
			Name: "Lark",
			Variables: []ConfigVar{{
				Key: "LARK_DOMAIN",
				Options: []ConfigVarOption{
					{Label: "Lark", Value: "https://open.larksuite.com"},
					{Label: "Feishu", Value: "https://open.feishu.cn"},
				},
			}},
			MCPs: []MCPResource{{
				Key:     "lark",
				Command: "lark-mcp",
				Args:    []string{"--domain", "${LARK_DOMAIN}"},
			}},
		},
		Variables: map[string]string{"LARK_DOMAIN": "https://open.feishu.cn"},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed.Status != StatusReady {
		t.Fatalf("status = %q, want %q", installed.Status, StatusReady)
	}
}

func TestInstallAllowsValidConfigOptionDefault(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installed, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "lark",
			Name: "Lark",
			Variables: []ConfigVar{{
				Key:          "LARK_DOMAIN",
				DefaultValue: "https://open.larksuite.com",
				Options: []ConfigVarOption{
					{Label: "Lark", Value: "https://open.larksuite.com"},
					{Label: "Feishu", Value: "https://open.feishu.cn"},
				},
			}},
			MCPs: []MCPResource{{
				Key:     "lark",
				Command: "lark-mcp",
				Args:    []string{"--domain", "${LARK_DOMAIN}"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed.Status != StatusReady {
		t.Fatalf("status = %q, want %q", installed.Status, StatusReady)
	}
}

func TestInstallTreatsMissingRequiredManifestVariableAsNeedsConfig(t *testing.T) {
	ctx := context.Background()
	_, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installed, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "docs",
			Name: "Docs",
			Variables: []ConfigVar{{
				Key:      "DOCS_TOKEN",
				Required: true,
				Secret:   true,
			}},
			MCPs: []MCPResource{{
				Key:     "docs",
				Command: "docs-mcp",
				Args:    []string{"--token", "${DOCS_TOKEN}"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed.Status != StatusNeedsConfig {
		t.Fatalf("status = %q, want %q", installed.Status, StatusNeedsConfig)
	}
}

func TestUpdateConfigRejectsInvalidConfigOption(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000183"
	manifest := Manifest{
		ID:   "lark",
		Name: "Lark",
		Variables: []ConfigVar{{
			Key: "LARK_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "Lark", Value: "https://open.larksuite.com"},
				{Label: "Feishu", Value: "https://open.feishu.cn"},
			},
		}},
		MCPs: []MCPResource{{
			Key:     "lark",
			Command: "lark-mcp",
			Args:    []string{"--domain", "${LARK_DOMAIN}"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{
		"LARK_DOMAIN": "https://open.larksuite.com",
	})

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "unsupported", value: "https://example.com"},
		{name: "leading space", value: " https://open.feishu.cn"},
		{name: "trailing space", value: "https://open.feishu.cn "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdateConfig(ctx, botID, installationID, UpdateConfigRequest{
				Variables: map[string]string{"LARK_DOMAIN": tc.value},
			})
			if err == nil {
				t.Fatal("UpdateConfig() error = nil, want invalid option error")
			}
		})
	}
}

func TestUpdateConfigClearsConfigOption(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000184"
	manifest := Manifest{
		ID:   "lark",
		Name: "Lark",
		Variables: []ConfigVar{{
			Key: "LARK_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "Lark", Value: "https://open.larksuite.com"},
				{Label: "Feishu", Value: "https://open.feishu.cn"},
			},
		}},
		MCPs: []MCPResource{{
			Key:     "lark",
			Command: "lark-mcp",
			Args:    []string{"--domain", "${LARK_DOMAIN}"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{
		"LARK_DOMAIN": "https://open.larksuite.com",
	})

	updated, err := svc.UpdateConfig(ctx, botID, installationID, UpdateConfigRequest{
		Variables: map[string]string{"LARK_DOMAIN": ""},
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.Enabled {
		t.Fatal("plugin should be disabled after config update")
	}
	row, err := svc.getRow(ctx, botID, installationID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	stored, err := variablesFromConfig(row.Config)
	if err != nil {
		t.Fatalf("variablesFromConfig: %v", err)
	}
	if _, ok := stored["LARK_DOMAIN"]; ok {
		t.Fatalf("LARK_DOMAIN should be removed from stored variables: %#v", stored)
	}
}

func TestSetEnabledRejectsStoredInvalidConfigOption(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000185"
	manifest := Manifest{
		ID:   "lark",
		Name: "Lark",
		Variables: []ConfigVar{{
			Key: "LARK_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "Lark", Value: "https://open.larksuite.com"},
				{Label: "Feishu", Value: "https://open.feishu.cn"},
			},
		}},
		MCPs: []MCPResource{{
			Key:     "lark",
			Command: "lark-mcp",
			Args:    []string{"--domain", "${LARK_DOMAIN}"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, false, map[string]string{
		"LARK_DOMAIN": "https://example.com",
	})

	if _, err := svc.SetEnabled(ctx, botID, installationID, true); err == nil {
		t.Fatal("SetEnabled() error = nil, want invalid stored option error")
	}
}

func TestActivateRejectsStoredInvalidConfigOption(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000190"
	connectionID := "00000000-0000-0000-0000-000000000191"
	manifest := Manifest{
		ID:   "lark",
		Name: "Lark",
		Variables: []ConfigVar{{
			Key: "LARK_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "Lark", Value: "https://open.larksuite.com"},
				{Label: "Feishu", Value: "https://open.feishu.cn"},
			},
		}},
		MCPs: []MCPResource{{
			Key:     "lark",
			Command: "lark-mcp",
			Args:    []string{"--domain", "${LARK_DOMAIN}"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, false, map[string]string{
		"LARK_DOMAIN": "https://example.com",
	})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'lark_lark', 'stdio', '{"command":"lark-mcp"}', 0, ?, 'lark')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}

	if _, err := svc.Activate(ctx, botID, installationID); err == nil {
		t.Fatal("Activate() error = nil, want invalid stored option error")
	}
}

func TestRefreshOAuthStatusRejectsStoredInvalidConfigOption(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000186"
	connectionID := "00000000-0000-0000-0000-000000000187"
	manifest := Manifest{
		ID:   "github",
		Name: "GitHub",
		Variables: []ConfigVar{{
			Key: "GITHUB_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "GitHub", Value: "https://github.com"},
				{Label: "Enterprise", Value: "https://github.example.com"},
			},
		}},
		AuthRequirements: []AuthRequirement{{
			Key:  "oauth",
			Type: "managed_oauth",
		}},
		MCPs: []MCPResource{{
			Key:     "api",
			URL:     "${GITHUB_DOMAIN}/mcp",
			AuthRef: "oauth",
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusNeedsAuth, false, map[string]string{
		"GITHUB_DOMAIN": "https://example.com",
	})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, auth_type, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'github_api', 'http', '{"url":"https://api.github.example/mcp"}', 0, 'oauth', ?, 'api')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000188', ?, 'mcp', 'api', ?, 'needs_auth', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_oauth_tokens(id, connection_id, authorization_endpoint, token_endpoint, access_token, expires_at)
VALUES('00000000-0000-0000-0000-000000000189', ?, 'https://auth.example/authorize', 'https://auth.example/token', 'valid-token', ?)
`, connectionID, expiresAt); err != nil {
		t.Fatalf("insert oauth token: %v", err)
	}

	if _, err := svc.RefreshOAuthStatus(ctx, botID, installationID); err == nil {
		t.Fatal("RefreshOAuthStatus() error = nil, want invalid stored option error")
	}
}

func TestStartOAuthUsesResolvedResourceURL(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000192"
	connectionID := "00000000-0000-0000-0000-000000000193"
	var probedPath string
	origin := func(r *http.Request) string {
		return "http://" + r.Host
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			probedPath = r.URL.Path
			w.Header().Set("WWW-Authenticate", `Bearer scope="mcp:connect"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_servers": []string{origin(r)},
				"scopes_supported":      []string{"mcp:connect"},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 origin(r),
				"authorization_endpoint": origin(r) + "/authorize",
				"token_endpoint":         origin(r) + "/token",
				"registration_endpoint":  origin(r) + "/register",
			})
		case "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "client-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manifest := Manifest{
		ID:   "github",
		Name: "GitHub",
		Variables: []ConfigVar{{
			Key: "GITHUB_DOMAIN",
			Options: []ConfigVarOption{
				{Label: "Local", Value: server.URL},
			},
		}},
		AuthRequirements: []AuthRequirement{{
			Key:  "oauth",
			Type: "managed_oauth",
		}},
		MCPs: []MCPResource{{
			Key:     "api",
			URL:     "${GITHUB_DOMAIN}/mcp",
			AuthRef: "oauth",
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusNeedsAuth, false, map[string]string{
		"GITHUB_DOMAIN": server.URL,
	})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, auth_type, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'github_api', 'http', ?, 0, 'oauth', ?, 'api')
`, connectionID, botID, `{"url":"`+server.URL+`/mcp"}`, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000194', ?, 'mcp', 'api', ?, 'needs_auth', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}

	auth, err := svc.StartOAuth(ctx, botID, installationID, "https://memoh.example/oauth/mcp/callback")
	if err != nil {
		t.Fatalf("StartOAuth() error = %v", err)
	}
	if auth.AuthorizationURL == "" {
		t.Fatal("authorization URL is empty")
	}
	if probedPath != "/mcp" {
		t.Fatalf("probed path = %q, want resolved /mcp", probedPath)
	}
	var resourceURI string
	if err := conn.QueryRowContext(ctx, `SELECT resource_uri FROM mcp_oauth_tokens WHERE connection_id = ?`, connectionID).Scan(&resourceURI); err != nil {
		t.Fatalf("select resource uri: %v", err)
	}
	if resourceURI != server.URL+"/mcp" {
		t.Fatalf("resource_uri = %q, want resolved server URL", resourceURI)
	}
}

func TestRefreshOAuthStatusPreservesDisabledPlugin(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000123"
	connectionID := "00000000-0000-0000-0000-000000000124"
	manifest := Manifest{
		ID:   "github",
		Name: "GitHub",
		AuthRequirements: []AuthRequirement{{
			Key:  "oauth",
			Type: "managed_oauth",
		}},
		MCPs: []MCPResource{{
			Key:     "api",
			URL:     "https://api.github.example/mcp",
			AuthRef: "oauth",
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusDisabled, false, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, auth_type, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'github_api', 'http', '{"url":"https://api.github.example/mcp"}', 0, 'oauth', ?, 'api')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000125', ?, 'mcp', 'api', ?, 'needs_auth', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_oauth_tokens(id, connection_id, authorization_endpoint, token_endpoint, access_token, expires_at)
VALUES('00000000-0000-0000-0000-000000000126', ?, 'https://auth.example/authorize', 'https://auth.example/token', 'valid-token', ?)
`, connectionID, expiresAt); err != nil {
		t.Fatalf("insert oauth token: %v", err)
	}

	updated, err := svc.RefreshOAuthStatus(ctx, botID, installationID)
	if err != nil {
		t.Fatalf("RefreshOAuthStatus() error = %v", err)
	}
	if updated.Status != StatusReady {
		t.Fatalf("status = %q, want %q", updated.Status, StatusReady)
	}
	if updated.Enabled {
		t.Fatal("valid OAuth token should not re-enable a disabled plugin")
	}
	var active int
	if err := conn.QueryRowContext(ctx, `SELECT is_active FROM mcp_connections WHERE id = ?`, connectionID).Scan(&active); err != nil {
		t.Fatalf("select active: %v", err)
	}
	if active != 0 {
		t.Fatalf("mcp is_active = %d, want 0", active)
	}
}

func TestRefreshOAuthStatusWithValidationActivatesNeedsAuthPluginAfterProbe(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000223"
	connectionID := "00000000-0000-0000-0000-000000000224"
	manifest := Manifest{
		ID:   "github",
		Name: "GitHub",
		AuthRequirements: []AuthRequirement{{
			Key:  "oauth",
			Type: "managed_oauth",
		}},
		MCPs: []MCPResource{{
			Key:     "api",
			URL:     "https://api.github.example/mcp",
			AuthRef: "oauth",
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusNeedsAuth, false, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, auth_type, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'github_api', 'http', '{"url":"https://api.github.example/mcp"}', 0, 'oauth', ?, 'api')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000225', ?, 'mcp', 'api', ?, 'needs_auth', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_oauth_tokens(id, connection_id, authorization_endpoint, token_endpoint, access_token, expires_at)
VALUES('00000000-0000-0000-0000-000000000226', ?, 'https://auth.example/authorize', 'https://auth.example/token', 'valid-token', ?)
`, connectionID, expiresAt); err != nil {
		t.Fatalf("insert oauth token: %v", err)
	}

	updated, err := svc.RefreshOAuthStatusWithValidation(ctx, botID, installationID, func(context.Context, Installation) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RefreshOAuthStatusWithValidation() error = %v", err)
	}
	if updated.Status != StatusReady || !updated.Enabled {
		t.Fatalf("updated = status %q enabled %v, want ready enabled", updated.Status, updated.Enabled)
	}
	var active int
	if err := conn.QueryRowContext(ctx, `SELECT is_active FROM mcp_connections WHERE id = ?`, connectionID).Scan(&active); err != nil {
		t.Fatalf("select active: %v", err)
	}
	if active != 1 {
		t.Fatalf("mcp is_active = %d, want 1", active)
	}
}

func TestUpdateConfigWithValidationRestoresPreviousConfigOnProbeFailure(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000233"
	connectionID := "00000000-0000-0000-0000-000000000234"
	manifest := Manifest{
		ID:   "docs",
		Name: "Docs",
		Variables: []ConfigVar{
			{Key: "TOKEN", Required: true, Secret: true},
		},
		MCPs: []MCPResource{{
			Key:     "docs",
			Command: "docs-mcp",
			Env:     []ConfigVar{{Key: "TOKEN", DefaultValue: "${TOKEN}", Required: true, Secret: true}},
		}},
		AuthRequirements: []AuthRequirement{{
			Key:       "token",
			Type:      "user_secret",
			Variables: []string{"TOKEN"},
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{"TOKEN": "old-secret"})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'docs_docs', 'stdio', '{"command":"docs-mcp","env":{"TOKEN":"old-secret"}}', 1, ?, 'docs')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000235', ?, 'mcp', 'docs', ?, 'ready', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}

	_, err := svc.UpdateConfigWithValidation(ctx, botID, installationID, UpdateConfigRequest{
		Variables: map[string]string{"TOKEN": "bad-secret"},
	}, func(context.Context, Installation) error {
		return errors.New("401 invalid token")
	})
	if !errors.Is(err, ErrPluginMCPProbeFailed) {
		t.Fatalf("UpdateConfigWithValidation() err = %v, want ErrPluginMCPProbeFailed", err)
	}

	row, err := svc.getRow(ctx, botID, installationID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	stored, err := variablesFromConfig(row.Config)
	if err != nil {
		t.Fatalf("decode stored config: %v", err)
	}
	if stored["TOKEN"] != "old-secret" || row.Status != StatusReady || !row.Enabled {
		t.Fatalf("stored config/status = %#v status %q enabled %v, want old ready enabled", stored, row.Status, row.Enabled)
	}
	var token string
	if err := conn.QueryRowContext(ctx, `SELECT json_extract(config, '$.env.TOKEN') FROM mcp_connections WHERE id = ?`, connectionID).Scan(&token); err != nil {
		t.Fatalf("select mcp token: %v", err)
	}
	if token != "old-secret" {
		t.Fatalf("mcp TOKEN = %q, want old-secret", token)
	}
}

func TestManagedMCPResourceErrorWrapsNameConflict(t *testing.T) {
	err := managedMCPResourceError(db.ErrNotFound, "docs_api")
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("non pgx no-rows error changed: %v", err)
	}
	err = managedMCPResourceError(errors.Join(db.ErrNotFound, pgx.ErrNoRows), "docs_api")
	if !errors.Is(err, ErrManagedMCPNameConflict) {
		t.Fatalf("error = %v, want ErrManagedMCPNameConflict", err)
	}
	if !strings.Contains(err.Error(), "docs_api") {
		t.Fatalf("error should include conflicting name: %v", err)
	}
}

func TestInstallManagedMCPNameConflictDoesNotCreatePlugin(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config)
VALUES('00000000-0000-0000-0000-000000000133', ?, 'docs_docs', 'stdio', '{"command":"manual"}')
`, botID); err != nil {
		t.Fatalf("insert conflicting connection: %v", err)
	}

	_, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "docs",
			Name: "Docs",
			MCPs: []MCPResource{{
				Key:     "docs",
				Command: "docs-mcp",
			}},
		},
	})
	if !errors.Is(err, ErrManagedMCPNameConflict) {
		t.Fatalf("Install() error = %v, want ErrManagedMCPNameConflict", err)
	}
	var count int
	if scanErr := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_plugin_installations WHERE plugin_id = 'docs'`).Scan(&count); scanErr != nil {
		t.Fatalf("count plugin rows: %v", scanErr)
	}
	if count != 0 {
		t.Fatalf("plugin installation rows = %d, want 0", count)
	}
}

func TestInstallRejectsAlreadyInstalledPluginWithoutChangingMCP(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000163"
	connectionID := "00000000-0000-0000-0000-000000000164"
	insertPluginFixture(t, ctx, conn, botID, installationID, Manifest{
		ID:   "docs",
		Name: "Docs",
		MCPs: []MCPResource{{Key: "old", Command: "old-mcp"}},
	}, StatusReady, true, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'docs_old', 'stdio', '{"command":"old-mcp"}', 1, ?, 'old')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}

	_, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "docs",
			Name: "Docs",
			MCPs: []MCPResource{{Key: "new", Command: "new-mcp"}},
		},
	})
	if !errors.Is(err, ErrPluginAlreadyInstalled) {
		t.Fatalf("Install() error = %v, want ErrPluginAlreadyInstalled", err)
	}

	var command string
	if err := conn.QueryRowContext(ctx, `SELECT json_extract(config, '$.command') FROM mcp_connections WHERE id = ?`, connectionID).Scan(&command); err != nil {
		t.Fatalf("select mcp config: %v", err)
	}
	if command != "old-mcp" {
		t.Fatalf("managed MCP command = %q, want old-mcp", command)
	}
	var pluginCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_plugin_installations WHERE plugin_id = 'docs'`).Scan(&pluginCount); err != nil {
		t.Fatalf("count plugin rows: %v", err)
	}
	if pluginCount != 1 {
		t.Fatalf("plugin rows = %d, want 1", pluginCount)
	}
}

func TestInstallReusesUninstalledPluginAndRemovesStaleMCPResources(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000173"
	connectionID := "00000000-0000-0000-0000-000000000174"
	insertPluginFixture(t, ctx, conn, botID, installationID, Manifest{
		ID:   "docs",
		Name: "Docs",
		MCPs: []MCPResource{{Key: "old", Command: "old-mcp"}},
	}, StatusUninstalled, false, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'docs_old', 'stdio', '{"command":"old-mcp"}', ?, 'old')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert stale mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000175', ?, 'mcp', 'old', ?, 'ready', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert stale plugin resource: %v", err)
	}

	installed, err := svc.Install(ctx, botID, InstallRequest{
		Manifest: Manifest{
			ID:   "docs",
			Name: "Docs",
			MCPs: []MCPResource{{Key: "new", Command: "new-mcp"}},
		},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed.ID != installationID {
		t.Fatalf("installation id = %q, want reused id %q", installed.ID, installationID)
	}

	var oldCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_connections WHERE id = ?`, connectionID).Scan(&oldCount); err != nil {
		t.Fatalf("count old mcp rows: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("old managed MCP rows = %d, want 0", oldCount)
	}
	var newCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_plugin_resources WHERE installation_id = ? AND resource_key = 'new'`, installationID).Scan(&newCount); err != nil {
		t.Fatalf("count new plugin resources: %v", err)
	}
	if newCount != 1 {
		t.Fatalf("new plugin resources = %d, want 1", newCount)
	}
}

func TestManifestScopesOverrideDiscoveredScopes(t *testing.T) {
	result := &mcp.DiscoveryResult{ScopesSupported: []string{"repo", "read:org", "workflow"}}
	applyRequestedScopes(result, []string{"repo", "read:org"})

	if len(result.ScopesSupported) != 2 || result.ScopesSupported[0] != "repo" || result.ScopesSupported[1] != "read:org" {
		t.Fatalf("scopes = %#v, want manifest scopes", result.ScopesSupported)
	}
}

func TestWithMCPProbeMetadataRecordsError(t *testing.T) {
	metadata := withMCPProbeMetadata(map[string]any{"display_name": "Docs MCP"}, "error", nil, "401 invalid or expired token")

	if metadata["probe_status"] != "error" {
		t.Fatalf("probe_status = %#v, want error", metadata["probe_status"])
	}
	if metadata["probe_error"] != "401 invalid or expired token" {
		t.Fatalf("probe_error = %#v, want probe message", metadata["probe_error"])
	}
	if metadata["tools_count"] != 0 {
		t.Fatalf("tools_count = %#v, want 0", metadata["tools_count"])
	}
	if metadata["display_name"] != "Docs MCP" {
		t.Fatalf("display_name was not preserved: %#v", metadata["display_name"])
	}
}

func TestWithMCPProbeMetadataClearsPreviousErrorOnSuccess(t *testing.T) {
	metadata := withMCPProbeMetadata(map[string]any{"probe_error": "old error"}, "connected", []mcp.ToolDescriptor{
		{Name: "search", InputSchema: map[string]any{"type": "object"}},
	}, "")

	if metadata["probe_status"] != "connected" {
		t.Fatalf("probe_status = %#v, want connected", metadata["probe_status"])
	}
	if _, ok := metadata["probe_error"]; ok {
		t.Fatalf("probe_error should be cleared on success: %#v", metadata)
	}
	if metadata["tools_count"] != 1 {
		t.Fatalf("tools_count = %#v, want 1", metadata["tools_count"])
	}
}

func TestNormalizeInstallationDoesNotDecorateMCPResourceWhenPluginDisabled(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000263"
	connectionID := "00000000-0000-0000-0000-000000000264"
	manifest := Manifest{
		ID:   "stripe",
		Name: "Stripe",
		MCPs: []MCPResource{{Key: "stripe", URL: "https://mcp.stripe.com"}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, false, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, status, tools_cache, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'stripe_stripe', 'http', '{"url":"https://mcp.stripe.com"}', 0, 'connected', '[{"name":"stripe_api_read","inputSchema":{"type":"object"}}]', ?, 'stripe')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000265', ?, 'mcp', 'stripe', ?, 'ready', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}

	row, err := svc.getRow(ctx, botID, installationID)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	installation, err := svc.normalizeInstallation(ctx, row)
	if err != nil {
		t.Fatalf("normalize installation: %v", err)
	}
	if len(installation.Resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(installation.Resources))
	}
	resource := installation.Resources[0]
	if resource.Status != StatusReady {
		t.Fatalf("resource status = %q, want stored ready status", resource.Status)
	}
	if _, ok := resource.Metadata["probe_status"]; ok {
		t.Fatalf("disabled plugin should not inherit stale probe metadata: %#v", resource.Metadata)
	}
	if _, ok := resource.Metadata["tools_count"]; ok {
		t.Fatalf("disabled plugin should not inherit stale tool count: %#v", resource.Metadata)
	}
}

func TestPluginSkillRawAddsFrontmatterAndOwnership(t *testing.T) {
	row := sqlc.BotPluginInstallation{
		ID:         pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true},
		PluginID:   "github",
		PluginName: "GitHub",
	}
	raw := pluginSkillRaw(SkillEntry{
		ID:          "github",
		Name:        "github",
		Description: "Use GitHub.",
		Content:     "# GitHub\n\nUse the connected app.",
	}, "github", row)

	parsed := skillset.ParseFile(raw, "")
	if parsed.Name != "github" {
		t.Fatalf("parsed name = %q, want github", parsed.Name)
	}
	if parsed.Description != "Use GitHub." {
		t.Fatalf("parsed description = %q", parsed.Description)
	}
	owner, ok := parsed.Metadata["managed_by_plugin"].(map[string]any)
	if !ok {
		t.Fatalf("managed_by_plugin metadata missing: %#v", parsed.Metadata)
	}
	if owner["plugin_id"] != "github" {
		t.Fatalf("plugin_id = %#v, want github", owner["plugin_id"])
	}
}

func TestCanDeletePluginSkillRequiresMatchingOwnerMarker(t *testing.T) {
	dir := path.Join(skillset.ManagedDir(), "github")
	client := &pluginSkillFileClient{
		files: map[string]string{
			path.Join(dir, ".memoh-plugin-owner.json"): `{"installation_id":"install-1"}`,
		},
	}

	if !canDeletePluginSkill(context.Background(), client, dir, "install-1") {
		t.Fatal("expected matching owner marker to allow deletion")
	}
	if canDeletePluginSkill(context.Background(), client, dir, "install-2") {
		t.Fatal("expected mismatched owner marker to block deletion")
	}
}

func TestEnsurePluginSkillWritableRejectsExistingUnownedSkill(t *testing.T) {
	dir := path.Join(skillset.ManagedDir(), "github")
	client := &pluginSkillFileClient{
		files: map[string]string{
			path.Join(dir, "SKILL.md"): "# Existing",
		},
	}

	if err := ensurePluginSkillWritable(context.Background(), client, dir, "install-1", "github"); err == nil {
		t.Fatal("expected existing unowned skill to be rejected")
	}
}

func TestEnsurePluginSkillWritableAllowsMatchingOwner(t *testing.T) {
	dir := path.Join(skillset.ManagedDir(), "github")
	client := &pluginSkillFileClient{
		files: map[string]string{
			path.Join(dir, ".memoh-plugin-owner.json"): `{"installation_id":"install-1"}`,
			path.Join(dir, "SKILL.md"):                 "# Existing",
		},
	}

	if err := ensurePluginSkillWritable(context.Background(), client, dir, "install-1", "github"); err != nil {
		t.Fatalf("expected matching owner to be writable: %v", err)
	}
}

func TestPurgeDeletesManagedMCPBeforeInstallation(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000153"
	connectionID := "00000000-0000-0000-0000-000000000154"
	manifest := Manifest{
		ID:   "docs",
		Name: "Docs",
		MCPs: []MCPResource{{Key: "docs", Command: "docs-mcp"}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'docs_docs', 'stdio', '{"command":"docs-mcp"}', ?, 'docs')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000155', ?, 'mcp', 'docs', ?, 'ready', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}

	if err := svc.Purge(ctx, botID, installationID); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_connections WHERE id = ?`, connectionID).Scan(&count); err != nil {
		t.Fatalf("count mcp connections: %v", err)
	}
	if count != 0 {
		t.Fatalf("managed MCP rows = %d, want 0", count)
	}
}

func TestPurgeDeletesManagedMCPWhenBundledSkillCleanupCannotRun(t *testing.T) {
	ctx := context.Background()
	conn, svc := newPluginServiceTestDB(t, ctx)
	botID := "00000000-0000-0000-0000-000000000102"
	installationID := "00000000-0000-0000-0000-000000000243"
	connectionID := "00000000-0000-0000-0000-000000000244"
	manifest := Manifest{
		ID:   "docs",
		Name: "Docs",
		MCPs: []MCPResource{{Key: "docs", Command: "docs-mcp"}},
		BundledSkills: []SkillEntry{{
			ID:      "docs",
			Name:    "docs",
			Content: "# Docs",
		}},
	}
	insertPluginFixture(t, ctx, conn, botID, installationID, manifest, StatusReady, true, map[string]string{})
	if _, err := conn.ExecContext(ctx, `
INSERT INTO mcp_connections(id, bot_id, name, type, config, is_active, managed_by_plugin_installation_id, managed_resource_key)
VALUES(?, ?, 'docs_docs', 'stdio', '{"command":"docs-mcp"}', 1, ?, 'docs')
`, connectionID, botID, installationID); err != nil {
		t.Fatalf("insert mcp connection: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_resources(id, installation_id, resource_type, resource_key, resource_id, status, metadata)
VALUES('00000000-0000-0000-0000-000000000245', ?, 'mcp', 'docs', ?, 'ready', '{}')
`, installationID, connectionID); err != nil {
		t.Fatalf("insert plugin resource: %v", err)
	}

	if err := svc.Purge(ctx, botID, installationID); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_connections WHERE id = ?`, connectionID).Scan(&count); err != nil {
		t.Fatalf("count mcp connections: %v", err)
	}
	if count != 0 {
		t.Fatalf("managed MCP rows = %d, want 0", count)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_plugin_installations WHERE id = ?`, installationID).Scan(&count); err != nil {
		t.Fatalf("count plugin installations: %v", err)
	}
	if count != 0 {
		t.Fatalf("plugin rows = %d, want 0", count)
	}
}

type pluginSkillFileClient struct {
	files map[string]string
}

func (c *pluginSkillFileClient) ReadRaw(_ context.Context, filePath string) (io.ReadCloser, error) {
	raw, ok := c.files[filePath]
	if !ok {
		return nil, bridge.ErrNotFound
	}
	return io.NopCloser(strings.NewReader(raw)), nil
}

func newPluginServiceTestDB(t *testing.T, ctx context.Context) (*sql.DB, *Service) {
	t.Helper()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, pluginServiceTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO users(id, email, role) VALUES('00000000-0000-0000-0000-000000000101', 'plugin-test@example.com', 'member');
INSERT INTO bots(id, owner_user_id, type, name, display_name)
VALUES('00000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000101', 'personal', 'plugin-test-bot', 'Plugin Test Bot');
`); err != nil {
		t.Fatalf("insert bot fixture: %v", err)
	}
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	queries := sqlitestore.NewQueries(store)
	logger := slog.New(slog.DiscardHandler)
	mcpService := mcp.NewConnectionService(logger, queries)
	oauthService := mcp.NewOAuthService(logger, queries, "https://memoh.example/oauth/mcp/callback")
	return conn, NewService(logger, queries, mcpService, oauthService, NewOAuthClientRegistry(logger, config.Config{OAuthClients: config.OAuthClientsConfig{ConfigPath: t.TempDir() + "/missing.toml"}}), BridgeProvider{})
}

func insertPluginFixture(t *testing.T, ctx context.Context, conn *sql.DB, botID, installationID string, manifest Manifest, status string, enabled bool, variables map[string]string) {
	t.Helper()
	manifest = normalizeManifest(manifest)
	configPayload, err := encodeJSON(map[string]any{"variables": variables})
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}
	metadataPayload, err := encodeJSON(manifestMetadata(manifest))
	if err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
	manifestPayload, err := encodeJSON(manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_plugin_installations(id, bot_id, plugin_id, plugin_name, version, status, enabled, config, metadata, manifest)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, installationID, botID, manifest.ID, manifest.Name, manifest.Version, status, enabledValue, string(configPayload), string(metadataPayload), string(manifestPayload)); err != nil {
		t.Fatalf("insert plugin fixture: %v", err)
	}
}

const pluginServiceTestSchema = `
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT,
  role TEXT NOT NULL DEFAULT 'member'
);

CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'personal',
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE bot_plugin_installations (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  plugin_id TEXT NOT NULL,
  plugin_name TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ready',
  enabled INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  manifest TEXT NOT NULL DEFAULT '{}',
  installed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_plugin_installations_unique UNIQUE (bot_id, plugin_id)
);

CREATE TABLE mcp_connections (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  is_active INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'unknown',
  tools_cache TEXT NOT NULL DEFAULT '[]',
  last_probed_at TEXT,
  status_message TEXT NOT NULL DEFAULT '',
  auth_type TEXT NOT NULL DEFAULT 'none',
  managed_by_plugin_installation_id TEXT REFERENCES bot_plugin_installations(id) ON DELETE SET NULL,
  managed_resource_key TEXT NOT NULL DEFAULT '',
  visible INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT mcp_connections_type_check CHECK (type IN ('stdio', 'http', 'sse')),
  CONSTRAINT mcp_connections_unique UNIQUE (bot_id, name)
);

CREATE TABLE bot_plugin_resources (
  id TEXT PRIMARY KEY,
  installation_id TEXT NOT NULL REFERENCES bot_plugin_installations(id) ON DELETE CASCADE,
  resource_type TEXT NOT NULL,
  resource_key TEXT NOT NULL,
  resource_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_plugin_resources_unique UNIQUE (installation_id, resource_type, resource_key)
);

CREATE TABLE mcp_oauth_tokens (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL UNIQUE REFERENCES mcp_connections(id) ON DELETE CASCADE,
  resource_metadata_url TEXT NOT NULL DEFAULT '',
  authorization_server_url TEXT NOT NULL DEFAULT '',
  authorization_endpoint TEXT NOT NULL DEFAULT '',
  token_endpoint TEXT NOT NULL DEFAULT '',
  registration_endpoint TEXT NOT NULL DEFAULT '',
  scopes_supported TEXT NOT NULL DEFAULT '{}',
  client_id TEXT NOT NULL DEFAULT '',
  client_secret TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT 'Bearer',
  expires_at TEXT,
  scope TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  state_param TEXT NOT NULL DEFAULT '',
  resource_uri TEXT NOT NULL DEFAULT '',
  redirect_uri TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
