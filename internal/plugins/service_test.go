package plugins

import (
	"strings"
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

func TestValidatePackageReferencesRequiresNamespacedUniqueIdentity(t *testing.T) {
	reference := PackageReference{RegistryID: "memoh", PackageID: "github"}
	if err := ValidatePackageReferences([]PackageReference{reference}); err != nil {
		t.Fatalf("ValidatePackageReferences(valid) error = %v", err)
	}
	if got := PackageReferenceIdentity(reference); got != "memoh/github" {
		t.Fatalf("PackageReferenceIdentity() = %q", got)
	}
	if err := ValidatePackageReferences([]PackageReference{reference, reference}); err == nil {
		t.Fatal("ValidatePackageReferences() accepted a duplicate reference")
	}
	dotted := PackageReference{RegistryID: "openai.api", PackageID: "documents.v2"}
	if err := ValidatePackageReferences([]PackageReference{dotted}); err != nil {
		t.Fatalf("ValidatePackageReferences(dotted) error = %v", err)
	}
	reference.RegistryID = "Not Valid"
	if err := ValidatePackageReferences([]PackageReference{reference}); err == nil {
		t.Fatal("ValidatePackageReferences() accepted an invalid Registry ID")
	}
	for _, invalid := range []PackageReference{
		{RegistryID: "user", PackageID: "github"},
		{RegistryID: "memoh", PackageID: "github..v2"},
		{RegistryID: "memoh", PackageID: "nul.txt"},
	} {
		if err := ValidatePackageReferences([]PackageReference{invalid}); err == nil {
			t.Fatalf("ValidatePackageReferences() accepted invalid reference %+v", invalid)
		}
	}
}

func TestValidateInstalledPackagesRequiresPinnedManifestPackages(t *testing.T) {
	references := []PackageReference{{RegistryID: "memoh", PackageID: "notion"}}
	installed := []InstalledPackage{{RegistryID: "memoh", PackageID: "notion", Revision: strings.Repeat("b", 64)}}
	if err := validateInstalledPackages(references, installed); err != nil {
		t.Fatalf("validateInstalledPackages(valid) error = %v", err)
	}
	if err := validateInstalledPackages(references, nil); err == nil {
		t.Fatal("validateInstalledPackages() accepted a missing Package")
	}
	installed[0].Revision = "not-a-digest"
	if err := validateInstalledPackages(references, installed); err == nil {
		t.Fatal("validateInstalledPackages() accepted an invalid revision")
	}
}

func TestValidateInstalledSkillsRequiresEveryReferencedPackage(t *testing.T) {
	packages := []PackageReference{{RegistryID: "memoh", PackageID: "notion"}}
	if err := validateInstalledSkills(packages, nil); err == nil {
		t.Fatal("validateInstalledSkills() accepted a Package without installed Skills")
	}
	skill := InstalledSkill{RegistryID: "memoh", PackageID: "notion", SkillID: "meeting"}
	if err := validateInstalledSkills(packages, []InstalledSkill{skill}); err != nil {
		t.Fatalf("validateInstalledSkills(valid) error = %v", err)
	}
	skill.PackageID = "other"
	if err := validateInstalledSkills(packages, []InstalledSkill{skill}); err == nil {
		t.Fatal("validateInstalledSkills() accepted a Skill outside the referenced Package")
	}
}
