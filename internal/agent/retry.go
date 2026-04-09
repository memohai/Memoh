package agent

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

// RetryConfig controls retry behavior for initial stream failures.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// serverErrPattern matches "api error 5XX" where XX is any two digits.
var serverErrPattern = regexp.MustCompile(`api error 5\d{2}`)

// DefaultRetryConfig returns sensible defaults for LLM provider retries.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    10 * time.Second,
	}
}

// isRetryableStreamError returns true for errors worth retrying.
func isRetryableStreamError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancelled/expired — do NOT retry (check first since
	// context.DeadlineExceeded also satisfies net.Error)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Network-level errors (connection refused, timeout, DNS)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// HTTP status errors: retry on 429 and 5xx
	errStr := err.Error()
	if strings.Contains(errStr, "429") {
		return true
	}
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "rate_limit") {
		return true
	}
	if serverErrPattern.MatchString(errStr) {
		return true
	}
	// Connection reset / EOF
	if strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection refused") {
		return true
	}
	return false
}

func retryBackoff(attempt int, cfg RetryConfig) time.Duration {
	delay := cfg.BaseDelay * time.Duration(1<<uint(attempt)) // exponential
	return min(delay, cfg.MaxDelay)
}
