package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

var (
	L      = slog.Default()
	logKey = ctxKey{}
)

// Init initializes the global logger with the given level and format (e.g. "debug", "json").
func Init(level, format string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	L = slog.New(handler)
	slog.SetDefault(L)
}

// FromContext returns the logger from ctx, or the global logger if not set.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(logKey).(*slog.Logger); ok {
		return l
	}
	return L
}

// WithContext stores the logger in ctx and returns the new context.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, logKey, l)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug, Info, Warn, Error log with the global logger (slog.Attr or key-value pairs).
func Debug(msg string, args ...any) {
	L.Log(context.Background(), slog.LevelDebug, "global log", append([]any{slog.String("message", msg)}, args...)...)
}

func Info(msg string, args ...any) {
	L.Log(context.Background(), slog.LevelInfo, "global log", append([]any{slog.String("message", msg)}, args...)...)
}

func Warn(msg string, args ...any) {
	L.Log(context.Background(), slog.LevelWarn, "global log", append([]any{slog.String("message", msg)}, args...)...)
}

func Error(msg string, args ...any) {
	L.Log(context.Background(), slog.LevelError, "global log", append([]any{slog.String("message", msg)}, args...)...)
}
