package sessionmode

import (
	"testing"
)

func TestIsInteractive(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"", Chat, "CHAT", ACPAgent} {
		if !IsInteractive(mode) {
			t.Fatalf("expected %q to be interactive", mode)
		}
	}

	for _, mode := range []string{Discuss, Schedule, Heartbeat, Subagent} {
		if IsInteractive(mode) {
			t.Fatalf("expected %q to be non-interactive", mode)
		}
	}
}
