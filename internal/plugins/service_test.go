package plugins

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/mcp"
)

func TestManifestInstallCommandsAcceptStringOrList(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "string",
			raw:  `{"id":"plugin","name":"Plugin","author":{"name":"Memoh"},"install":" sh scripts/install.sh "}`,
			want: []string{"sh scripts/install.sh"},
		},
		{
			name: "list",
			raw:  `{"id":"plugin","name":"Plugin","author":{"name":"Memoh"},"install":[" sh scripts/a.sh ","","python3 scripts/b.py"]}`,
			want: []string{"sh scripts/a.sh", "python3 scripts/b.py"},
		},
		{
			name: "null",
			raw:  `{"id":"plugin","name":"Plugin","author":{"name":"Memoh"},"install":null}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manifest Manifest
			if err := json.Unmarshal([]byte(tt.raw), &manifest); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if !reflect.DeepEqual([]string(manifest.Install), tt.want) {
				t.Fatalf("install = %#v, want %#v", []string(manifest.Install), tt.want)
			}
		})
	}
}

func TestManifestInstallCommandsRejectInvalidShape(t *testing.T) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(`{"id":"plugin","install":7}`), &manifest); err == nil {
		t.Fatal("expected invalid install shape to fail")
	}
}

func TestNormalizeManifestTrimsInstallCommands(t *testing.T) {
	manifest := NormalizeManifest(Manifest{
		ID:      "Plugin.ID",
		Name:    " Plugin ",
		Install: InstallCommands{" sh scripts/install.sh ", "", "python3 scripts/setup.py"},
	})

	want := []string{"sh scripts/install.sh", "python3 scripts/setup.py"}
	if !reflect.DeepEqual([]string(manifest.Install), want) {
		t.Fatalf("install = %#v, want %#v", []string(manifest.Install), want)
	}
}

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

func TestManifestScopesOverrideDiscoveredScopes(t *testing.T) {
	result := &mcp.DiscoveryResult{ScopesSupported: []string{"repo", "read:org", "workflow"}}
	applyRequestedScopes(result, []string{"repo", "read:org"})

	if len(result.ScopesSupported) != 2 || result.ScopesSupported[0] != "repo" || result.ScopesSupported[1] != "read:org" {
		t.Fatalf("scopes = %#v, want manifest scopes", result.ScopesSupported)
	}
}
