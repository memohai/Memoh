package handlers

import (
	"archive/tar"
	"bytes"
	"context"
	"strings"
	"testing"

	skillset "github.com/memohai/memoh/internal/skills"
)

func TestPluginSkillArchiveRelativePathRejectsUnsafeNames(t *testing.T) {
	valid, ok := pluginSkillArchiveRelativePath("github", "github/skills/review/SKILL.md")
	if !ok || valid != "review/SKILL.md" {
		t.Fatalf("valid archive path = %q, %v; want review/SKILL.md, true", valid, ok)
	}

	for _, name := range []string{
		"",
		"github/plugin.yaml",
		"github/skills",
		"github/skills/",
		"github/../escape",
		"github/skills/../escape",
		"../escape",
		"/data/escape",
		"github/skills\\escape",
	} {
		if got, ok := pluginSkillArchiveRelativePath("github", name); ok {
			t.Fatalf("pluginSkillArchiveRelativePath(%q) = %q, true; want rejected", name, got)
		}
	}
}

func TestExtractPluginSkillsArchiveWritesOnlySafeSkillFiles(t *testing.T) {
	archive := tarArchive(t, map[string]string{
		"github/plugin.yaml":                  "id: github",
		"github/skills/review/SKILL.md":       "# Review",
		"github/skills/review/assets/info.md": "asset",
		"github/skills/../escape":             "escape",
		"github/../outside":                   "outside",
		"/data/outside":                       "absolute",
	})
	writer := &pluginSkillsTestWriter{files: map[string]string{}}

	filesWritten, err := extractPluginSkillsArchive(context.Background(), writer, "github", "github", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("extractPluginSkillsArchive returned error: %v", err)
	}
	if filesWritten != 2 {
		t.Fatalf("filesWritten = %d, want 2", filesWritten)
	}

	root, err := skillset.PluginSkillsDirForID("github")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	wantFiles := map[string]string{
		root + "/review/SKILL.md":       "# Review",
		root + "/review/assets/info.md": "asset",
	}
	for path, want := range wantFiles {
		if got := writer.files[path]; got != want {
			t.Fatalf("file %s = %q, want %q", path, got, want)
		}
	}
	for path := range writer.files {
		if strings.Contains(path, "plugin.yaml") || strings.Contains(path, "outside") || strings.Contains(path, "escape") {
			t.Fatalf("unsafe file was written: %s", path)
		}
	}
}

func TestExtractPluginSkillsArchiveSeparatesArchiveAndTargetPluginIDs(t *testing.T) {
	archive := tarArchive(t, map[string]string{
		"GitHub.Plugin/skills/review/SKILL.md": "# Review",
	})
	writer := &pluginSkillsTestWriter{files: map[string]string{}}

	filesWritten, err := extractPluginSkillsArchive(context.Background(), writer, "GitHub.Plugin", "github_plugin", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("extractPluginSkillsArchive returned error: %v", err)
	}
	if filesWritten != 1 {
		t.Fatalf("filesWritten = %d, want 1", filesWritten)
	}

	root, err := skillset.PluginSkillsDirForID("github_plugin")
	if err != nil {
		t.Fatalf("plugin root: %v", err)
	}
	if got := writer.files[root+"/review/SKILL.md"]; got != "# Review" {
		t.Fatalf("target plugin file = %q, want # Review", got)
	}
}

func tarArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

type pluginSkillsTestWriter struct {
	dirs  []string
	files map[string]string
}

func (w *pluginSkillsTestWriter) Mkdir(_ context.Context, path string) error {
	w.dirs = append(w.dirs, path)
	return nil
}

func (w *pluginSkillsTestWriter) WriteFile(_ context.Context, path string, content []byte) error {
	w.files[path] = string(content)
	return nil
}
