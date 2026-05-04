package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AdminCredentials mirrors the subset of [admin] in config.toml that we
// need to authenticate against the local server.
type AdminCredentials struct {
	Username string
	Password string //nolint:gosec // local-only desktop credential read from userData/config.toml
}

// CachedToken is the on-disk shape of cli-token.json.
type CachedToken struct {
	BaseURL   string    `json:"base_url"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedAt  time.Time `json:"issued_at"`
}

// Valid reports whether the cached token is non-empty and not within
// the configurable refresh window of expiring.
func (c CachedToken) Valid(now time.Time, refreshBefore time.Duration) bool {
	if c.Token == "" {
		return false
	}
	if c.ExpiresAt.IsZero() {
		// Treat zero ExpiresAt as "unknown" — be conservative and
		// re-login. The server always returns expires_at so this is
		// only hit on corrupted/legacy cache files.
		return false
	}
	return now.Add(refreshBefore).Before(c.ExpiresAt)
}

var adminKeyValuePattern = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=\s*"(.*)"\s*$`)

// ReadAdminCredentials parses the [admin] section of the config file at
// configPath and returns the username/password pair. Mirrors
// readAdminCredentials in apps/desktop/src/main/local-server.ts.
func ReadAdminCredentials(configPath string) (AdminCredentials, error) {
	raw, err := os.ReadFile(configPath) //nolint:gosec // configPath is derived from UserDataDir, not user input
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("read config %s: %w", configPath, err)
	}
	var creds AdminCredentials
	inAdmin := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inAdmin = trimmed == "[admin]"
			continue
		}
		if !inAdmin || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := adminKeyValuePattern.FindStringSubmatch(trimmed)
		if len(match) != 3 {
			continue
		}
		switch match[1] {
		case "username":
			creds.Username = match[2]
		case "password":
			creds.Password = match[2]
		}
	}
	if creds.Username == "" || creds.Password == "" {
		return AdminCredentials{}, fmt.Errorf("missing [admin] username/password in %s", configPath)
	}
	return creds, nil
}

// SelfLogin posts the [admin] credentials from configPath to baseURL's
// /auth/login endpoint and returns a fresh JWT.
func SelfLogin(ctx context.Context, baseURL, configPath string) (CachedToken, error) {
	creds, err := ReadAdminCredentials(configPath)
	if err != nil {
		return CachedToken{}, err
	}
	body, err := json.Marshal(map[string]string{
		"username": creds.Username,
		"password": creds.Password,
	})
	if err != nil {
		return CachedToken{}, fmt.Errorf("encode login payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return CachedToken{}, fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // baseURL is the CLI's local server endpoint, not user-controlled
	if err != nil {
		return CachedToken{}, fmt.Errorf("post login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return CachedToken{}, fmt.Errorf("self-login failed: HTTP %d %s", resp.StatusCode, string(raw))
	}
	var payload struct {
		AccessToken string `json:"access_token"` //nolint:gosec // expected JWT field name from the server
		ExpiresAt   string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CachedToken{}, fmt.Errorf("decode login response: %w", err)
	}
	if payload.AccessToken == "" {
		return CachedToken{}, errors.New("self-login: server returned empty access_token")
	}
	expiresAt := time.Time{}
	if payload.ExpiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, payload.ExpiresAt); err == nil {
			expiresAt = parsed
		}
	}
	return CachedToken{
		BaseURL:   baseURL,
		Token:     payload.AccessToken,
		ExpiresAt: expiresAt,
		IssuedAt:  time.Now().UTC(),
	}, nil
}

// LoadCachedToken reads cli-token.json and returns its parsed contents.
// A missing file returns (CachedToken{}, nil) so callers can treat it
// as "not yet logged in" without distinguishing IO errors from absence.
func LoadCachedToken(path string) (CachedToken, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is derived from UserDataDir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CachedToken{}, nil
		}
		return CachedToken{}, fmt.Errorf("read token cache %s: %w", path, err)
	}
	var token CachedToken
	if err := json.Unmarshal(raw, &token); err != nil {
		return CachedToken{}, fmt.Errorf("decode token cache %s: %w", path, err)
	}
	return token, nil
}

// SaveCachedToken atomically writes the token to path with mode 0600.
func SaveCachedToken(path string, token CachedToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token cache dir: %w", err)
	}
	encoded, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("write token cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename token cache: %w", err)
	}
	return nil
}

// EnsureToken returns a valid bearer token for baseURL, performing a
// self-login if the on-disk cache is missing, expired, or for a
// different baseURL. The returned token is also written back to disk so
// subsequent invocations skip the network round-trip.
func EnsureToken(ctx context.Context, baseURL, configPath string) (string, error) {
	cachePath, err := TokenCachePath()
	if err != nil {
		return "", err
	}
	cached, err := LoadCachedToken(cachePath)
	if err != nil {
		return "", err
	}
	if cached.BaseURL == baseURL && cached.Valid(time.Now().UTC(), time.Minute) {
		return cached.Token, nil
	}
	fresh, err := SelfLogin(ctx, baseURL, configPath)
	if err != nil {
		return "", err
	}
	if err := SaveCachedToken(cachePath, fresh); err != nil {
		// Saving is best-effort; surface the error in stderr-style logs
		// elsewhere if needed. Returning the token still lets the
		// current command succeed.
		_ = err
	}
	return fresh.Token, nil
}
