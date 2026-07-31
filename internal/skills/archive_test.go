package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"
)

const validArchiveManifest = "---\nname: skill\ndescription: Demo\n---\n\n# Demo\n"

func TestReadArchiveReadsContentRoot(t *testing.T) {
	archive, err := ReadArchive(testArchive(t, []archiveTestEntry{
		{name: "SKILL.md", content: validArchiveManifest},
		{name: "scripts/run.sh", content: "#!/bin/sh\n", mode: 0o755},
	}))
	if err != nil {
		t.Fatalf("ReadArchive() error = %v", err)
	}
	if archive.FileCount() != 2 {
		t.Fatalf("files = %d, want 2", archive.FileCount())
	}
	if !strings.Contains(string(archiveFileByPath(t, archive, "SKILL.md").content), "# Demo") {
		t.Fatal("manifest body was not preserved")
	}
	if !archiveFileByPath(t, archive, "scripts/run.sh").executable {
		t.Fatal("executable mode was not retained")
	}
}

func TestReadArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := map[string][]archiveTestEntry{
		"path traversal":  {{name: "SKILL.md", content: validArchiveManifest}, {name: "../escape", content: "bad"}},
		"backslash":       {{name: "SKILL.md", content: validArchiveManifest}, {name: `scripts\escape`, content: "bad"}},
		"symlink":         {{name: "SKILL.md", content: validArchiveManifest}, {name: "link", entryType: tar.TypeSymlink, linkName: "../../escape"}},
		"duplicate":       {{name: "SKILL.md", content: validArchiveManifest}, {name: "SKILL.md", content: validArchiveManifest}},
		"file conflict":   {{name: "SKILL.md", content: validArchiveManifest}, {name: "scripts", content: "file"}, {name: "scripts/run.sh", content: "bad"}},
		"nested manifest": {{name: "other/SKILL.md", content: validArchiveManifest}},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadArchive(testArchive(t, entries)); err == nil {
				t.Fatal("ReadArchive() error = nil, want rejection")
			}
		})
	}
}

func TestReadArchiveRejectsEntryFlood(t *testing.T) {
	entries := []archiveTestEntry{{name: "SKILL.md", content: validArchiveManifest}}
	for i := 0; i <= maxArchiveEntries; i++ {
		entries = append(entries, archiveTestEntry{name: fmt.Sprintf("dir%d/", i), entryType: tar.TypeDir})
	}
	if _, err := ReadArchive(testArchive(t, entries)); err == nil || !strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("want too-many-entries rejection, got %v", err)
	}
}

func TestReadArchiveRejectsMalformedFrontmatter(t *testing.T) {
	for name, manifest := range map[string]string{
		"missing closing fence": "---\nname: skill\n",
		"not a mapping":         "---\n- a\n- b\n---\n# Body\n",
		"missing frontmatter":   "# Body only\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadArchive(testArchive(t, []archiveTestEntry{{name: "SKILL.md", content: manifest}})); err == nil {
				t.Fatal("ReadArchive() accepted malformed SKILL.md frontmatter")
			}
		})
	}
}

type archiveTestEntry struct {
	name      string
	content   string
	mode      int64
	entryType byte
	linkName  string
}

func testArchive(t *testing.T, entries []archiveTestEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		kind := entry.entryType
		if kind == 0 {
			kind = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{Name: entry.name, Mode: mode, Typeflag: kind, Linkname: entry.linkName}
		if kind == tar.TypeReg {
			header.Size = int64(len(entry.content))
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q): %v", entry.name, err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.content)); err != nil {
				t.Fatalf("Write(%q): %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return output.Bytes()
}

func archiveFileByPath(t *testing.T, archive Archive, name string) archiveFile {
	t.Helper()
	for _, file := range archive.files {
		if file.path == name {
			return file
		}
	}
	t.Fatalf("archive does not contain %q", name)
	return archiveFile{}
}
