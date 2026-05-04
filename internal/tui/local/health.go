package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HealthStatus is the parsed payload of GET /ping on the desktop server.
type HealthStatus struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	CommitHash  string `json:"commit_hash"`
	ReachableAt string `json:"-"`
}

// Probe issues a single short-timeout GET /ping. A non-2xx response or
// transport error returns a non-nil error so callers can decide
// whether to escalate to "server not running".
func Probe(ctx context.Context, baseURL string) (HealthStatus, error) {
	if baseURL == "" {
		baseURL = LocalServerBaseURL
	}
	target := strings.TrimRight(baseURL, "/") + "/ping"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return HealthStatus{}, err
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req) //nolint:gosec // baseURL is the CLI's local server endpoint, not user-controlled
	if err != nil {
		return HealthStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return HealthStatus{}, fmt.Errorf("ping returned HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var payload HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return HealthStatus{}, fmt.Errorf("decode ping payload: %w", err)
	}
	payload.ReachableAt = baseURL
	return payload, nil
}

// WaitForReady polls Probe until it returns a healthy status or the
// deadline elapses. Used by `memoh start` after spawning the binary.
func WaitForReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		status, err := Probe(probeCtx, baseURL)
		cancel()
		if err == nil && status.Status != "" {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = errors.New("server did not become ready before timeout")
	}
	return fmt.Errorf("wait for server ready: %w", lastErr)
}
