package skills

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

func TestParseFileFallbacks(t *testing.T) {
	raw := "# Use this skill\n\nDo something useful."
	got := ParseFile(raw, "plain-skill")

	if got.Name != "plain-skill" {
		t.Fatalf("expected name plain-skill, got %q", got.Name)
	}
	if got.Description != "plain-skill" {
		t.Fatalf("expected description plain-skill, got %q", got.Description)
	}
	if got.Content != raw {
		t.Fatalf("expected content to keep original markdown, got %q", got.Content)
	}
}

func TestParseFileMalformedMetadataFailsRuntimeClosed(t *testing.T) {
	raw := "---\nname: bad-metadata\ndescription: Bad metadata\nmetadata: runtime-visible\n---\n\n# Bad"
	parsed := ParseFile(raw, "fallback")
	entry := entryFromParsed(parsed, raw, Root{
		Path:    ManagedDirPath,
		Kind:    SourceKindManaged,
		Managed: true,
	}, pathJoin(ManagedDirPath, "bad-metadata", "SKILL.md"))
	entry.State = StateEffective
	entry = NormalizeRuntimeUsability(entry)

	if parsed.Name != "bad-metadata" {
		t.Fatalf("parsed.Name = %q, want bad-metadata", parsed.Name)
	}
	if entry.RuntimeUsable {
		t.Fatal("malformed metadata entry is runtime usable, want fail-closed")
	}
	if entry.RuntimeUnusableReason != "metadata" {
		t.Fatalf("RuntimeUnusableReason = %q, want metadata", entry.RuntimeUnusableReason)
	}
}

func TestResolveSupportsDisabledFallbackAndShadowing(t *testing.T) {
	items := []Entry{
		{Name: "alpha", SourcePath: "/data/skills/alpha/SKILL.md", Managed: true, SourceKind: SourceKindManaged},
		{Name: "alpha", SourcePath: "/data/.agents/skills/alpha/SKILL.md", SourceKind: SourceKindCompat},
		{Name: "beta", SourcePath: "/data/.agents/skills/beta/SKILL.md", SourceKind: SourceKindCompat},
	}

	resolved := resolve(items, map[string]indexOverride{
		"/data/skills/alpha/SKILL.md": {Disabled: true},
	})

	managedAlpha, ok := findBySourcePath(resolved, "/data/skills/alpha/SKILL.md")
	if !ok {
		t.Fatalf("managed alpha not found in resolved items")
	}
	if managedAlpha.State != StateDisabled {
		t.Fatalf("managed alpha state = %q, want disabled", managedAlpha.State)
	}
	compatAlpha, ok := findBySourcePath(resolved, "/data/.agents/skills/alpha/SKILL.md")
	if !ok {
		t.Fatalf("compat alpha not found in resolved items")
	}
	if compatAlpha.State != StateEffective {
		t.Fatalf("compat alpha state = %q, want effective", compatAlpha.State)
	}
	beta, ok := findBySourcePath(resolved, "/data/.agents/skills/beta/SKILL.md")
	if !ok {
		t.Fatalf("beta not found in resolved items")
	}
	if beta.State != StateEffective {
		t.Fatalf("beta state = %q, want effective", beta.State)
	}
}

func TestListReadsFullRawContentAndWritesIndex(t *testing.T) {
	client := newFakeClient()
	client.listings[ManagedDirPath] = []*pb.FileEntry{{Path: "alpha", IsDir: true}}
	client.files[pathJoin(ManagedDirPath, "alpha", "SKILL.md")] = "---\nname: alpha\ndescription: Alpha\n---\n\n" + strings.Repeat("A", 7000)

	items, err := List(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].Raw) <= 7000 {
		t.Fatalf("expected full raw content, got len=%d", len(items[0].Raw))
	}
	if _, ok := client.files[IndexFilePath]; !ok {
		t.Fatalf("expected index file to be written")
	}
}

