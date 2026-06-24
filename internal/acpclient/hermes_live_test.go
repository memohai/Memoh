package acpclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestHermesACPLiveLocalManagedProvider(t *testing.T) {
	if os.Getenv("MEMOH_LIVE_HERMES_ACP") != "1" {
		t.Skip("set MEMOH_LIVE_HERMES_ACP=1 to run the live Hermes ACP local managed provider smoke test")
	}
	apiKey := strings.TrimSpace(os.Getenv("MEMOH_LIVE_HERMES_API_KEY"))
	if apiKey == "" {
		t.Skip("MEMOH_LIVE_HERMES_API_KEY is required for the live Hermes ACP smoke test")
	}
	provider := strings.TrimSpace(os.Getenv("MEMOH_LIVE_HERMES_PROVIDER"))
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MEMOH_LIVE_HERMES_BASE_URL")), "/")
	if provider == "" {
		if baseURL == "" {
			provider = "gemini"
		} else {
			provider = "custom"
		}
	}
	if strings.EqualFold(provider, "custom") && baseURL == "" {
		t.Skip("MEMOH_LIVE_HERMES_BASE_URL is required for the live Hermes ACP custom provider smoke test")
	}
	model := strings.TrimSpace(os.Getenv("MEMOH_LIVE_HERMES_MODEL"))
	if model == "" {
		if strings.EqualFold(provider, "gemini") || strings.EqualFold(provider, "google") || strings.EqualFold(provider, "google-ai-studio") {
			model = "gemini-3.5-flash"
		} else {
			model = "gpt-4o-mini"
		}
	}

	root, err := os.MkdirTemp("/tmp", "memoh-hermes-live-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Memoh Hermes ACP live smoke\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		BotID:         "bot-live-hermes",
		Backend:       bridge.WorkspaceBackendLocal,
		WorkspaceRoot: root,
		ProjectPath:   "/data/project",
		LocalDataRoot: root,
	})
	if err != nil {
		t.Fatalf("resolve live Hermes session context: %v", err)
	}
	managed := map[string]string{
		"provider": provider,
		"model":    model,
		"api_key":  apiKey,
	}
	if baseURL != "" {
		managed["base_url"] = baseURL
	}
	if err := WriteHermesManagedConfigToLocalFS(HermesManagedConfig{
		Home:    resolved.HermesHome,
		Managed: managed,
	}); err != nil {
		t.Fatalf("write live Hermes managed config: %v", err)
	}

	runner := NewRunner(nil, testWorkspace{
		client: newTestBridgeClient(t, root),
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sess, err := runner.StartSession(ctx, StartRequest{
		AgentID:     "hermes",
		BotID:       "bot-live-hermes",
		ProjectPath: "/data/project",
		Command:     "uv",
		Args: []string{
			"tool",
			"run",
			"--from",
			"hermes-agent[acp,mcp]==0.17.0",
			"hermes-acp",
		},
		SetupMode: SetupModeAPIKey,
		Resolved:  &resolved,
		Timeout:   2 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatalf("start live Hermes ACP session failed: %v", err)
	}
	defer func() { _ = sess.Close() }()

	const marker = "memoh-hermes-live-ok"
	result, err := sess.Prompt(ctx, "This is a connectivity smoke test. Reply with exactly this token and do not call tools: "+marker)
	if err != nil {
		t.Fatalf("live Hermes ACP prompt failed: %v\n%s", err, hermesLiveFailureDetails(sess, resolved.HermesHome, apiKey))
	}
	if !strings.Contains(strings.ToLower(result.Text), marker) {
		t.Fatalf("live Hermes ACP text = %q, stop_reason=%q events=%s, want marker %s\n%s",
			result.Text, result.StopReason, summarizeHermesLiveEvents(result.Events), marker,
			hermesLiveFailureDetails(sess, resolved.HermesHome, apiKey))
	}
}

func summarizeHermesLiveEvents(events []event.StreamEvent) string {
	if len(events) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, ev := range events {
		if i > 0 {
			b.WriteString(", ")
		}
		if i >= 12 {
			fmt.Fprintf(&b, "...+%d", len(events)-i)
			break
		}
		b.WriteString(string(ev.Type))
		if ev.ToolName != "" {
			b.WriteString(":")
			b.WriteString(ev.ToolName)
		}
		if ev.Status != "" {
			b.WriteString(":")
			b.WriteString(ev.Status)
		}
		if ev.Error != "" {
			b.WriteString(":error=")
			b.WriteString(ev.Error)
		}
	}
	b.WriteByte(']')
	return b.String()
}

func hermesLiveFailureDetails(sess *Session, home, apiKey string) string {
	var parts []string
	if sess != nil && sess.proc != nil && sess.proc.tail != nil {
		if stderr := sanitizeHermesLiveLog(sess.proc.tail.String(), apiKey); strings.TrimSpace(stderr) != "" {
			parts = append(parts, "stderr tail:\n"+stderr)
		}
	}
	for _, path := range hermesLiveLogFiles(home) {
		data, err := os.ReadFile(path) //nolint:gosec // live test only reads logs under its temp HERMES_HOME.
		if err != nil || len(data) == 0 {
			continue
		}
		text := tailString(string(data), 4096)
		text = sanitizeHermesLiveLog(text, apiKey)
		if strings.TrimSpace(text) != "" {
			parts = append(parts, filepath.Base(path)+":\n"+text)
		}
	}
	if len(parts) == 0 {
		return "no Hermes stderr/log details captured"
	}
	return strings.Join(parts, "\n\n")
}

func hermesLiveLogFiles(home string) []string {
	logDir := filepath.Join(home, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".log") {
			files = append(files, filepath.Join(logDir, name))
		}
	}
	return files
}

var bearerTokenPattern = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s]+`)

func sanitizeHermesLiveLog(text, apiKey string) string {
	if apiKey != "" {
		text = strings.ReplaceAll(text, apiKey, "<redacted>")
	}
	return bearerTokenPattern.ReplaceAllString(text, "${1}<redacted>")
}

func tailString(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
