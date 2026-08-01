package plugins

import (
	"testing"

	"github.com/memohai/memoh/internal/mcp"
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

func TestManifestScopesOverrideDiscoveredScopes(t *testing.T) {
	result := &mcp.DiscoveryResult{ScopesSupported: []string{"repo", "read:org", "workflow"}}
	applyRequestedScopes(result, []string{"repo", "read:org"})

	if len(result.ScopesSupported) != 2 || result.ScopesSupported[0] != "repo" || result.ScopesSupported[1] != "read:org" {
		t.Fatalf("scopes = %#v, want manifest scopes", result.ScopesSupported)
	}
}

func TestValidateSkillReferencesRequiresNamespacedUniqueIdentity(t *testing.T) {
	reference := SkillReference{RegistryID: "memoh", PackageID: "github", SkillID: "github"}
	if err := validateSkillReferences([]SkillReference{reference}); err != nil {
		t.Fatalf("validateSkillReferences(valid) error = %v", err)
	}
	if got := SkillReferenceIdentity(reference); got != "memoh/github/github" {
		t.Fatalf("SkillReferenceIdentity() = %q", got)
	}
	if err := validateSkillReferences([]SkillReference{reference, reference}); err == nil {
		t.Fatal("validateSkillReferences() accepted a duplicate reference")
	}
	reference.RegistryID = "Not Valid"
	if err := validateSkillReferences([]SkillReference{reference}); err == nil {
		t.Fatal("validateSkillReferences() accepted an invalid Registry ID")
	}
}

func TestOwnsRegistrySkillKeepsSharedAndDisabledPluginReferences(t *testing.T) {
	sourcePath := "/data/skills/memoh/notion/meeting/SKILL.md"
	resource := Resource{Type: "skill", ResourceID: sourcePath, Status: "installed"}

	for name, installation := range map[string]Installation{
		"ready":      {Status: StatusReady, Enabled: true, Resources: []Resource{resource}},
		"disabled":   {Status: StatusDisabled, Enabled: false, Resources: []Resource{resource}},
		"needs auth": {Status: StatusNeedsAuth, Enabled: false, Resources: []Resource{resource}},
	} {
		t.Run(name, func(t *testing.T) {
			if !OwnsRegistrySkill([]Installation{installation}, sourcePath) {
				t.Fatal("active installation did not retain Registry Skill ownership")
			}
		})
	}

	if OwnsRegistrySkill([]Installation{{Status: StatusUninstalled, Resources: []Resource{resource}}}, sourcePath) {
		t.Fatal("uninstalled Plugin retained Registry Skill ownership")
	}
	if OwnsRegistrySkill([]Installation{{Status: StatusReady, Resources: []Resource{resource}}}, "/data/skills/memoh/notion/other/SKILL.md") {
		t.Fatal("Plugin owned an unrelated Registry Skill")
	}
	if OwnsRegistrySkill([]Installation{{Status: StatusReady, Resources: []Resource{resource}}}, "/data/skills/user/personal/meeting/SKILL.md") {
		t.Fatal("Plugin owned a user-authored Skill")
	}
}
