package storefs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/mcp/mcpclient"
)

const (
	memoryDateLayout = "2006-01-02"
	entryStartPrefix = "<!-- MEMOH:ENTRY "
	entryStartSuffix = " -->"
	entryEndMarker   = "<!-- /MEMOH:ENTRY -->"
)

var ErrNotConfigured = errors.New("memory filesystem not configured")

// scanEntry maps a memory ID to the file that contains it.
type scanEntry struct {
	FilePath string
}

type Service struct {
	provider mcpclient.Provider
}

type MemoryItem struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	Hash      string         `json:"hash,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Score     float64        `json:"score,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	BotID     string         `json:"bot_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
}

func New(provider mcpclient.Provider) *Service {
	return &Service{provider: provider}
}

func (s *Service) client(ctx context.Context, botID string) (*mcpclient.Client, error) {
	if s.provider == nil {
		return nil, ErrNotConfigured
	}
	return s.provider.MCPClient(ctx, botID)
}

func (s *Service) readFile(ctx context.Context, botID, filePath string) (string, error) {
	c, err := s.client(ctx, botID)
	if err != nil {
		return "", err
	}
	reader, err := c.ReadRaw(ctx, filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) writeFile(ctx context.Context, botID, filePath, content string) error {
	c, err := s.client(ctx, botID)
	if err != nil {
		return err
	}
	return c.WriteFile(ctx, filePath, []byte(content))
}

func (s *Service) deleteFile(ctx context.Context, botID, filePath string, recursive bool) error {
	c, err := s.client(ctx, botID)
	if err != nil {
		return err
	}
	return c.DeleteFile(ctx, filePath, recursive)
}

// buildScanIndex scans all daily memory files and builds a map of id -> file path.
func (s *Service) buildScanIndex(ctx context.Context, botID string) (map[string]scanEntry, error) {
	c, err := s.client(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries, err := c.ListDir(ctx, memoryDirPath(), false)
	if err != nil {
		if isNotFound(err) {
			return map[string]scanEntry{}, nil
		}
		return nil, err
	}
	index := make(map[string]scanEntry)
	for _, entry := range entries {
		if entry.GetIsDir() || !strings.HasSuffix(entry.GetPath(), ".md") {
			continue
		}
		entryPath := path.Join(memoryDirPath(), entry.GetPath())
		content, readErr := s.readFile(ctx, botID, entryPath)
		if readErr != nil {
			continue
		}
		parsed, parseErr := parseMemoryDayMD(content)
		if parseErr != nil {
			legacy, legacyErr := parseLegacyMemoryMD(content)
			if legacyErr != nil {
				continue
			}
			parsed = []MemoryItem{legacy}
		}
		for _, item := range parsed {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if _, ok := index[id]; !ok {
				index[id] = scanEntry{FilePath: entryPath}
			}
		}
	}
	return index, nil
}

func (s *Service) PersistMemories(ctx context.Context, botID string, items []MemoryItem, _ map[string]any) error {
	if s.provider == nil {
		return ErrNotConfigured
	}
	if len(items) == 0 {
		return nil
	}
	index, err := s.buildScanIndex(ctx, botID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	touched := make(map[string]map[string]MemoryItem)
	toRemoveFromOld := make(map[string]map[string]struct{})
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Memory = strings.TrimSpace(item.Memory)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		date := memoryDateForItem(item, now)
		filePath := memoryDayPath(date)
		if current, ok := index[item.ID]; ok && current.FilePath != filePath {
			if toRemoveFromOld[current.FilePath] == nil {
				toRemoveFromOld[current.FilePath] = map[string]struct{}{}
			}
			toRemoveFromOld[current.FilePath][item.ID] = struct{}{}
		}
		if touched[filePath] == nil {
			touched[filePath] = make(map[string]MemoryItem)
		}
		touched[filePath][item.ID] = item
	}

	for filePath, incoming := range touched {
		existing, readErr := s.readMemoryDay(ctx, botID, filePath)
		if readErr != nil {
			return readErr
		}
		merged := toItemMap(existing)
		maps.Copy(merged, incoming)
		if err := s.writeMemoryDay(ctx, botID, filePath, mapToItems(merged)); err != nil {
			return err
		}
	}
	if err := s.removeIDsFromFiles(ctx, botID, toRemoveFromOld); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RebuildFiles(ctx context.Context, botID string, items []MemoryItem, _ map[string]any) error {
	if s.provider == nil {
		return ErrNotConfigured
	}
	if err := s.deleteFile(ctx, botID, memoryDirPath(), true); err != nil && !isNotFound(err) {
		return err
	}
	grouped := make(map[string][]MemoryItem)
	now := time.Now().UTC()
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Memory = strings.TrimSpace(item.Memory)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		date := memoryDateForItem(item, now)
		filePath := memoryDayPath(date)
		grouped[filePath] = append(grouped[filePath], item)
	}
	for filePath, dayItems := range grouped {
		if err := s.writeMemoryDay(ctx, botID, filePath, dayItems); err != nil {
			return err
		}
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RemoveMemories(ctx context.Context, botID string, ids []string) error {
	if s.provider == nil {
		return ErrNotConfigured
	}
	if len(ids) == 0 {
		return nil
	}
	index, err := s.buildScanIndex(ctx, botID)
	if err != nil {
		return err
	}
	removals := make(map[string]map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targets := make([]string, 0, 2)
		if entry, ok := index[id]; ok {
			targets = append(targets, entry.FilePath)
		}
		targets = append(targets, memoryLegacyItemPath(id))
		for _, target := range targets {
			if removals[target] == nil {
				removals[target] = map[string]struct{}{}
			}
			removals[target][id] = struct{}{}
		}
	}
	if err := s.removeIDsFromFiles(ctx, botID, removals); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RemoveAllMemories(ctx context.Context, botID string) error {
	if s.provider == nil {
		return ErrNotConfigured
	}
	if err := s.deleteFile(ctx, botID, memoryDirPath(), true); err != nil && !isNotFound(err) {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) ReadAllMemoryFiles(ctx context.Context, botID string) ([]MemoryItem, error) {
	if s.provider == nil {
		return nil, ErrNotConfigured
	}
	c, err := s.client(ctx, botID)
	if err != nil {
		return nil, err
	}
	entries, err := c.ListDir(ctx, memoryDirPath(), false)
	if err != nil {
		if isNotFound(err) {
			return []MemoryItem{}, nil
		}
		return nil, err
	}
	items := make([]MemoryItem, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.GetIsDir() || !strings.HasSuffix(entry.GetPath(), ".md") {
			continue
		}
		entryPath := path.Join(memoryDirPath(), entry.GetPath())
		content, readErr := s.readFile(ctx, botID, entryPath)
		if readErr != nil {
			continue
		}
		parsed, parseErr := parseMemoryDayMD(content)
		if parseErr != nil {
			legacy, legacyErr := parseLegacyMemoryMD(content)
			if legacyErr != nil {
				continue
			}
			parsed = []MemoryItem{legacy}
		}
		for _, item := range parsed {
			if strings.TrimSpace(item.ID) == "" {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return memoryTime(items[i]).Before(memoryTime(items[j]))
	})
	return items, nil
}

func (s *Service) SyncOverview(ctx context.Context, botID string) error {
	if s.provider == nil {
		return ErrNotConfigured
	}
	items, err := s.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return err
	}
	return s.writeFile(ctx, botID, memoryOverviewPath(), formatMemoryOverviewMD(items))
}

func (s *Service) readMemoryDay(ctx context.Context, botID, filePath string) ([]MemoryItem, error) {
	content, err := s.readFile(ctx, botID, filePath)
	if err != nil {
		if isNotFound(err) {
			return []MemoryItem{}, nil
		}
		return nil, err
	}
	items, parseErr := parseMemoryDayMD(content)
	if parseErr == nil {
		return items, nil
	}
	legacy, legacyErr := parseLegacyMemoryMD(content)
	if legacyErr != nil {
		return []MemoryItem{}, nil
	}
	return []MemoryItem{legacy}, nil
}

func (s *Service) writeMemoryDay(ctx context.Context, botID, filePath string, items []MemoryItem) error {
	date := strings.TrimSuffix(path.Base(filePath), ".md")
	return s.writeFile(ctx, botID, filePath, formatMemoryDayMD(date, items))
}

func (s *Service) removeIDsFromFiles(ctx context.Context, botID string, removals map[string]map[string]struct{}) error {
	for filePath, ids := range removals {
		if len(ids) == 0 {
			continue
		}
		items, err := s.readMemoryDay(ctx, botID, filePath)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			continue
		}
		filtered := make([]MemoryItem, 0, len(items))
		for _, item := range items {
			if _, remove := ids[item.ID]; remove {
				continue
			}
			filtered = append(filtered, item)
		}
		if len(filtered) == 0 {
			if err := s.deleteFile(ctx, botID, filePath, false); err != nil && !isNotFound(err) {
				return err
			}
			continue
		}
		if err := s.writeMemoryDay(ctx, botID, filePath, filtered); err != nil {
			return err
		}
	}
	return nil
}

// --- path helpers ---

func memoryOverviewPath() string { return path.Join(config.DefaultDataMount, "MEMORY.md") }
func memoryDirPath() string      { return path.Join(config.DefaultDataMount, "memory") }
func memoryDayPath(date string) string {
	return path.Join(memoryDirPath(), strings.TrimSpace(date)+".md")
}

func memoryLegacyItemPath(id string) string {
	return path.Join(memoryDirPath(), strings.TrimSpace(id)+".md")
}

// --- format / parse helpers ---

func formatMemoryDayMD(date string, items []MemoryItem) string {
	var b strings.Builder
	b.WriteString("# Memory ")
	b.WriteString(date)
	b.WriteString("\n\n")
	sort.Slice(items, func(i, j int) bool {
		ti, tj := memoryTime(items[i]), memoryTime(items[j])
		if ti.Equal(tj) {
			return items[i].ID < items[j].ID
		}
		return ti.Before(tj)
	})
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Memory = strings.TrimSpace(item.Memory)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		meta := map[string]string{"id": item.ID}
		if item.Hash != "" {
			meta["hash"] = item.Hash
		}
		if item.CreatedAt != "" {
			meta["created_at"] = item.CreatedAt
		}
		if item.UpdatedAt != "" {
			meta["updated_at"] = item.UpdatedAt
		}
		rawMeta, _ := json.Marshal(meta)
		b.WriteString(entryStartPrefix)
		b.Write(rawMeta)
		b.WriteString(entryStartSuffix)
		b.WriteString("\n")
		b.WriteString(item.Memory)
		b.WriteString("\n")
		b.WriteString(entryEndMarker)
		b.WriteString("\n\n")
	}
	return b.String()
}

func parseMemoryDayMD(content string) ([]MemoryItem, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("empty memory file")
	}
	lines := strings.Split(content, "\n")
	items := make([]MemoryItem, 0, 8)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, entryStartPrefix) || !strings.HasSuffix(line, entryStartSuffix) {
			continue
		}
		metaJSON := strings.TrimSuffix(strings.TrimPrefix(line, entryStartPrefix), entryStartSuffix)
		var meta map[string]string
		if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
			continue
		}
		start := i + 1
		end := start
		for ; end < len(lines); end++ {
			if strings.TrimSpace(lines[end]) == entryEndMarker {
				break
			}
		}
		if end >= len(lines) {
			break
		}
		item := MemoryItem{
			ID:        strings.TrimSpace(meta["id"]),
			Hash:      strings.TrimSpace(meta["hash"]),
			CreatedAt: strings.TrimSpace(meta["created_at"]),
			UpdatedAt: strings.TrimSpace(meta["updated_at"]),
			Memory:    strings.TrimSpace(strings.Join(lines[start:end], "\n")),
		}
		if item.ID != "" && item.Memory != "" {
			items = append(items, item)
		}
		i = end
	}
	if len(items) == 0 {
		return nil, errors.New("no memory entries found")
	}
	return items, nil
}