func TestApplyActionAdoptAndDisable(t *testing.T) {
	client := newFakeClient()
	externalPath := pathJoin("/data/.agents/skills", "alpha", "SKILL.md")
	client.listings["/data/.agents/skills"] = []*pb.FileEntry{{Path: "alpha", IsDir: true}}
	client.files[externalPath] = "---\nname: alpha\ndescription: Alpha\n---\n\n# Alpha"

	if err := ApplyAction(context.Background(), client, nil, ActionRequest{
		Action:     ActionAdopt,
		TargetPath: externalPath,
	}); err != nil {
		t.Fatalf("adopt returned error: %v", err)
	}
	if _, ok := client.files[pathJoin(ManagedDirPath, "alpha", "SKILL.md")]; !ok {
		t.Fatalf("expected managed copy after adopt")
	}

	if err := ApplyAction(context.Background(), client, nil, ActionRequest{
		Action:     ActionDisable,
		TargetPath: externalPath,
	}); err != nil {
		t.Fatalf("disable returned error: %v", err)
	}
	idx := readIndex(context.Background(), client)
	if !idx.Overrides[externalPath].Disabled {
		t.Fatalf("expected disabled override for %s", externalPath)
	}
}

func TestApplyActionAdoptRejectsInvalidManagedName(t *testing.T) {
	client := newFakeClient()
	externalPath := pathJoin("/data/.agents/skills", "escape", "SKILL.md")
	client.listings["/data/.agents/skills"] = []*pb.FileEntry{{Path: "escape", IsDir: true}}
	client.files[externalPath] = "---\nname: ..\ndescription: Escape\n---\n\n# Escape"

	err := ApplyAction(context.Background(), client, nil, ActionRequest{
		Action:     ActionAdopt,
		TargetPath: externalPath,
	})
	if !errors.Is(err, bridge.ErrBadRequest) {
		t.Fatalf("adopt err = %v, want ErrBadRequest", err)
	}
	if _, ok := client.files[pathJoin(ManagedDirPath, "..", "SKILL.md")]; ok {
		t.Fatalf("unexpected managed write for invalid adopted name")
	}
}

func TestIsValidNameRejectsTraversalPatterns(t *testing.T) {
	for _, name := range []string{
		"",
		".",
		"..",
		".hidden",
		"alpha..beta",
		"../escape",
		"alpha/../beta",
	} {
		if IsValidName(name) {
			t.Fatalf("IsValidName(%q) = true, want false", name)
		}
	}

	for _, name := range []string{"alpha", "alpha-beta", "alpha_beta", "alpha.beta", "registry+package+skill"} {
		if !IsValidName(name) {
			t.Fatalf("IsValidName(%q) = false, want true", name)
		}
	}
}

func TestManagedSkillDirForNameRejectsEscapingNames(t *testing.T) {
	for _, name := range []string{".", "..", ".alpha", "alpha..beta"} {
		if _, err := ManagedSkillDirForName(name); !errors.Is(err, bridge.ErrBadRequest) {
			t.Fatalf("ManagedSkillDirForName(%q) err = %v, want ErrBadRequest", name, err)
		}
	}

	dirPath, err := ManagedSkillDirForName("alpha.beta")
	if err != nil {
		t.Fatalf("ManagedSkillDirForName(valid) returned error: %v", err)
	}
	if dirPath != pathJoin(ManagedDirPath, "alpha.beta") {
		t.Fatalf("ManagedSkillDirForName(valid) = %q, want %q", dirPath, pathJoin(ManagedDirPath, "alpha.beta"))
	}
}

