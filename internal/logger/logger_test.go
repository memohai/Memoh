package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestContextLogger(t *testing.T) {
	expectedKey := "request_id"
	expectedValue := "12345"
	customLogger := L.With(slog.String(expectedKey, expectedValue))

	ctx := WithContext(context.Background(), customLogger)
	extracted := FromContext(ctx)

	if extracted != customLogger {
		t.Fatal("expected logger from context")
	}

	if fallback := FromContext(context.Background()); fallback != L {
		t.Fatal("expected global logger fallback")
	}
}