func parseLegacyMemoryMD(content string) (MemoryItem, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return MemoryItem{}, errors.New("missing frontmatter")
	}
	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 2 {
		return MemoryItem{}, errors.New("incomplete frontmatter")
	}
	item := MemoryItem{Memory: strings.TrimSpace(parts[1])}
	for _, line := range strings.Split(strings.TrimSpace(parts[0]), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "id":
			item.ID = strings.TrimSpace(value)
		case "hash":
			item.Hash = strings.TrimSpace(value)
		case "created_at":
			item.CreatedAt = strings.TrimSpace(value)
		case "updated_at":
			item.UpdatedAt = strings.TrimSpace(value)
		}
	}
	if item.ID == "" {
		return MemoryItem{}, errors.New("missing id in frontmatter")
	}
	return item, nil
}

func formatMemoryOverviewMD(items []MemoryItem) string {
	var b strings.Builder
	b.WriteString("# MEMORY\n\n")
	if len(items) == 0 {
		b.WriteString("> No memory entries yet.\n")
		return b.String()
	}
	ordered := append([]MemoryItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool {
		ti, tj := memoryTime(ordered[i]), memoryTime(ordered[j])
		if ti.Equal(tj) {
			return ordered[i].ID > ordered[j].ID
		}
		return ti.After(tj)
	})
	for i, item := range ordered {
		if i >= 500 {
			break
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = "unknown"
		}
		created := strings.TrimSpace(item.CreatedAt)
		if created == "" {
			created = "unknown"
		}
		body := strings.TrimSpace(item.Memory)
		if body == "" {
			continue
		}
		body = strings.Join(strings.Fields(body), " ")
		if len(body) > 400 {
			body = strings.TrimSpace(body[:400]) + "..."
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". [")
		b.WriteString(created)
		b.WriteString("] (")
		b.WriteString(id)
		b.WriteString(") ")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// --- utility helpers ---

func isNotFound(err error) bool {
	return errors.Is(err, mcpclient.ErrNotFound)
}

func toItemMap(items []MemoryItem) map[string]MemoryItem {
	m := make(map[string]MemoryItem, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			m[id] = item
		}
	}
	return m
}

func mapToItems(m map[string]MemoryItem) []MemoryItem {
	items := make([]MemoryItem, 0, len(m))
	for _, item := range m {
		items = append(items, item)
	}
	return items
}

func memoryDateForItem(item MemoryItem, now time.Time) string {
	if d := memoryDateFromRaw(item.CreatedAt, now); d != "" {
		return d
	}
	if d := memoryDateFromRaw(item.UpdatedAt, now); d != "" {
		return d
	}
	return now.Format(memoryDateLayout)
}

func memoryDateFromRaw(raw string, now time.Time) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return now.Format(memoryDateLayout)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", memoryDateLayout} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format(memoryDateLayout)
		}
	}
	if len(raw) >= len(memoryDateLayout) {
		if t, err := time.Parse(memoryDateLayout, raw[:len(memoryDateLayout)]); err == nil {
			return t.UTC().Format(memoryDateLayout)
		}
	}
	return now.Format(memoryDateLayout)
}

func memoryTime(item MemoryItem) time.Time {
	parse := func(v string) (time.Time, bool) {
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, false
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UTC(), true
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.UTC(), true
		}
		return time.Time{}, false
	}
	if t, ok := parse(item.CreatedAt); ok {
		return t
	}
	if t, ok := parse(item.UpdatedAt); ok {
		return t
	}
	return time.Time{}
}
