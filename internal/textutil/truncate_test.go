package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	text := "你好世界"
	got := TruncateRunes(text, 3)
	if got != "你好世" {
		t.Fatalf("TruncateRunes() = %q, want %q", got, "你好世")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("TruncateRunes() returned invalid UTF-8: %q", got)
	}
}

func TestTruncateRunesWithSuffix(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("你", 10) + "abc"
	got := TruncateRunesWithSuffix(text, 8, "...")
	if utf8.RuneCountInString(got) != 8 {
		t.Fatalf("TruncateRunesWithSuffix() rune count = %d, want 8", utf8.RuneCountInString(got))
	}
	if got != strings.Repeat("你", 5)+"..." {
		t.Fatalf("TruncateRunesWithSuffix() = %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("TruncateRunesWithSuffix() returned invalid UTF-8: %q", got)
	}
}

func TestTruncateRunesWithSuffixNoTruncation(t *testing.T) {
	t.Parallel()

	text := "你好世界"
	if got := TruncateRunesWithSuffix(text, 8, "..."); got != text {
		t.Fatalf("TruncateRunesWithSuffix() = %q, want %q", got, text)
	}
}

func TestTruncateRunesWithSuffixKeepsInvalidUTF8Bytes(t *testing.T) {
	t.Parallel()

	text := "ab\xffcd"
	got := TruncateRunesWithSuffix(text, 4, "...")
	if got != "a..." {
		t.Fatalf("TruncateRunesWithSuffix() = %q, want %q", got, "a...")
	}
}
