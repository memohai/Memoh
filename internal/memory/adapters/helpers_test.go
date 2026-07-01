package adapters

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateSnippet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		input    string
		limit    int
		want     string
		utf8Safe bool
	}{
		{name: "ascii", input: "hello world", limit: 5, want: "hello..."},
		{name: "no_truncation", input: "short", limit: 100, want: "short"},
		{name: "cjk", input: "你好世界啊", limit: 3, want: "你好世...", utf8Safe: true},
		{name: "trim_whitespace", input: "  hello  ", limit: 100, want: "hello"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := TruncateSnippet(tc.input, tc.limit)
			if tc.utf8Safe && !utf8.ValidString(got) {
				t.Fatalf("result is not valid UTF-8: %q", got)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestDeduplicateItems(t *testing.T) {
	t.Parallel()
	items := []MemoryItem{
		{ID: "a", Memory: "first"},
		{ID: "b", Memory: "second"},
		{ID: "a", Memory: "duplicate"},
	}
	result := DeduplicateItems(items)
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
}
