package healthcheck

import "context"

const (
	// StatusOK indicates check passed.
	StatusOK = "ok"
	// StatusWarn indicates check completed with warning.
	StatusWarn = "warn"
	// StatusError indicates check failed.
	StatusError = "error"
	// StatusUnknown indicates check result is not yet known.
	StatusUnknown = "unknown"
)

// CheckResult is one runtime check item produced by a checker.
type CheckResult struct {
	ID       string
	Type     string
	TitleKey string
	Subtitle string
	Status   string
	Summary  string
	Detail   string
	Metadata map[string]any
}

// Checker evaluates one or more runtime checks for a bot.
type Checker interface {
	ListChecks(ctx context.Context, botID string) []CheckResult
}
