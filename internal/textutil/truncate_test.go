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

	tests := []struct {
		name   string
		text   string
		max    int
		suffix string
		want   string
	}{
		{
			name:   "truncates on rune boundary and reserves suffix budget",
			text:   strings.Repeat("你", 10) + "abc",
			max:    8,
			suffix: "...",
			want:   strings.Repeat("你", 5) + "...",
		},
		{
			name:   "returns original text when already within budget",
			text:   "你好世界",
			max:    8,
			suffix: "...",
			want:   "你好世界",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := TruncateRunesWithSuffix(tt.text, tt.max, tt.suffix)
			if got != tt.want {
				t.Fatalf("TruncateRunesWithSuffix() = %q, want %q", got, tt.want)
			}
			if utf8.RuneCountInString(got) > tt.max {
				t.Fatalf("TruncateRunesWithSuffix() rune count = %d, want <= %d", utf8.RuneCountInString(got), tt.max)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateRunesWithSuffix() returned invalid UTF-8: %q", got)
			}
		})
	}
}
