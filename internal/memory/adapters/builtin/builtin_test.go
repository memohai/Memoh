package builtin

import (
	"testing"
	"unicode/utf8"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
)

func TestTruncateSnippet_ASCII(t *testing.T) {
	t.Parallel()
	got := adapters.TruncateSnippet("hello world", 5)
	if got != "hello..." {
		t.Fatalf("expected %q, got %q", "hello...", got)
	}
}

func TestTruncateSnippet_NoTruncation(t *testing.T) {
	t.Parallel()
	got := adapters.TruncateSnippet("short", 100)
	if got != "short" {
		t.Fatalf("expected %q, got %q", "short", got)
	}
}

func TestTruncateSnippet_CJK(t *testing.T) {
	t.Parallel()
	// 5 CJK characters (15 bytes in UTF-8), truncate to 3 runes.
	got := adapters.TruncateSnippet("你好世界啊", 3)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	if got != "你好世..." {
		t.Fatalf("expected %q, got %q", "你好世...", got)
	}
}

func TestTruncateSnippet_Emoji(t *testing.T) {
	t.Parallel()
	// Emoji are 4 bytes each in UTF-8.
	got := adapters.TruncateSnippet("😀😁😂🤣😃", 2)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	if got != "😀😁..." {
		t.Fatalf("expected %q, got %q", "😀😁...", got)
	}
}

func TestTruncateSnippet_TrimWhitespace(t *testing.T) {
	t.Parallel()
	got := adapters.TruncateSnippet("  hello  ", 100)
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}
