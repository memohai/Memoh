package feishu

import (
	"context"
	"fmt"
	"log/slog"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type larkSlogLogger struct {
	logger *slog.Logger
}

func newLarkSlogLogger(logger *slog.Logger) larkcore.Logger {
	if logger == nil {
		return nil
	}
	return &larkSlogLogger{logger: logger}
}

func (l *larkSlogLogger) Debug(ctx context.Context, args ...any) {
	l.log(ctx, slog.LevelDebug, args...)
}

func (l *larkSlogLogger) Info(ctx context.Context, args ...any) {
	l.log(ctx, slog.LevelInfo, args...)
}

func (l *larkSlogLogger) Warn(ctx context.Context, args ...any) {
	l.log(ctx, slog.LevelWarn, args...)
}

func (l *larkSlogLogger) Error(ctx context.Context, args ...any) {
	l.log(ctx, slog.LevelError, args...)
}

func (l *larkSlogLogger) log(ctx context.Context, level slog.Level, args ...any) {
	if l.logger == nil {
		return
	}
	msg := fmt.Sprint(args...)
	l.logger.Log(ctx, level, "feishu sdk", slog.String("detail", msg))
}
