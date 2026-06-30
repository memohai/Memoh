package command

import (
	"context"
	"strings"
	"testing"
)

func TestRenderProgressBar(t *testing.T) {
	t.Parallel()
	if got := renderProgressBar(0.5, 10); got != strings.Repeat("█", 5)+strings.Repeat("░", 5) {
		t.Errorf("bar 0.5 = %q", got)
	}
	if got := renderProgressBar(2, 4); got != strings.Repeat("█", 4) {
		t.Errorf("bar clamp high = %q", got)
	}
	if got := renderProgressBar(-1, 4); got != strings.Repeat("░", 4) {
		t.Errorf("bar clamp low = %q", got)
	}
}

func TestContextCommandRendersUsageNoWindow(t *testing.T) {
	t.Parallel()
	h := newTestHandlerWithQueries(&fakeRoleResolver{role: "owner"}, &fakeCommandQueries{
		messageCount: 7, latestUsage: 1500,
	})
	out, err := h.ExecuteWithInput(context.Background(), ExecuteInput{
		BotID:             "b",
		ChannelIdentityID: "owner-1",
		Text:              "/context",
		SessionID:         "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "**Context**") {
		t.Errorf("missing bold title: %s", out)
	}
	if !strings.Contains(out, "Messages: 7") {
		t.Errorf("missing message count: %s", out)
	}
	// No model service wired => no window => the "N tokens used" fallback path.
	if !strings.Contains(out, "1.5K tokens used") {
		t.Errorf("missing used tokens: %s", out)
	}
}
