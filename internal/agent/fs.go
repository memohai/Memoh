package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

// FSClient provides file operations against a bot's container filesystem.
type FSClient struct {
	provider bridge.Provider
	botID    string
	now      func() time.Time
	logger   *slog.Logger
}

// NewFSClient creates a new container filesystem client.
func NewFSClient(provider bridge.Provider, botID string, now func() time.Time, logger *slog.Logger) *FSClient {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FSClient{provider: provider, botID: botID, now: now, logger: logger}
}

// ReadText reads a text file from the container, returning its content as a string.
// Returns an empty string if the file does not exist or cannot be read.
func (f *FSClient) ReadText(ctx context.Context, path string) (string, error) {
	if f.provider == nil {
		return "", nil
	}
	client, err := f.provider.MCPClient(ctx, f.botID)
	if err != nil {
		return "", fmt.Errorf("mcp client: %w", err)
	}
	resp, err := client.ReadFile(ctx, path, 0, 0)
	if err != nil {
		return "", err
	}
	return resp.GetContent(), nil
}

// ReadTextSafe reads a text file, returning empty string on any error.
func (f *FSClient) ReadTextSafe(ctx context.Context, path string) string {
	content, _ := f.ReadText(ctx, path)
	return content
}

// LoadSystemFiles loads the standard set of system files from the bot container.
// Logs warnings for files that fail to load (container down, bridge unreachable).
func (f *FSClient) LoadSystemFiles(ctx context.Context) []SystemFile {
	home := "/data"
	now := time.Now()
	if f.now != nil {
		now = f.now()
	}
	pad := func(n int) string { return fmt.Sprintf("%02d", n) }
	today := fmt.Sprintf("%d-%s-%s", now.Year(), pad(int(now.Month())), pad(now.Day()))
	yesterday := now.AddDate(0, 0, -1)
	yesterdayStr := fmt.Sprintf("%d-%s-%s", yesterday.Year(), pad(int(yesterday.Month())), pad(yesterday.Day()))

	filenames := []string{
		"IDENTITY.md",
		"SOUL.md",
		"TOOLS.md",
		"MEMORY.md",
		"PROFILES.md",
		"memory/" + today + ".md",
		"memory/" + yesterdayStr + ".md",
	}

	files := make([]SystemFile, len(filenames))
	for i, name := range filenames {
		content, err := f.ReadText(ctx, home+"/"+name)
		if err != nil {
			f.logger.Warn("failed to read container file",
				slog.String("bot_id", f.botID),
				slog.String("file", name),
				slog.Any("error", err),
			)
			content = ""
		}
		files[i] = SystemFile{
			Filename: name,
			Content:  strings.TrimSpace(content),
		}
	}
	return files
}
