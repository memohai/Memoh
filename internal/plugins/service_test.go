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

func TestValidateSkillReferencesRequiresNamespacedUniqueIdentity(t *testing.T) {
	reference := SkillReference{RegistryID: "memoh", PackageID: "github", SkillID: "github"}
	if err := ValidateSkillReferences([]SkillReference{reference}); err != nil {
		t.Fatalf("ValidateSkillReferences(valid) error = %v", err)
	}
	if got := SkillReferenceIdentity(reference); got != "memoh/github/github" {
		t.Fatalf("SkillReferenceIdentity() = %q", got)
	}
	if err := ValidateSkillReferences([]SkillReference{reference, reference}); err == nil {
		t.Fatal("ValidateSkillReferences() accepted a duplicate reference")
	}
	dotted := SkillReference{RegistryID: "openai.api", PackageID: "documents.v2", SkillID: "pdf.reader"}
	if err := ValidateSkillReferences([]SkillReference{dotted}); err != nil {
		t.Fatalf("ValidateSkillReferences(dotted) error = %v", err)
	}
	reference.RegistryID = "Not Valid"
	if err := ValidateSkillReferences([]SkillReference{reference}); err == nil {
		t.Fatal("ValidateSkillReferences() accepted an invalid Registry ID")
	}
	for _, invalid := range []SkillReference{
		{RegistryID: "user", PackageID: "github", SkillID: "github"},
		{RegistryID: "memoh", PackageID: "github..v2", SkillID: "github"},
		{RegistryID: "memoh", PackageID: "github", SkillID: "nul.txt"},
	} {
		if err := ValidateSkillReferences([]SkillReference{invalid}); err == nil {
			t.Fatalf("ValidateSkillReferences() accepted invalid reference %+v", invalid)
		}
	}
}

func TestOwnsRegistrySkillAtTargetKeepsSharedAndDisabledPluginReferences(t *testing.T) {
	sourcePath := "/data/skills/memoh/notion/meeting/SKILL.md"
	resource := Resource{Type: "skill", ResourceID: sourcePath, Status: "installed"}

	for name, installation := range map[string]Installation{
		"ready":      {Status: StatusReady, Enabled: true, Metadata: map[string]any{"workspace_target_id": "remote-target"}, Resources: []Resource{resource}},
		"disabled":   {Status: StatusDisabled, Enabled: false, Metadata: map[string]any{"workspace_target_id": "remote-target"}, Resources: []Resource{resource}},
		"needs auth": {Status: StatusNeedsAuth, Enabled: false, Metadata: map[string]any{"workspace_target_id": "remote-target"}, Resources: []Resource{resource}},
	} {
		t.Run(name, func(t *testing.T) {
			if !OwnsRegistrySkillAtTarget([]Installation{installation}, sourcePath, "remote-target") {
				t.Fatal("active installation did not retain Registry Skill ownership")
			}
		})
	}

	metadata := map[string]any{"workspace_target_id": "remote-target"}
	if OwnsRegistrySkillAtTarget([]Installation{{Status: StatusUninstalled, Metadata: metadata, Resources: []Resource{resource}}}, sourcePath, "remote-target") {
		t.Fatal("uninstalled Plugin retained Registry Skill ownership")
	}
	if OwnsRegistrySkillAtTarget([]Installation{{Status: StatusReady, Metadata: metadata, Resources: []Resource{resource}}}, "/data/skills/memoh/notion/other/SKILL.md", "remote-target") {
		t.Fatal("Plugin owned an unrelated Registry Skill")
	}
	if OwnsRegistrySkillAtTarget([]Installation{{Status: StatusReady, Metadata: metadata, Resources: []Resource{resource}}}, "/data/skills/user/personal/meeting/SKILL.md", "remote-target") {
		t.Fatal("Plugin owned a user-authored Skill")
	}
	if OwnsRegistrySkillAtTarget([]Installation{{Status: StatusReady, Metadata: metadata, Resources: []Resource{resource}}}, sourcePath, "native") {
		t.Fatal("Plugin Registry Skill ownership leaked into another workspace target")
	}
}

