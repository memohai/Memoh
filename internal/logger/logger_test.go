package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestInitAndLogging(t *testing.T) {
	Init("debug", "json")

	if L.Enabled(context.Background(), slog.LevelDebug) != true {
		t.Error("expected debug level to be enabled")
	}

	Info("test info message", "key", "value")
}

func TestContextLogger(t *testing.T) {
	Init("info", "text")

	expectedKey := "request_id"
	expectedValue := "12345"
	customLogger := L.With(slog.String(expectedKey, expectedValue))

	ctx := WithContext(context.Background(), customLogger)
	extracted := FromContext(ctx)

	if extracted == nil {
		t.Fatal("extracted logger should not be nil")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"Warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		if got := parseLevel(tt.input); got != tt.expected {
			t.Errorf("parseLevel(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}
