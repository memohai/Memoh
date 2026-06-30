package plugins

import (
	"context"
	"io"
	"path"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
)

func TestUserSecretTemplateConfigRequiresProvidedVariable(t *testing.T) {
	manifest := Manifest{
		Variables: []ConfigVar{
			{Key: "NOTION_TOKEN", Required: true, Secret: true},
		},
		AuthRequirements: []AuthRequirement{
			{Key: "notion", Type: "user_secret", Variables: []string{"NOTION_TOKEN"}},
		},
	}
	resource := MCPResource{
		AuthRef: "notion",
		Env: []ConfigVar{
			{Key: "NOTION_TOKEN", DefaultValue: "${NOTION_TOKEN}", Required: true, Secret: true},
		},
		Headers: []ConfigVar{
			{Key: "Authorization", DefaultValue: "Bearer ${NOTION_TOKEN}", Required: true, Secret: true},
		},
	}

	if status := (&Service{}).evaluateInitialStatus(Manifest{
		Variables:        manifest.Variables,
		AuthRequirements: manifest.AuthRequirements,
		MCPs:             []MCPResource{resource},
	}, map[string]string{}); status != StatusNeedsConfig {
		t.Fatalf("status = %q, want %q", status, StatusNeedsConfig)
	}

	authReq := manifest.AuthRequirements[0]
	req := buildMCPConnectionRequest(manifest, resource, authReq, map[string]string{"NOTION_TOKEN": "abc123"})
	if got := req.Env["NOTION_TOKEN"]; got != "abc123" {
		t.Fatalf("env NOTION_TOKEN = %q, want abc123", got)
	}
	if got := req.Headers["Authorization"]; got != "Bearer abc123" {
		t.Fatalf("authorization header = %q, want Bearer abc123", got)
	}
	if status := (&Service{}).evaluateInitialStatus(Manifest{
		Variables:        manifest.Variables,
		AuthRequirements: manifest.AuthRequirements,
		MCPs:             []MCPResource{resource},
	}, map[string]string{"NOTION_TOKEN": "abc123"}); status != StatusReady {
		t.Fatalf("status = %q, want %q", status, StatusReady)
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

func TestManifestScopesOverrideDiscoveredScopes(t *testing.T) {
	result := &mcp.DiscoveryResult{ScopesSupported: []string{"repo", "read:org", "workflow"}}
	applyRequestedScopes(result, []string{"repo", "read:org"})

	if len(result.ScopesSupported) != 2 || result.ScopesSupported[0] != "repo" || result.ScopesSupported[1] != "read:org" {
		t.Fatalf("scopes = %#v, want manifest scopes", result.ScopesSupported)
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

type pluginSkillFileClient struct {
	files map[string]string
}

func (c *pluginSkillFileClient) ReadRaw(_ context.Context, filePath string) (io.ReadCloser, error) {
	raw, ok := c.files[filePath]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return io.NopCloser(strings.NewReader(raw)), nil
}