func TestValidateSkillArtifactsRequiresVerifiedInstallResult(t *testing.T) {
	reference := SkillReference{RegistryID: "memoh", PackageID: "notion", SkillID: "meeting"}
	identity := SkillReferenceIdentity(reference)
	valid := SkillArtifactMetadata{
		RegistryRevision: strings.Repeat("b", 64),
		InstallID:        "memoh+notion+meeting",
		ArtifactDigest:   strings.Repeat("a", 64),
		FilesWritten:     2,
	}
	if err := validateSkillArtifacts([]SkillReference{reference}, map[string]SkillArtifactMetadata{identity: valid}); err != nil {
		t.Fatalf("validateSkillArtifacts(valid) error = %v", err)
	}
	if err := validateSkillArtifacts([]SkillReference{reference}, nil); err == nil {
		t.Fatal("validateSkillArtifacts() accepted missing Artifact metadata")
	}
	invalid := valid
	invalid.ArtifactDigest = "not-a-digest"
	if err := validateSkillArtifacts([]SkillReference{reference}, map[string]SkillArtifactMetadata{identity: invalid}); err == nil {
		t.Fatal("validateSkillArtifacts() accepted an invalid digest")
	}
}

func TestCheckSkillArtifactConflictsRejectsDifferentSharedDigests(t *testing.T) {
	identity := "memoh/notion/meeting"
	installation := Installation{
		PluginID: "calendar",
		Status:   StatusReady,
		Metadata: map[string]any{"workspace_target_id": "native"},
		Resources: []Resource{{
			Type: "skill",
			Key:  identity,
			Metadata: map[string]any{
				"artifact_digest": strings.Repeat("a", 64),
			},
		}},
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation},
		"notion",
		"native",
		map[string]string{identity: strings.Repeat("b", 64)},
	); err == nil {
		t.Fatal("checkSkillArtifactConflicts() accepted a different shared Artifact")
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation},
		"notion",
		"native",
		map[string]string{identity: strings.Repeat("a", 64)},
	); err != nil {
		t.Fatalf("checkSkillArtifactConflicts() rejected the same Artifact: %v", err)
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation},
		"notion",
		"remote-target",
		map[string]string{identity: strings.Repeat("b", 64)},
	); err != nil {
		t.Fatalf("different workspace target inherited an Artifact conflict: %v", err)
	}
}

func TestCheckSkillArtifactConflictsAllowsTargetPluginUpdate(t *testing.T) {
	identity := "memoh/notion/meeting"
	installation := Installation{
		PluginID: "notion",
		Status:   StatusDisabled,
		Metadata: map[string]any{"workspace_target_id": "native"},
		Resources: []Resource{{
			Type: "skill",
			Key:  identity,
			Metadata: map[string]any{
				"artifact_digest": strings.Repeat("a", 64),
			},
		}},
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation}, "notion", "native", map[string]string{identity: strings.Repeat("b", 64)},
	); err != nil {
		t.Fatalf("target Plugin could not update its own Artifact: %v", err)
	}
}

func TestCheckSkillArtifactConflictsRejectsPluginMoveAcrossWorkspaceTargets(t *testing.T) {
	installation := Installation{
		PluginID: "notion",
		Status:   StatusReady,
		Metadata: map[string]any{"workspace_target_id": "native"},
	}
	err := checkSkillArtifactConflicts(
		[]Installation{installation},
		"notion",
		"remote-target",
		nil,
	)
	if err == nil {
		t.Fatal("cross-target Plugin reinstall was accepted despite bot/plugin uniqueness")
	}
}

func TestCheckSkillArtifactConflictsRejectsDirectSkillUpdate(t *testing.T) {
	identity := "memoh/notion/meeting"
	installation := Installation{
		PluginID: "notion",
		Status:   StatusReady,
		Metadata: map[string]any{"workspace_target_id": "native"},
		Resources: []Resource{{
			Type: "skill",
			Key:  identity,
			Metadata: map[string]any{
				"artifact_digest": strings.Repeat("a", 64),
			},
		}},
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation}, "", "native", map[string]string{identity: strings.Repeat("b", 64)},
	); err == nil {
		t.Fatal("direct Skill update replaced an Artifact locked by an installed Plugin")
	}
	if err := checkSkillArtifactConflicts(
		[]Installation{installation}, "", "native", map[string]string{identity: strings.Repeat("a", 64)},
	); err != nil {
		t.Fatalf("direct Skill update rejected the Artifact already used by a Plugin: %v", err)
	}
}
