package inbound

import "testing"

func TestDetectMode(t *testing.T) {
	tests := []struct {
		input    string
		wantMode InboundMode
		wantText string
	}{
		{"hello world", ModeInject, "hello world"},
		{"/btw hello", ModeInject, "hello"},
		{"/now hello", ModeParallel, "hello"},
		{"/next hello", ModeQueue, "hello"},
		{"/BTW hello", ModeInject, "hello"},
		{"/NOW hello", ModeParallel, "hello"},
		{"/NEXT hello", ModeQueue, "hello"},
		{"/next\tqueued", ModeQueue, "queued"},
		{"/btw\nfollow up", ModeInject, "follow up"},
		{"/next\n\n[Voice transcript]\nhello", ModeQueue, "[Voice transcript]\nhello"},
		{"/now", ModeParallel, ""},
		{"/next", ModeQueue, ""},
		{"/btw", ModeInject, ""},
		{"  /now  hello  ", ModeParallel, "hello"},
		{"/unknown hello", ModeInject, "/unknown hello"},
		{"", ModeInject, ""},
		{"/new session", ModeInject, "/new session"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mode, text := DetectMode(tt.input)
			if mode != tt.wantMode {
				t.Errorf("DetectMode(%q) mode = %d, want %d", tt.input, mode, tt.wantMode)
			}
			if text != tt.wantText {
				t.Errorf("DetectMode(%q) text = %q, want %q", tt.input, text, tt.wantText)
			}
		})
	}
}

func TestIsStartCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/start", true},
		{"/start@MemohBot", true},
		{"/start abc123", true},
		{"/start@MemohBot abc123", true},
		{"/start deep_link_payload", true},
		{"/new", false},
		{"/started", false},
		{"start", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isStartCommand(tt.input); got != tt.want {
				t.Errorf("isStartCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsModeCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/btw hello", true},
		{"/now hello", true},
		{"/next hello", true},
		{"/next\tqueued", true},
		{"/btw\nfollow up", true},
		{"/btw", true},
		{"/now", true},
		{"/next", true},
		{"/new", false},
		{"/fs list", false},
		{"hello", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsModeCommand(tt.input); got != tt.want {
				t.Errorf("IsModeCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
