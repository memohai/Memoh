package feishu

import (
	"testing"
)

func TestExtractReadableFromJSON(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"json with text", `{"text":"extracted"}`, "extracted"},
		{"json with message", `{"message":"ok"}`, "ok"},
		{"json with content", `{"content":"result"}`, "result"},
		{"invalid json", `{invalid`, `{invalid`},
		{"empty object", `{}`, `{}`},
		{"array of strings", `["first"]`, "first"},
		{"array empty", `[]`, `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractReadableFromJSON(tc.in)
			if got != tc.want {
				t.Errorf("extractReadableFromJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
