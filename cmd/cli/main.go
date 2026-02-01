package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/version"
	"github.com/memohai/memoh/internal/logger"
)

type cliOptions struct {
	configPath string
	username   string
	password   string
	timeout    time.Duration
	apiBaseURL string
	jwtToken   string
	showVersion bool
}

func main() {
	opts := parseFlags()
	if opts.showVersion {
		fmt.Printf("Memoh CLI %s\n", version.GetInfo())
		return
	}
	ctx := context.Background()

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)

	if strings.TrimSpace(opts.apiBaseURL) == "" {
		opts.apiBaseURL = defaultAPIBaseURL(cfg.Server.Addr)
	}
	if strings.TrimSpace(opts.apiBaseURL) == "" {
		logger.Error("api url is required")
		os.Exit(1)
	}
	opts.apiBaseURL = normalizeBaseURL(opts.apiBaseURL)

	jwtToken := strings.TrimSpace(opts.jwtToken)
	client := &http.Client{Timeout: opts.timeout}
	if jwtToken == "" {
		username, password, err := resolveLoginCredentials(opts, cfg)
		if err != nil {
			logger.Error("resolve login", slog.Any("error", err))
			os.Exit(1)
		}
		loginCtx := ctx
		if opts.timeout > 0 {
			var cancel context.CancelFunc
			loginCtx, cancel = context.WithTimeout(ctx, opts.timeout)
			defer cancel()
		}
		jwtToken, err = resolveJWTToken(loginCtx, client, opts.apiBaseURL, username, password)
		if err != nil {
			logger.Error("resolve jwt", slog.Any("error", err))
			os.Exit(1)
		}
	}

	query := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if query != "" {
		if err := sendChat(ctx, client, opts.apiBaseURL, jwtToken, query); err != nil {
			logger.Error("chat failed", slog.Any("error", err))
			os.Exit(1)
		}
		return
	}
	if err := runInteractive(ctx, client, opts.apiBaseURL, jwtToken); err != nil {
		logger.Error("chat failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func parseFlags() cliOptions {
	var opts cliOptions
	defaultConfig := os.Getenv("CONFIG_PATH")
	if strings.TrimSpace(defaultConfig) == "" {
		defaultConfig = config.DefaultConfigPath
	}

	flag.StringVar(&opts.configPath, "config", defaultConfig, "Path to config.toml")
	flag.StringVar(&opts.username, "username", "", "Username for login")
	flag.StringVar(&opts.password, "password", "", "Password for login (or set MEMOH_PASSWORD)")
	flag.StringVar(&opts.jwtToken, "jwt", "", "JWT token (optional)")
	flag.StringVar(&opts.apiBaseURL, "api-url", "", "API server base URL (e.g. http://127.0.0.1:8080)")
	flag.DurationVar(&opts.timeout, "timeout", 30*time.Second, "Request timeout")
	flag.BoolVar(&opts.showVersion, "version", false, "Show version information")
	flag.Parse()

	return opts
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func defaultAPIBaseURL(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return normalizeBaseURL(trimmed)
	}
	if strings.HasPrefix(trimmed, ":") {
		return "http://127.0.0.1" + trimmed
	}
	return "http://" + trimmed
}

func resolveLoginCredentials(opts cliOptions, cfg config.Config) (string, string, error) {
	username := strings.TrimSpace(opts.username)
	if username == "" {
		username = strings.TrimSpace(cfg.Admin.Username)
	}
	if username == "" {
		return "", "", fmt.Errorf("username is required for login")
	}

	password := strings.TrimSpace(opts.password)
	if password == "" {
		password = strings.TrimSpace(os.Getenv("MEMOH_PASSWORD"))
	}
	if password == "" {
		if candidate := strings.TrimSpace(cfg.Admin.Password); candidate != "" && candidate != "change-your-password-here" {
			password = candidate
		}
	}
	if password == "" {
		return "", "", fmt.Errorf("password is required; pass --password or set MEMOH_PASSWORD")
	}
	return username, password, nil
}

func resolveJWTToken(ctx context.Context, client *http.Client, baseURL, username, password string) (string, error) {
	resp, err := loginForToken(ctx, client, baseURL, username, password)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.AccessToken) == "" {
		return "", fmt.Errorf("login succeeded but token missing")
	}
	return resp.AccessToken, nil
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
}

func loginForToken(ctx context.Context, client *http.Client, baseURL, username, password string) (loginResponse, error) {
	body, err := json.Marshal(loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return loginResponse{}, err
	}
	url := normalizeBaseURL(baseURL) + "/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return loginResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return loginResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return loginResponse{}, fmt.Errorf("login failed: %s", strings.TrimSpace(string(payload)))
	}

	var parsed loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return loginResponse{}, err
	}
	return parsed, nil
}

func runInteractive(ctx context.Context, client *http.Client, baseURL, jwtToken string) error {
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	fmt.Fprint(os.Stdout, "You: ")
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			fmt.Fprint(os.Stdout, "You: ")
			continue
		}
		lower := strings.ToLower(line)
		if lower == "exit" || lower == "quit" {
			return nil
		}
		if err := sendChat(ctx, client, baseURL, jwtToken, line); err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, "You: ")
	}
	return reader.Err()
}

func sendChat(ctx context.Context, client *http.Client, baseURL, jwtToken, query string) error {
	return streamAPIChat(ctx, client, baseURL, jwtToken, chat.ChatRequest{Query: query})
}

func streamAPIChat(ctx context.Context, client *http.Client, baseURL, jwtToken string, req chat.ChatRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	url := baseURL + "/chat/stream"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+jwtToken)

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api server error: %s", strings.TrimSpace(string(payload)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			break
		}
		if text, ok := extractStreamText([]byte(data)); ok {
			fmt.Fprint(os.Stdout, text)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func renderMessageContent(raw interface{}) (string, bool) {
	switch v := raw.(type) {
	case string:
		return v, true
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := renderMessageContent(item); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ""), true
		}
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text, true
		}
		if text, ok := v["content"].(string); ok && strings.TrimSpace(text) != "" {
			return text, true
		}
		if kind, ok := v["type"].(string); ok && kind == "text" {
			if text, ok := v["text"].(string); ok {
				return text, true
			}
		}
	}
	return "", false
}

func extractStreamText(raw []byte) (string, bool) {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	return extractTextFromPayload(payload)
}

func extractTextFromPayload(payload interface{}) (string, bool) {
	switch v := payload.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := extractTextFromPayload(item); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ""), true
		}
	case map[string]interface{}:
		if text, ok := v["textDelta"].(string); ok && strings.TrimSpace(text) != "" {
			return text, true
		}
		if text, ok := v["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text, true
		}
		if content, ok := v["content"]; ok {
			if text, ok := renderMessageContent(content); ok {
				return text, true
			}
		}
		if delta, ok := v["delta"]; ok {
			if text, ok := extractTextFromPayload(delta); ok {
				return text, true
			}
		}
		if data, ok := v["data"]; ok {
			if text, ok := extractTextFromPayload(data); ok {
				return text, true
			}
		}
		if msg, ok := v["message"]; ok {
			if text, ok := extractTextFromPayload(msg); ok {
				return text, true
			}
		}
	}
	return "", false
}
