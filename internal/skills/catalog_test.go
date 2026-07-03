package skills

import (
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/slash"
)

func TestBuildSafeCatalogFiltersUnsafeEntries(t *testing.T) {
	codec := testRefCodec(t)
	entries := []Entry{
		testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content"),
		testEntry("alpha", StateShadowed, SourceKindCompat, "/compat/alpha", "other"),
		testEntry("beta", StateEffective, SourceKindManaged, "/skills/beta", "beta content"),
		testEntry("disabled", StateDisabled, SourceKindManaged, "/skills/disabled", "disabled content"),
		testEntryWithMetadata("hidden", StateEffective, SourceKindManaged, "/skills/hidden", "hidden content", map[string]any{"hidden": true}),
		testEntryWithMetadata("malformed", StateEffective, SourceKindManaged, "/skills/malformed", "malformed content", map[string]any{"internal": "maybe"}),
	}
	catalog, err := BuildSafeCatalog("bot-1", entries, codec, "effective")
	if err != nil {
		t.Fatalf("BuildSafeCatalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("catalog len = %d, want 1: %#v", len(catalog), catalog)
	}
	if catalog[0].Name != "beta" {
		t.Fatalf("catalog[0].Name = %q, want beta", catalog[0].Name)
	}
	if catalog[0].SkillRef == "" {
		t.Fatal("SkillRef is empty")
	}
}

func TestBuildSafeCatalogFiltersMalformedParsedMetadata(t *testing.T) {
	codec := testRefCodec(t)
	raw := "---\nname: malformed\ndescription: Malformed\nmetadata: runtime-visible\n---\n\n# Malformed"
	malformed := entryFromParsed(ParseFile(raw, "malformed"), raw, Root{
		Path:    ManagedDirPath,
		Kind:    SourceKindManaged,
		Managed: true,
	}, pathJoin(ManagedDirPath, "malformed", "SKILL.md"))
	malformed.State = StateEffective
	valid := testEntry("valid", StateEffective, SourceKindManaged, pathJoin(ManagedDirPath, "valid", "SKILL.md"), "valid content")

	catalog, err := BuildSafeCatalog("bot-1", []Entry{malformed, valid}, codec, "effective")
	if err != nil {
		t.Fatalf("BuildSafeCatalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("catalog len = %d, want 1: %#v", len(catalog), catalog)
	}
	if catalog[0].Name != "valid" {
		t.Fatalf("catalog[0].Name = %q, want valid", catalog[0].Name)
	}
}

func TestResolveRefErrorMapping(t *testing.T) {
	codec := testRefCodec(t)
	entry := testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content")
	catalog, err := BuildSafeCatalog("bot-1", []Entry{entry}, codec, "effective")
	if err != nil {
		t.Fatalf("BuildSafeCatalog: %v", err)
	}
	ref := catalog[0].SkillRef

	_, err = ResolveRef("wrong-bot", []Entry{entry}, codec, "effective", "alpha", ref)
	assertSlashCode(t, err, slash.CodeInvalidSkillRef)

	stale := entry
	stale.Raw = "changed"
	stale.Content = "changed"
	_, err = ResolveRef("bot-1", []Entry{stale}, codec, "effective", "alpha", ref)
	assertSlashCode(t, err, slash.CodeStaleSkillRef)

	disabled := entry
	disabled.State = StateDisabled
	_, err = ResolveRef("bot-1", []Entry{disabled}, codec, "effective", "alpha", ref)
	assertSlashCode(t, err, slash.CodeRequestedSkillDisabled)

	hidden := entry
	hidden.Metadata = map[string]any{"internal_only": true}
	_, err = ResolveRef("bot-1", []Entry{hidden}, codec, "effective", "alpha", ref)
	assertSlashCode(t, err, slash.CodeRequestedSkillNotRuntimeUsable)
}

func TestResolveRefUsesTokenKidForOpaqueSourceID(t *testing.T) {
	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("abcdef0123456789abcdef0123456789")
	oldCodec, err := newRefCodec("kid1", map[string][]byte{
		"kid1": oldKey,
	}, &deterministicReader{})
	if err != nil {
		t.Fatalf("old newRefCodec: %v", err)
	}
	currentCodec, err := newRefCodec("kid2", map[string][]byte{
		"kid1": oldKey,
		"kid2": newKey,
	}, &deterministicReader{})
	if err != nil {
		t.Fatalf("current newRefCodec: %v", err)
	}

	entry := testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content")
	catalog, err := BuildSafeCatalog("bot-1", []Entry{entry}, oldCodec, "effective")
	if err != nil {
		t.Fatalf("BuildSafeCatalog: %v", err)
	}

	resolved, err := ResolveRef("bot-1", []Entry{entry}, currentCodec, "effective", "alpha", catalog[0].SkillRef)
	if err != nil {
		t.Fatalf("ResolveRef with old token kid: %v", err)
	}
	if resolved.Name != "alpha" {
		t.Fatalf("resolved.Name = %q, want alpha", resolved.Name)
	}
}

func TestResolveTextRequestedSkills(t *testing.T) {
	codec := testRefCodec(t)
	alpha := testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content")
	beta := testEntry("beta", StateEffective, SourceKindManaged, "/skills/beta", "beta content")
	resolved, err := ResolveTextRequestedSkills("bot-1", []Entry{alpha, beta}, []string{"alpha", "alpha", "beta"}, codec, "effective", ResolveLimits{MaxCount: 5, MaxSingleBytes: 1024, MaxTotalBytes: 2048})
	if err != nil {
		t.Fatalf("ResolveTextRequestedSkills: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved len = %d, want 2", len(resolved))
	}
	if resolved[0].Name != "alpha" || resolved[1].Name != "beta" {
		t.Fatalf("resolved order = %#v", resolved)
	}
	if resolved[0].Identity == "" || resolved[0].Identity == resolved[1].Identity {
		t.Fatalf("bad identities: %#v", resolved)
	}
}

func TestResolveTextRequestedSkillsRejectsAmbiguous(t *testing.T) {
	codec := testRefCodec(t)
	entries := []Entry{
		testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content"),
		testEntry("alpha", StateShadowed, SourceKindCompat, "/compat/alpha", "other"),
	}
	_, err := ResolveTextRequestedSkills("bot-1", entries, []string{"alpha"}, codec, "effective", ResolveLimits{})
	assertSlashCode(t, err, slash.CodeRequestedSkillAmbiguous)
}

func TestResolveTextRequestedSkillsLimits(t *testing.T) {
	codec := testRefCodec(t)
	entry := testEntry("alpha", StateEffective, SourceKindManaged, "/skills/alpha", "alpha content")
	_, err := ResolveTextRequestedSkills("bot-1", []Entry{entry}, []string{"alpha"}, codec, "effective", ResolveLimits{MaxSingleBytes: 1})
	assertSlashCode(t, err, slash.CodeRequestedSkillContextTooLarge)
}

func testEntry(name, state, sourceKind, sourcePath, content string) Entry {
	return testEntryWithMetadata(name, state, sourceKind, sourcePath, content, nil)
}

func testEntryWithMetadata(name, state, sourceKind, sourcePath, content string, metadata map[string]any) Entry {
	return Entry{
		Name:        name,
		Description: name + " description",
		Content:     content,
		Raw:         content,
		Metadata:    metadata,
		SourcePath:  sourcePath,
		SourceRoot:  "/root",
		SourceKind:  sourceKind,
		State:       state,
	}
}

func assertSlashCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %s", code)
	}
	var slashErr slash.Error
	if !errors.As(err, &slashErr) {
		t.Fatalf("err = %T %[1]v, want slash.Error %s", err, code)
	}
	if slashErr.Code != code {
		t.Fatalf("code = %s, want %s", slashErr.Code, code)
	}
}
