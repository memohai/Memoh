package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/mcp/providers/container"
)

const (
	manifestPath  = "index/manifest.json"
	memoryDirPath = "memory"
	manifestVer   = 1
)

// MemoryFS persists memory entries as files inside the bot container via ExecRunner.
type MemoryFS struct {
	execRunner container.ExecRunner
	workDir    string // e.g. "/data"
	logger     *slog.Logger
	mu         sync.Mutex // serialize manifest updates
}

// Manifest is the index file that records everything needed to rebuild memories.
type Manifest struct {
	Version   int                      `json:"version"`
	UpdatedAt string                   `json:"updated_at"`
	Entries   map[string]ManifestEntry `json:"entries"`
}

// ManifestEntry records metadata for a single memory entry.
type ManifestEntry struct {
	Hash      string         `json:"hash"`
	CreatedAt string         `json:"created_at"`
	Lang      string         `json:"lang,omitempty"`
	Filters   map[string]any `json:"filters,omitempty"`
}

// NewMemoryFS creates a MemoryFS that writes through the given ExecRunner.
func NewMemoryFS(log *slog.Logger, runner container.ExecRunner, workDir string) *MemoryFS {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(workDir) == "" {
		workDir = "/data"
	}
	return &MemoryFS{
		execRunner: runner,
		workDir:    workDir,
		logger:     log.With(slog.String("component", "memoryfs")),
	}
}

// ----- write operations -----

// PersistMemories writes .md files for new items and incrementally updates the manifest.
// Used after Add — does NOT delete existing files.
func (fs *MemoryFS) PersistMemories(ctx context.Context, botID string, items []MemoryItem, filters map[string]any) error {
	if len(items) == 0 {
		return nil
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Read existing manifest (or create new one).
	manifest, _ := fs.readManifestLocked(ctx, botID)
	if manifest == nil {
		manifest = &Manifest{
			Version: manifestVer,
			Entries: map[string]ManifestEntry{},
		}
	}

	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Memory) == "" {
			continue
		}
		// Write individual .md file.
		if err := fs.writeMemoryFile(ctx, botID, item); err != nil {
			fs.logger.Warn("write memory file failed", slog.String("id", item.ID), slog.Any("error", err))
			continue
		}
		// Update manifest entry.
		manifest.Entries[item.ID] = ManifestEntry{
			Hash:      item.Hash,
			CreatedAt: item.CreatedAt,
			Filters:   filters,
		}
	}

	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return fs.writeManifest(ctx, botID, manifest)
}

// RebuildFiles does a full replace: deletes all old memory/*.md files, writes new ones,
// and rewrites manifest from scratch. Used after Compact.
func (fs *MemoryFS) RebuildFiles(ctx context.Context, botID string, items []MemoryItem, filters map[string]any) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Delete old memory dir contents.
	fs.execDeleteDir(ctx, botID, memoryDirPath)

	manifest := &Manifest{
		Version:   manifestVer,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   make(map[string]ManifestEntry, len(items)),
	}

	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Memory) == "" {
			continue
		}
		if err := fs.writeMemoryFile(ctx, botID, item); err != nil {
			fs.logger.Warn("rebuild write memory file failed", slog.String("id", item.ID), slog.Any("error", err))
			continue
		}
		manifest.Entries[item.ID] = ManifestEntry{
			Hash:      item.Hash,
			CreatedAt: item.CreatedAt,
			Filters:   filters,
		}
	}

	return fs.writeManifest(ctx, botID, manifest)
}

// RemoveMemories removes specific memory files from the FS and updates the manifest.
func (fs *MemoryFS) RemoveMemories(ctx context.Context, botID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	manifest, _ := fs.readManifestLocked(ctx, botID)
	if manifest == nil {
		manifest = &Manifest{Version: manifestVer, Entries: map[string]ManifestEntry{}}
	}

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		fs.execDeleteFile(ctx, botID, fmt.Sprintf("%s/%s.md", memoryDirPath, id))
		delete(manifest.Entries, id)
	}

	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return fs.writeManifest(ctx, botID, manifest)
}

// RemoveAllMemories deletes all memory files and the manifest.
func (fs *MemoryFS) RemoveAllMemories(ctx context.Context, botID string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.execDeleteDir(ctx, botID, memoryDirPath)
	emptyManifest := &Manifest{
		Version:   manifestVer,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]ManifestEntry{},
	}
	return fs.writeManifest(ctx, botID, emptyManifest)
}

// ----- read operations -----

