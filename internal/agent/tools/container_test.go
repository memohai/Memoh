package tools

import "testing"

func TestDetectBlockedSleep(t *testing.T) {
	tests := []struct {
		command string
		blocked bool
	}{
		// Should block
		{"sleep 5", true},
		{"sleep 10", true},
		{"sleep 30", true},
		{"sleep 5 && echo done", true},
		{"sleep 5; echo done", true},

		// Should allow
		{"sleep 1", false},       // under 2 seconds
		{"sleep 0.5", false},     // under 2 seconds
		{"echo hello", false},    // not sleep
		{"npm install", false},   // not sleep
		{"echo sleep 5", false},  // sleep not at start
		{"cat sleep.txt", false}, // not the sleep command
	}

	for _, tt := range tests {
		result := detectBlockedSleep(tt.command)
		if tt.blocked && result == "" {
			t.Errorf("expected %q to be blocked, but it was allowed", tt.command)
		}
		if !tt.blocked && result != "" {
			t.Errorf("expected %q to be allowed, but got: %s", tt.command, result)
		}
	}
}
