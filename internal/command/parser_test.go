package command

import (
	"testing"
)

func TestParse_Basic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		resource string
		action   string
		args     []string
	}{
		{"/help", "help", "", nil},
		{"/help model", "help", "model", nil},
		{"/help model set", "help", "model", []string{"set"}},
		{"/subagent list", "subagent", "list", nil},
		{"/subagent get mybot", "subagent", "get", []string{"mybot"}},
		{"/schedule create daily \"0 9 * * *\" Send report", "schedule", "create", []string{"daily", "0 9 * * *", "Send", "report"}},
		{"  /settings  ", "settings", "", nil},
		{"/HELP", "help", "", nil},
		{"/Schedule List", "schedule", "list", nil},
		{"/help@MemohBot", "help", "", nil},
		{"/schedule@BotName list", "schedule", "list", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			parsed, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Resource != tt.resource {
				t.Errorf("resource: got %q, want %q", parsed.Resource, tt.resource)
			}
			if parsed.Action != tt.action {
				t.Errorf("action: got %q, want %q", parsed.Action, tt.action)
			}
			if len(parsed.Args) != len(tt.args) {
				t.Fatalf("args length: got %d, want %d", len(parsed.Args), len(tt.args))
			}
			for i, arg := range tt.args {
				if parsed.Args[i] != arg {
					t.Errorf("arg[%d]: got %q, want %q", i, parsed.Args[i], arg)
				}
			}
		})
	}
}

func TestExtractCommandText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"/help", "/help"},
		{" /subagent list", "/subagent list"},
		{"@BotName /help", "/help"},
		{"@_user_1 /schedule list arg1", "/schedule list arg1"},
		{"<@123456> /mcp list", "/mcp list"},
		{"@bot hello", ""},
		{"hello world", ""},
		{"", ""},
		{"some text with no slash", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ExtractCommandText(tt.input)
			if got != tt.want {
				t.Errorf("ExtractCommandText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		"hello",
		"no slash",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(input)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestTokenize_Quotes(t *testing.T) {
	t.Parallel()
	tokens := tokenize(`create daily "0 9 * * *" 'Send report now'`)
	expected := []string{"create", "daily", "0 9 * * *", "Send report now"}
	if len(tokens) != len(expected) {
		t.Fatalf("tokens length: got %d, want %d (%v)", len(tokens), len(expected), tokens)
	}
	for i, tok := range expected {
		if tokens[i] != tok {
			t.Errorf("token[%d]: got %q, want %q", i, tokens[i], tok)
		}
	}
}
