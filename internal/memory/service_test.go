package memory

import (
	"regexp"
	"strings"
	"testing"
)

func TestNormalizeMemoryDayContent_StructuredJSON(t *testing.T) {
	path := "/data/memory/2026-03-01.md"
	input := `[
  {
    "topic": "Decision",
    "memory": "Choose provider architecture."
  }
]`

	out := NormalizeMemoryDayContent(path, input)
	if !strings.Contains(out, "# Memory 2026-03-01") {
		t.Fatalf("expected day header, got: %s", out)
	}
	if !strings.Contains(out, `<!-- MEMOH:ENTRY `) {
		t.Fatalf("expected entry marker, got: %s", out)
	}
	if !strings.Contains(out, `"topic":"Decision"`) {
		t.Fatalf("expected topic metadata, got: %s", out)
	}
	if !strings.Contains(out, "Choose provider architecture.") {
		t.Fatalf("expected memory body, got: %s", out)
	}
	if !strings.Contains(out, `"hash":"`) {
		t.Fatalf("expected generated hash metadata, got: %s", out)
	}
}

func TestNormalizeMemoryDayContent_FallbackPlainText(t *testing.T) {
	path := "/data/memory/2026-03-01.md"
	input := "Unstructured note from model output."
	out := NormalizeMemoryDayContent(path, input)

	if !strings.Contains(out, "# Memory 2026-03-01") {
		t.Fatalf("expected day header, got: %s", out)
	}
	if !strings.Contains(out, "Unstructured note from model output.") {
		t.Fatalf("expected original text preserved, got: %s", out)
	}
	if !strings.Contains(out, `"created_at":"`) || !strings.Contains(out, `"updated_at":"`) {
		t.Fatalf("expected timestamps, got: %s", out)
	}
	if !regexp.MustCompile(`"id":"mem_\d+"`).MatchString(out) {
		t.Fatalf("expected generated id, got: %s", out)
	}
}

func TestNormalizeMemoryDayContent_LegacyFrontmatter(t *testing.T) {
	path := "/data/memory/2026-03-01.md"
	input := `---
id: mem_legacy_1
hash: legacyhash
created_at: 2026-03-01T09:00:00Z
updated_at: 2026-03-01T10:00:00Z
---
Legacy body text.`

	out := NormalizeMemoryDayContent(path, input)
	if !strings.Contains(out, `"id":"mem_legacy_1"`) {
		t.Fatalf("expected legacy id reused, got: %s", out)
	}
	if !strings.Contains(out, `"hash":"legacyhash"`) {
		t.Fatalf("expected legacy hash reused, got: %s", out)
	}
	if !strings.Contains(out, "Legacy body text.") {
		t.Fatalf("expected legacy body reused, got: %s", out)
	}
}

func TestRenderMemoryDayForDisplay(t *testing.T) {
	path := "/data/memory/2026-03-01.md"
	raw := `# Memory 2026-03-01

<!-- MEMOH:ENTRY {"id":"mem_1","topic":"Decision","created_at":"2026-03-01T09:40:00Z"} -->
结论：采用 provider 架构
<!-- /MEMOH:ENTRY -->

<!-- MEMOH:ENTRY {"id":"mem_2","topic":"Notes","created_at":"2026-03-01T11:15:00Z"} -->
用户偏好：简短回复
<!-- /MEMOH:ENTRY -->
`

	out := RenderMemoryDayForDisplay(path, raw)
	if strings.Contains(out, "MEMOH:ENTRY") {
		t.Fatalf("display output should hide raw markers: %s", out)
	}
	if !strings.Contains(out, "# 2026-03-01") {
		t.Fatalf("expected display day header, got: %s", out)
	}
	if !strings.Contains(out, "## 09:40 AM - Decision") {
		t.Fatalf("expected timeline section, got: %s", out)
	}
	if !strings.Contains(out, "- 结论：采用 provider 架构") {
		t.Fatalf("expected bulletized body, got: %s", out)
	}
	if !strings.Contains(out, "## 11:15 AM - Notes") {
		t.Fatalf("expected second timeline section, got: %s", out)
	}
}

func TestRenderMemoryDayForDisplay_NonMemoryPathUnchanged(t *testing.T) {
	raw := "plain content"
	out := RenderMemoryDayForDisplay("/data/notes.md", raw)
	if out != raw {
		t.Fatalf("non-memory path should be unchanged, got: %s", out)
	}
}