func TestDeletableSkillDirForSourcePath(t *testing.T) {
	flat, err := DeletableSkillDirForSourcePath(pathJoin(ManagedDirPath, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("DeletableSkillDirForSourcePath(flat) error = %v", err)
	}
	if flat != pathJoin(ManagedDirPath, "alpha") {
		t.Fatalf("DeletableSkillDirForSourcePath(flat) = %q", flat)
	}

	builtin, err := DeletableSkillDirForSourcePath(pathJoin(IndexDirPath, "skill-creator", "SKILL.md"))
	if err != nil {
		t.Fatalf("DeletableSkillDirForSourcePath(builtin) error = %v", err)
	}
	if builtin != pathJoin(IndexDirPath, "skill-creator") {
		t.Fatalf("DeletableSkillDirForSourcePath(builtin) = %q", builtin)
	}

	registrySkillDir := pathJoin(ManagedDirPath, "openai-api-curated", "docs", "xlsx")
	registry, err := DeletableSkillDirForSourcePath(pathJoin(registrySkillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("DeletableSkillDirForSourcePath(registry) error = %v", err)
	}
	if registry != registrySkillDir {
		t.Fatalf("DeletableSkillDirForSourcePath(registry) = %q", registry)
	}

	for _, sourcePath := range []string{
		"",
		"relative/SKILL.md",
		pathJoin(ManagedDirPath, "alpha", "README.md"),
		"/data/.agents/skills/alpha/SKILL.md",
		pathJoin(PluginDirPath, "github", "skills", "review", "SKILL.md"),
		pathJoin(ManagedDirPath, "..", "escape", "SKILL.md"),
	} {
		if _, err := DeletableSkillDirForSourcePath(sourcePath); !errors.Is(err, bridge.ErrBadRequest) {
			t.Fatalf("DeletableSkillDirForSourcePath(%q) err = %v, want ErrBadRequest", sourcePath, err)
		}
	}
}

func TestPrunableRegistryDirs(t *testing.T) {
	got := PrunableRegistryDirs(pathJoin(ManagedDirPath, "openai-api-curated", "docs", "xlsx"))
	want := []string{
		pathJoin(ManagedDirPath, "openai-api-curated", "docs"),
		pathJoin(ManagedDirPath, "openai-api-curated"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("PrunableRegistryDirs(registry) = %+v, want %+v", got, want)
	}
	if got := PrunableRegistryDirs(pathJoin(ManagedDirPath, "alpha")); got != nil {
		t.Fatalf("PrunableRegistryDirs(flat) = %+v, want nil", got)
	}
}

func TestPlanUpsertCreateAndRenameAndRegistryInPlace(t *testing.T) {
	create, err := PlanUpsert("---\nname: alpha\ndescription: A\n---\n\n# A\n", "")
	if err != nil {
		t.Fatalf("PlanUpsert(create) error = %v", err)
	}
	if create.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || create.RemoveDir != "" {
		t.Fatalf("PlanUpsert(create) = %+v", create)
	}

	same, err := PlanUpsert("---\nname: alpha\ndescription: A\n---\n\n# A\n", pathJoin(ManagedDirPath, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("PlanUpsert(same) error = %v", err)
	}
	if same.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || same.RemoveDir != "" {
		t.Fatalf("PlanUpsert(same) = %+v", same)
	}

	rename, err := PlanUpsert("---\nname: beta\ndescription: B\n---\n\n# B\n", pathJoin(ManagedDirPath, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("PlanUpsert(rename) error = %v", err)
	}
	if rename.WritePath != pathJoin(ManagedDirPath, "beta", "SKILL.md") {
		t.Fatalf("PlanUpsert(rename).WritePath = %q", rename.WritePath)
	}
	if rename.RemoveDir != pathJoin(ManagedDirPath, "alpha") {
		t.Fatalf("PlanUpsert(rename).RemoveDir = %q", rename.RemoveDir)
	}

	registryPath := pathJoin(ManagedDirPath, "openai-api-curated", "docs", "xlsx", "SKILL.md")
	registry, err := PlanUpsert("---\nname: xlsx\ndescription: Sheet\n---\n\n# Sheet\n", registryPath)
	if err != nil {
		t.Fatalf("PlanUpsert(registry) error = %v", err)
	}
	if registry.WritePath != registryPath || registry.RemoveDir != "" {
		t.Fatalf("PlanUpsert(registry) = %+v", registry)
	}

	builtinPath := pathJoin(IndexDirPath, "skill-creator", "SKILL.md")
	builtinSame, err := PlanUpsert("---\nname: skill-creator\ndescription: Creator\n---\n\n# Creator\n", builtinPath)
	if err != nil {
		t.Fatalf("PlanUpsert(builtin same) error = %v", err)
	}
	if builtinSame.WritePath != builtinPath || builtinSame.RemoveDir != "" {
		t.Fatalf("PlanUpsert(builtin same) = %+v", builtinSame)
	}

	builtinRename, err := PlanUpsert("---\nname: skill-builder\ndescription: Builder\n---\n\n# Builder\n", builtinPath)
	if err != nil {
		t.Fatalf("PlanUpsert(builtin rename) error = %v", err)
	}
	if builtinRename.WritePath != pathJoin(IndexDirPath, "skill-builder", "SKILL.md") {
		t.Fatalf("PlanUpsert(builtin rename).WritePath = %q", builtinRename.WritePath)
	}
	if builtinRename.RemoveDir != pathJoin(IndexDirPath, "skill-creator") {
		t.Fatalf("PlanUpsert(builtin rename).RemoveDir = %q", builtinRename.RemoveDir)
	}

	override, err := PlanUpsert("---\nname: alpha\ndescription: A\n---\n\n# A\n", "/data/.agents/skills/alpha/SKILL.md")
	if err != nil {
		t.Fatalf("PlanUpsert(override) error = %v", err)
	}
	if override.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || override.RemoveDir != "" {
		t.Fatalf("PlanUpsert(override) = %+v", override)
	}
}

func TestPluginPathsForIDRejectEscapingIDs(t *testing.T) {
	for _, id := range []string{".", "..", ".plugin", "alpha..beta", "alpha/beta"} {
		for name, fn := range map[string]func(string) (string, error){
			"PluginDirForID":        PluginDirForID,
			"PluginSkillsDirForID":  PluginSkillsDirForID,
			"PluginHooksPathForID":  PluginHooksPathForID,
			"PluginScriptsDirForID": PluginScriptsDirForID,
		} {
			if _, err := fn(id); !errors.Is(err, bridge.ErrBadRequest) {
				t.Fatalf("%s(%q) err = %v, want ErrBadRequest", name, id, err)
			}
		}
	}

	gotRoot, err := PluginDirForID("github")
	if err != nil {
		t.Fatalf("PluginDirForID(valid) returned error: %v", err)
	}
	if gotRoot != pathJoin(PluginDirPath, "github") {
		t.Fatalf("PluginDirForID(valid) = %q, want %q", gotRoot, pathJoin(PluginDirPath, "github"))
	}
	gotSkills, err := PluginSkillsDirForID("github")
	if err != nil {
		t.Fatalf("PluginSkillsDirForID(valid) returned error: %v", err)
	}
	if gotSkills != pathJoin(PluginDirPath, "github", "skills") {
		t.Fatalf("PluginSkillsDirForID(valid) = %q, want %q", gotSkills, pathJoin(PluginDirPath, "github", "skills"))
	}
	gotHooks, err := PluginHooksPathForID("github")
	if err != nil {
		t.Fatalf("PluginHooksPathForID(valid) returned error: %v", err)
	}
	if gotHooks != pathJoin(PluginDirPath, "github", "hooks.json") {
		t.Fatalf("PluginHooksPathForID(valid) = %q, want %q", gotHooks, pathJoin(PluginDirPath, "github", "hooks.json"))
	}
	gotScripts, err := PluginScriptsDirForID("github")
	if err != nil {
		t.Fatalf("PluginScriptsDirForID(valid) returned error: %v", err)
	}
	if gotScripts != pathJoin(PluginDirPath, "github", "scripts") {
		t.Fatalf("PluginScriptsDirForID(valid) = %q, want %q", gotScripts, pathJoin(PluginDirPath, "github", "scripts"))
	}
}

func TestRegistrySkillPaths(t *testing.T) {
	for _, tc := range []struct {
		registryID, packageID, skillID string
	}{
		{".", "pkg", "skill"},
		{"reg", "..", "skill"},
		{"reg", "pkg", "alpha/beta"},
		{"reg", "pkg", ".hidden"},
	} {
		if _, err := RegistrySkillDirForIDs(tc.registryID, tc.packageID, tc.skillID); !errors.Is(err, bridge.ErrBadRequest) {
			t.Fatalf("RegistrySkillDirForIDs(%q,%q,%q) err = %v, want ErrBadRequest", tc.registryID, tc.packageID, tc.skillID, err)
		}
	}

	dirPath, err := RegistrySkillDirForIDs("openai-api-curated", "docs", "xlsx")
	if err != nil {
		t.Fatalf("RegistrySkillDirForIDs(valid) error = %v", err)
	}
	want := pathJoin(ManagedDirPath, "openai-api-curated", "docs", "xlsx")
	if dirPath != want {
		t.Fatalf("RegistrySkillDirForIDs(valid) = %q, want %q", dirPath, want)
	}

	packagePath, err := RegistryPackageSkillsDirForIDs("openai-api-curated", "docs")
	if err != nil {
		t.Fatalf("RegistryPackageSkillsDirForIDs(valid) error = %v", err)
	}
	wantPackage := pathJoin(ManagedDirPath, "openai-api-curated", "docs")
	if packagePath != wantPackage {
		t.Fatalf("RegistryPackageSkillsDirForIDs(valid) = %q, want %q", packagePath, wantPackage)
	}
}

func TestListDiscoversRegistrySkillsWithoutRenaming(t *testing.T) {
	client := newFakeClient()
	packagePath := pathJoin(ManagedDirPath, "openai-api-curated", "docs")
	skillPath := pathJoin(packagePath, "xlsx")
	client.listings[ManagedDirPath] = []*pb.FileEntry{{Path: "openai-api-curated", IsDir: true}}
	client.listings[pathJoin(ManagedDirPath, "openai-api-curated")] = []*pb.FileEntry{{Path: "docs", IsDir: true}}
	client.listings[packagePath] = []*pb.FileEntry{{Path: "xlsx", IsDir: true}}
	client.files[pathJoin(skillPath, "SKILL.md")] = "---\nname: xlsx\ndescription: Spreadsheet helper\n---\n\n# XLSX\n"

	items, err := ListWithPluginRoots(context.Background(), client, []string{}, nil)
	if err != nil {
		t.Fatalf("ListWithPluginRoots() error = %v", err)
	}
	got, ok := findBySourcePath(items, pathJoin(skillPath, "SKILL.md"))
	if !ok {
		t.Fatalf("registry skill not discovered: %+v", items)
	}
	if got.Name != "xlsx" {
		t.Fatalf("name = %q, want xlsx", got.Name)
	}
	if got.SourceKind != SourceKindRegistry {
		t.Fatalf("source_kind = %q, want %q", got.SourceKind, SourceKindRegistry)
	}
	if !got.Managed {
		t.Fatal("registry skill should be managed")
	}
	if got.State != StateEffective {
		t.Fatalf("state = %q, want %q", got.State, StateEffective)
	}
}

func TestDiscoveryRootsMatchDefaultPolicy(t *testing.T) {
	roots := DiscoveryRoots(nil)
	want := []Root{
		{Path: ManagedDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: IndexDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: LegacyDirPath, Kind: SourceKindLegacy, Managed: false},
		{Path: "/data/.agents/skills", Kind: SourceKindCompat, Managed: false},
		{Path: "/root/.agents/skills", Kind: SourceKindCompat, Managed: false},
	}
	if !slices.Equal(roots, want) {
		t.Fatalf("DiscoveryRoots() = %+v, want %+v", roots, want)
	}
}

func TestDiscoveryRootsUseConfiguredCompatRoots(t *testing.T) {
	roots := DiscoveryRoots([]string{
		" /custom/skills ",
		"/data/skills",
		"/data/.memoh/skills",
		"/custom/skills",
		"relative/skills",
		"/root/.openclaw/skills",
	})
	want := []Root{
		{Path: ManagedDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: IndexDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: LegacyDirPath, Kind: SourceKindLegacy, Managed: false},
		{Path: "/custom/skills", Kind: SourceKindCompat, Managed: false},
		{Path: "/root/.openclaw/skills", Kind: SourceKindCompat, Managed: false},
	}
	if !slices.Equal(roots, want) {
		t.Fatalf("DiscoveryRoots(custom) = %+v, want %+v", roots, want)
	}
}

func TestDiscoveryRootsAllowExplicitEmptyCompatRoots(t *testing.T) {
	roots := DiscoveryRoots([]string{})
	want := []Root{
		{Path: ManagedDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: IndexDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: LegacyDirPath, Kind: SourceKindLegacy, Managed: false},
	}
	if !slices.Equal(roots, want) {
		t.Fatalf("DiscoveryRoots(empty) = %+v, want %+v", roots, want)
	}
}

func TestDiscoveryRootsIncludePluginRootsAsServerManagedSource(t *testing.T) {
	roots := DiscoveryRootsWithPluginRoots([]string{"/custom/skills"}, []string{
		" /data/.memoh/plugins/github/skills ",
		"/data/.memoh/plugins/github/skills",
		"/data/.memoh/plugins/bad",
		"relative/plugin/skills",
	})
	want := []Root{
		{Path: ManagedDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: IndexDirPath, Kind: SourceKindManaged, Managed: true},
		{Path: LegacyDirPath, Kind: SourceKindLegacy, Managed: false},
		{Path: "/data/.memoh/plugins/github/skills", Kind: SourceKindPlugin, Managed: false},
		{Path: "/custom/skills", Kind: SourceKindCompat, Managed: false},
	}
	if !slices.Equal(roots, want) {
		t.Fatalf("DiscoveryRootsWithPluginRoots() = %+v, want %+v", roots, want)
	}
}

func TestListScansConfiguredDiscoveryRootsInOrder(t *testing.T) {
	client := newFakeClient()
	rawCompatRoots := []string(nil)
	for _, root := range DiscoveryRoots(rawCompatRoots) {
		client.listings[root.Path] = nil
	}
	client.listings[ManagedDirPath] = []*pb.FileEntry{{Path: "alpha", IsDir: true}}
	client.files[pathJoin(ManagedDirPath, "alpha", "SKILL.md")] = "---\nname: alpha\ndescription: Alpha\n---\n\n# Alpha"

	items, err := List(context.Background(), client, rawCompatRoots)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].SourceRoot != ManagedDirPath {
		t.Fatalf("List() items = %+v, want managed alpha only", items)
	}

	// discoverRegistryPackageRoots lists ManagedDirPath first, then each discovery root is scanned.
	wantCalls := []string{ManagedDirPath}
	for _, root := range DiscoveryRoots(rawCompatRoots) {
		wantCalls = append(wantCalls, root.Path)
	}
	if !slices.Equal(client.listCalls, wantCalls) {
		t.Fatalf("ListDirAll calls = %+v, want %+v", client.listCalls, wantCalls)
	}
}

func TestContainerEnvUsesDataHomeAndXDGDirs(t *testing.T) {
	env := ContainerEnv(nil)
	for _, want := range []string{
		"HOME=/data",
		"XDG_CONFIG_HOME=/data/.config",
		"XDG_DATA_HOME=/data/.local/share",
		"XDG_CACHE_HOME=/data/.cache",
		"MEMOH_SKILL_DISCOVERY_ROOTS=/data/.agents/skills:/root/.agents/skills",
	} {
		if !slices.Contains(env, want) {
			t.Fatalf("env %+v does not contain %q", env, want)
		}
	}
}

func TestContainerEnvUsesConfiguredSkillDiscoveryRoots(t *testing.T) {
	env := ContainerEnv([]string{"/custom/skills", "/root/.openclaw/skills"})
	want := SkillDiscoveryRootsEnvVar + "=/custom/skills:/root/.openclaw/skills"
	if !slices.Contains(env, want) {
		t.Fatalf("env %+v does not contain %q", env, want)
	}
}

type fakeClient struct {
	listings  map[string][]*pb.FileEntry
	files     map[string]string
	listCalls []string
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		listings: make(map[string][]*pb.FileEntry),
		files:    make(map[string]string),
	}
}

func (f *fakeClient) ListDirAll(_ context.Context, p string, _ bool) ([]*pb.FileEntry, error) {
	f.listCalls = append(f.listCalls, p)
	items, ok := f.listings[p]
	if !ok {
		return nil, io.EOF
	}
	return items, nil
}

func (f *fakeClient) ReadRaw(_ context.Context, p string) (io.ReadCloser, error) {
	content, ok := f.files[p]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (f *fakeClient) WriteRaw(_ context.Context, p string, r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.files[p] = string(data)
	return int64(len(data)), nil
}

func (*fakeClient) Mkdir(_ context.Context, _ string) error {
	return nil
}

func pathJoin(parts ...string) string {
	return strings.Join(parts, "/")
}