// ReadManifest reads and parses the manifest.json file.
func (fs *MemoryFS) ReadManifest(ctx context.Context, botID string) (*Manifest, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.readManifestLocked(ctx, botID)
}

func (fs *MemoryFS) readManifestLocked(ctx context.Context, botID string) (*Manifest, error) {
	content, err := container.ExecRead(ctx, fs.execRunner, botID, fs.workDir, manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &manifest, nil
}

// ReadAllMemoryFiles lists and reads all .md files under memory/ and parses their frontmatter.
func (fs *MemoryFS) ReadAllMemoryFiles(ctx context.Context, botID string) ([]MemoryItem, error) {
	entries, err := container.ExecList(ctx, fs.execRunner, botID, fs.workDir, memoryDirPath, false)
	if err != nil {
		return nil, fmt.Errorf("list memory dir: %w", err)
	}

	var items []MemoryItem
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		filePath := memoryDirPath + "/" + entry.Path
		content, err := container.ExecRead(ctx, fs.execRunner, botID, fs.workDir, filePath)
		if err != nil {
			fs.logger.Warn("read memory file failed", slog.String("path", filePath), slog.Any("error", err))
			continue
		}
		item, err := parseMemoryMD(content)
		if err != nil {
			fs.logger.Warn("parse memory file failed", slog.String("path", filePath), slog.Any("error", err))
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// ----- internal helpers -----

func (fs *MemoryFS) writeMemoryFile(ctx context.Context, botID string, item MemoryItem) error {
	content := formatMemoryMD(item)
	filePath := fmt.Sprintf("%s/%s.md", memoryDirPath, item.ID)
	return container.ExecWrite(ctx, fs.execRunner, botID, fs.workDir, filePath, content)
}

func (fs *MemoryFS) writeManifest(ctx context.Context, botID string, manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return container.ExecWrite(ctx, fs.execRunner, botID, fs.workDir, manifestPath, string(data))
}

// execDeleteDir removes all files inside a directory (but keeps the directory itself).
func (fs *MemoryFS) execDeleteDir(ctx context.Context, botID, dirPath string) {
	// Use find + rm to avoid shell quoting issues with glob wildcards.
	script := fmt.Sprintf("find %s -type f -delete 2>/dev/null; true", container.ShellQuote(dirPath))
	_, err := fs.execRunner.ExecWithCapture(ctx, mcpgw.ExecRequest{
		BotID:   botID,
		Command: []string{"/bin/sh", "-c", script},
		WorkDir: fs.workDir,
	})
	if err != nil {
		fs.logger.Warn("exec delete dir failed", slog.String("path", dirPath), slog.Any("error", err))
	}
}

// execDeleteFile removes a single file.
func (fs *MemoryFS) execDeleteFile(ctx context.Context, botID, filePath string) {
	script := fmt.Sprintf("rm -f %s", container.ShellQuote(filePath))
	_, err := fs.execRunner.ExecWithCapture(ctx, mcpgw.ExecRequest{
		BotID:   botID,
		Command: []string{"/bin/sh", "-c", script},
		WorkDir: fs.workDir,
	})
	if err != nil {
		fs.logger.Warn("exec delete file failed", slog.String("path", filePath), slog.Any("error", err))
	}
}

// ----- .md formatting / parsing -----

func formatMemoryMD(item MemoryItem) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", item.ID))
	if item.Hash != "" {
		b.WriteString(fmt.Sprintf("hash: %s\n", item.Hash))
	}
	if item.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("created_at: %s\n", item.CreatedAt))
	}
	if item.UpdatedAt != "" {
		b.WriteString(fmt.Sprintf("updated_at: %s\n", item.UpdatedAt))
	}
	b.WriteString("---\n")
	b.WriteString(item.Memory)
	b.WriteString("\n")
	return b.String()
}

func parseMemoryMD(content string) (MemoryItem, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return MemoryItem{}, fmt.Errorf("missing frontmatter")
	}
	// Split on "---" delimiters.
	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 2 {
		return MemoryItem{}, fmt.Errorf("incomplete frontmatter")
	}
	frontmatter := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	item := MemoryItem{Memory: body}
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "id":
			item.ID = value
		case "hash":
			item.Hash = value
		case "created_at":
			item.CreatedAt = value
		case "updated_at":
			item.UpdatedAt = value
		}
	}
	if item.ID == "" {
		return MemoryItem{}, fmt.Errorf("missing id in frontmatter")
	}
	return item, nil
}
