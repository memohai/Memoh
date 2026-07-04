package tui

import (
	"strings"
	"testing"
)

// Command events are the WS protocol's terminal answer to slash input; the
// CLI must render them instead of waiting for a stream "end" that never
// comes (the pre-fix behavior was an indefinite hang on "/help").
func TestCommandResultText(t *testing.T) {
	t.Parallel()

	r := &wsCommandResult{
		Title: "Quick actions",
		Text:  "Available Web quick actions: /help, /skill list.",
		Items: []struct {
			Title       string `json:"title"`
			Description string `json:"description,omitempty"`
		}{
			{Title: "/help", Description: "Show available quick actions"},
			{Title: "/skill list"},
		},
	}
	got := commandResultText(r)
	for _, want := range []string{"Quick actions", "Available Web quick actions", "- /help — Show available quick actions", "- /skill list"} {
		if !strings.Contains(got, want) {
			t.Errorf("commandResultText missing %q in:\n%s", want, got)
		}
	}
	if commandResultText(nil) != "" {
		t.Error("nil result should render empty")
	}
}

func TestCommandErrorText(t *testing.T) {
	t.Parallel()

	if got := commandErrorText(&wsCommandError{Code: "unknown_slash", Message: "Unknown slash command."}); got != "Unknown slash command." {
		t.Errorf("commandErrorText = %q", got)
	}
	if got := commandErrorText(&wsCommandError{Code: "unknown_slash"}); got != "unknown_slash" {
		t.Errorf("commandErrorText code fallback = %q", got)
	}
	if got := commandErrorText(nil); got != "command failed" {
		t.Errorf("commandErrorText nil fallback = %q", got)
	}
}
