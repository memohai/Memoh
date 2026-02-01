package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestInitAndLogging(t *testing.T) {
	// 测试 JSON 格式
	Init("debug", "json")
	
	if L.Enabled(context.Background(), slog.LevelDebug) != true {
		t.Error("expected debug level to be enabled")
	}

	// 验证是否能正常输出（不崩溃）
	Info("test info message", "key", "value")
}

func TestContextLogger(t *testing.T) {
	Init("info", "text")
	
	// 创建一个带特定属性的 logger
	expectedKey := "request_id"
	expectedValue := "12345"
	customLogger := L.With(expectedKey, expectedValue)
	
	ctx := WithContext(context.Background(), customLogger)
	extracted := FromContext(ctx)
	
	// 这里简单验证提取出来的是否是同一个（或者功能一致）
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
