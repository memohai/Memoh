// Package tools provides bridge-based resource loader for progressive skill loading.
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

// BridgeResourceLoader implements ResourceLoader using bridge client.
// This enables Level 3 on-demand loading of skill resources from containers.
type BridgeResourceLoader struct {
	client bridge.Provider
	botID  string
}

// NewBridgeResourceLoader creates a new resource loader for the given bot.
func NewBridgeResourceLoader(client bridge.Provider, botID string) *BridgeResourceLoader {
	return &BridgeResourceLoader{
		client: client,
		botID:  botID,
	}
}

// ReadFile reads a file from the skill's directory in the container.
// skillDir is the skill's root directory (e.g., "/data/.skills/public/chart-visualization")
// resourcePath is the relative path within the skill directory.
func (l *BridgeResourceLoader) ReadFile(ctx context.Context, skillDir string, resourcePath string) ([]byte, error) {
	client, err := l.client.MCPClient(ctx, l.botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP client: %w", err)
	}

	// Build full path
	fullPath := resourcePath
	if skillDir != "" {
		fullPath = skillDir + "/" + resourcePath
	}

	// Remove leading slash for gRPC call (bridge expects relative paths)
	fullPath = strings.TrimPrefix(fullPath, "/")

	resp, err := client.ReadFile(ctx, fullPath, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

	return []byte(resp.GetContent()), nil
}

// ListDir lists files in a directory within the skill.
// skillDir is the skill's root directory.
// subDir is the relative directory to list (e.g., "references", "scripts").
func (l *BridgeResourceLoader) ListDir(ctx context.Context, skillDir string, subDir string) ([]string, error) {
	client, err := l.client.MCPClient(ctx, l.botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP client: %w", err)
	}

	// Build full path
	fullPath := subDir
	if skillDir != "" {
		fullPath = skillDir + "/" + subDir
	}

	// Remove leading slash for gRPC call
	fullPath = strings.TrimPrefix(fullPath, "/")

	entries, err := client.ListDir(ctx, fullPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory %s: %w", fullPath, err)
	}

	var names []string
	for _, entry := range entries {
		name := entry.GetPath()
		// Remove skillDir prefix if present to get relative path
		name = strings.TrimPrefix(name, skillDir+"/")
		name = strings.TrimPrefix(name, "/")
		names = append(names, name)
	}

	return names, nil
}

// Ensure BridgeResourceLoader implements ResourceLoader interface
var _ ResourceLoader = (*BridgeResourceLoader)(nil)
