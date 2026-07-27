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

func TestApplyActionAdoptRejectsRegistryLayoutConflict(t *testing.T) {
	client := newFakeClient()
	registryID := "openai-api-curated"
	externalPath := pathJoin("/data/.agents/skills", registryID, "SKILL.md")
	client.listings["/data/.agents/skills"] = []*pb.FileEntry{{Path: registryID, IsDir: true}}
	client.files[externalPath] = "---\nname: " + registryID + "\ndescription: Compat\n---\n\n# Compat"

	registryRoot := pathJoin(ManagedDirPath, registryID)
	packageRoot := pathJoin(registryRoot, "docs")
	client.listings[ManagedDirPath] = []*pb.FileEntry{{Path: registryID, IsDir: true}}
	client.listings[registryRoot] = []*pb.FileEntry{{Path: "docs", IsDir: true}}
	client.listings[packageRoot] = []*pb.FileEntry{{Path: "xlsx", IsDir: true}}
	client.files[pathJoin(packageRoot, "xlsx", "SKILL.md")] = "---\nname: xlsx\ndescription: Spreadsheet\n---\n\n# Spreadsheet"

	err := ApplyAction(context.Background(), client, nil, ActionRequest{
		Action:     ActionAdopt,
		TargetPath: externalPath,
	})
	if !errors.Is(err, ErrRegistryLayoutConflict) {
		t.Fatalf("adopt err = %v, want ErrRegistryLayoutConflict", err)
	}
	if _, ok := client.files[pathJoin(registryRoot, "SKILL.md")]; ok {
		t.Fatal("adopt wrote a flat SKILL.md into a registry root")
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

	if _, err := DeletableSkillDirForSourcePath(pathJoin(IndexDirPath, "skill-creator", "SKILL.md")); !errors.Is(err, ErrBuiltinSkillReadOnly) {
		t.Fatalf("DeletableSkillDirForSourcePath(builtin) err = %v, want ErrBuiltinSkillReadOnly", err)
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
	if create.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || create.RenameFromDir != "" {
		t.Fatalf("PlanUpsert(create) = %+v", create)
	}

	same, err := PlanUpsert("---\nname: alpha\ndescription: A\n---\n\n# A\n", pathJoin(ManagedDirPath, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("PlanUpsert(same) error = %v", err)
	}
	if same.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || same.RenameFromDir != "" {
		t.Fatalf("PlanUpsert(same) = %+v", same)
	}

	rename, err := PlanUpsert("---\nname: beta\ndescription: B\n---\n\n# B\n", pathJoin(ManagedDirPath, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("PlanUpsert(rename) error = %v", err)
	}
	if rename.WritePath != pathJoin(ManagedDirPath, "beta", "SKILL.md") {
		t.Fatalf("PlanUpsert(rename).WritePath = %q", rename.WritePath)
	}
	if rename.RenameFromDir != pathJoin(ManagedDirPath, "alpha") {
		t.Fatalf("PlanUpsert(rename).RenameFromDir = %q", rename.RenameFromDir)
	}

	registryPath := pathJoin(ManagedDirPath, "openai-api-curated", "docs", "xlsx", "SKILL.md")
	registry, err := PlanUpsert("---\nname: xlsx\ndescription: Sheet\n---\n\n# Sheet\n", registryPath)
	if err != nil {
		t.Fatalf("PlanUpsert(registry) error = %v", err)
	}
	if registry.WritePath != registryPath || registry.RenameFromDir != "" {
		t.Fatalf("PlanUpsert(registry) = %+v", registry)
	}

	builtinPath := pathJoin(IndexDirPath, "skill-creator", "SKILL.md")
	if _, err := PlanUpsert("---\nname: skill-creator\ndescription: Creator\n---\n\n# Creator\n", builtinPath); !errors.Is(err, ErrBuiltinSkillReadOnly) {
		t.Fatalf("PlanUpsert(builtin) error = %v, want ErrBuiltinSkillReadOnly", err)
	}

	override, err := PlanUpsert("---\nname: alpha\ndescription: A\n---\n\n# A\n", "/data/.agents/skills/alpha/SKILL.md")
	if err != nil {
		t.Fatalf("PlanUpsert(override) error = %v", err)
	}
	if override.WritePath != pathJoin(ManagedDirPath, "alpha", "SKILL.md") || override.RenameFromDir != "" {
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

func TestListDiscoversRegistryPackagesEvenWhenFlatSkillSharesRegistryDir(t *testing.T) {
	client := newFakeClient()
	registryPath := pathJoin(ManagedDirPath, "openai-api-curated")
	packagePath := pathJoin(registryPath, "docs")
	skillPath := pathJoin(packagePath, "xlsx")
	client.listings[ManagedDirPath] = []*pb.FileEntry{{Path: "openai-api-curated", IsDir: true}}
	client.listings[registryPath] = []*pb.FileEntry{{Path: "docs", IsDir: true}}
	client.listings[packagePath] = []*pb.FileEntry{{Path: "xlsx", IsDir: true}}
	// Flat managed marker at the registry root must not hide nested packages.
	client.files[pathJoin(registryPath, "SKILL.md")] = "---\nname: openai-api-curated\ndescription: Flat\n---\n\n# Flat\n"
	client.files[pathJoin(skillPath, "SKILL.md")] = "---\nname: xlsx\ndescription: Spreadsheet\n---\n\n# XLSX\n"

	items, err := ListWithPluginRoots(context.Background(), client, []string{}, nil)
	if err != nil {
		t.Fatalf("ListWithPluginRoots() error = %v", err)
	}
	if _, ok := findBySourcePath(items, pathJoin(registryPath, "SKILL.md")); !ok {
		t.Fatalf("flat skill at registry root not discovered: %+v", items)
	}
	registry, ok := findBySourcePath(items, pathJoin(skillPath, "SKILL.md"))
	if !ok {
		t.Fatalf("registry skill was hidden by flat marker: %+v", items)
	}
	if registry.SourceKind != SourceKindRegistry {
		t.Fatalf("source_kind = %q, want %q", registry.SourceKind, SourceKindRegistry)
	}
}

func TestGuardFlatManagedWriteAndRegistryInstall(t *testing.T) {
	client := newFakeClient()
	registryPath := pathJoin(ManagedDirPath, "openai-api-curated")
	packagePath := pathJoin(registryPath, "docs")
	skillPath := pathJoin(packagePath, "xlsx")
	client.listings[registryPath] = []*pb.FileEntry{{Path: "docs", IsDir: true}}
	client.listings[packagePath] = []*pb.FileEntry{{Path: "xlsx", IsDir: true}}
	client.files[pathJoin(skillPath, "SKILL.md")] = "---\nname: xlsx\ndescription: Sheet\n---\n\n# Sheet\n"

	hasRegistryPackages, err := hasRegistryPackageLayout(context.Background(), client, registryPath)
	if err != nil || !hasRegistryPackages {
		t.Fatalf("hasRegistryPackageLayout() = %v, %v; want true, nil", hasRegistryPackages, err)
	}
	if err := GuardFlatManagedWrite(context.Background(), client, pathJoin(registryPath, "SKILL.md")); !errors.Is(err, ErrRegistryLayoutConflict) {
		t.Fatalf("GuardFlatManagedWrite() = %v, want ErrRegistryLayoutConflict", err)
	}
	if err := GuardFlatManagedWrite(context.Background(), client, pathJoin(ManagedDirPath, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("GuardFlatManagedWrite(empty) = %v", err)
	}

	// Resource folders under a flat skill (scripts/, references/) are not registry packages.
	resourceRoot := pathJoin(ManagedDirPath, "my-skill")
	client.listings[resourceRoot] = []*pb.FileEntry{{Path: "scripts", IsDir: true}}
	client.listings[pathJoin(resourceRoot, "scripts")] = []*pb.FileEntry{{Path: "run.sh", IsDir: false}}
	hasRegistryPackages, err = hasRegistryPackageLayout(context.Background(), client, resourceRoot)
	if err != nil || hasRegistryPackages {
		t.Fatalf("resource folders layout = %v, %v; want false, nil", hasRegistryPackages, err)
	}
	if err := GuardFlatManagedWrite(context.Background(), client, pathJoin(resourceRoot, "SKILL.md")); err != nil {
		t.Fatalf("GuardFlatManagedWrite(resource folders) = %v", err)
	}

	client.files[pathJoin(registryPath, "SKILL.md")] = "---\nname: openai-api-curated\ndescription: Flat\n---\n\n# Flat\n"
	if err := GuardRegistryInstall(context.Background(), client, "openai-api-curated"); !errors.Is(err, ErrFlatSkillOccupiesRegistry) {
		t.Fatalf("GuardRegistryInstall() = %v, want ErrFlatSkillOccupiesRegistry", err)
	}
	if err := GuardRegistryInstall(context.Background(), client, "memoh"); err != nil {
		t.Fatalf("GuardRegistryInstall(empty) = %v", err)
	}

	boom := errors.New("storage unavailable")
	brokenRoot := pathJoin(ManagedDirPath, "broken")
	client.listErrors[brokenRoot] = boom
	if err := GuardFlatManagedWrite(context.Background(), client, pathJoin(brokenRoot, "SKILL.md")); !errors.Is(err, boom) {
		t.Fatalf("GuardFlatManagedWrite(storage error) = %v, want storage error", err)
	}
	client.readErrors[pathJoin(ManagedDirPath, "broken-registry", "SKILL.md")] = boom
	if err := GuardRegistryInstall(context.Background(), client, "broken-registry"); !errors.Is(err, boom) {
		t.Fatalf("GuardRegistryInstall(storage error) = %v, want storage error", err)
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

	// discoverRegistryPackageRoots lists ManagedDirPath, then each candidate
	// registry directory (including flat skill dirs) to look for packages.
	wantCalls := []string{ManagedDirPath, pathJoin(ManagedDirPath, "alpha")}
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
	listings   map[string][]*pb.FileEntry
	files      map[string]string
	listErrors map[string]error
	readErrors map[string]error
	listCalls  []string
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		listings:   make(map[string][]*pb.FileEntry),
		files:      make(map[string]string),
		listErrors: make(map[string]error),
		readErrors: make(map[string]error),
	}
}

func (f *fakeClient) ListDirAll(_ context.Context, p string, _ bool) ([]*pb.FileEntry, error) {
	f.listCalls = append(f.listCalls, p)
	if err := f.listErrors[p]; err != nil {
		return nil, err
	}
	items, ok := f.listings[p]
	if !ok {
		return nil, io.EOF
	}
	return items, nil
}

func (f *fakeClient) ReadRaw(_ context.Context, p string) (io.ReadCloser, error) {
	if err := f.readErrors[p]; err != nil {
		return nil, err
	}
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
