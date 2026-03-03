package container

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's", `'it'\''s'`},
		{"a b", "'a b'"},
	}
	for _, tt := range tests {
		got := ShellQuote(tt.in)
		if got != tt.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestApplyEdit(t *testing.T) {
	raw := "hello world\n"
	updated, err := applyEdit(raw, "test.txt", "hello", "goodbye")
	if err != nil {
		t.Fatal(err)
	}
	if updated != "goodbye world\n" {
		t.Errorf("updated = %q", updated)
	}
}

func TestApplyEdit_NotFound(t *testing.T) {
	raw := "hello world\n"
	_, err := applyEdit(raw, "test.txt", "missing text", "new")
	if err == nil {
		t.Error("expected error for missing text")
	}
}

func TestApplyEdit_MultipleOccurrences(t *testing.T) {
	raw := "foo bar foo\n"
	_, err := applyEdit(raw, "test.txt", "foo", "baz")
	if err == nil {
		t.Error("expected error for multiple occurrences")
	}
}

func TestApplyEdit_NoChange(t *testing.T) {
	raw := "hello world\n"
	_, err := applyEdit(raw, "test.txt", "hello", "hello")
	if err == nil {
		t.Error("expected error for identical replacement")
	}
}

func TestFuzzyFindText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		found   bool
	}{
		{"exact match", "hello world", "hello", true},
		{"no match", "hello world", "missing", false},
		{"smart quote match", "it\u2019s a test", "it's a test", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fuzzyFindText(tt.content, tt.old)
			if result.Found != tt.found {
				t.Errorf("found = %v, want %v", result.Found, tt.found)
			}
		})
	}
}

func TestDetectLineEnding(t *testing.T) {
	if detectLineEnding("foo\r\nbar") != "\r\n" {
		t.Error("expected CRLF")
	}
	if detectLineEnding("foo\nbar") != "\n" {
		t.Error("expected LF")
	}
	if detectLineEnding("foo") != "\n" {
		t.Error("expected LF default")
	}
}

func TestStripBOM(t *testing.T) {
	bom, content := stripBOM("\uFEFFhello")
	if bom != "\uFEFF" || content != "hello" {
		t.Errorf("bom=%q content=%q", bom, content)
	}
	bom2, content2 := stripBOM("hello")
	if bom2 != "" || content2 != "hello" {
		t.Errorf("bom=%q content=%q", bom2, content2)
	}
}
