package storefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	fsops "github.com/memohai/memoh/internal/fs"
)

const manifestVersion = 1

const (
	memoryDateLayout = "2006-01-02"
	entryStartPrefix = "<!-- MEMOH:ENTRY "
	entryStartSuffix = " -->"
	entryEndMarker   = "<!-- /MEMOH:ENTRY -->"
)

var ErrNotConfigured = errors.New("memory filesystem not configured")

type Manifest struct {
	Version   int                      `json:"version"`
	UpdatedAt string                   `json:"updated_at"`
	Entries   map[string]ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Hash      string         `json:"hash"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Date      string         `json:"date,omitempty"`
	FilePath  string         `json:"file_path,omitempty"`
	Filters   map[string]any `json:"filters,omitempty"`
}

type Service struct {
	fs *fsops.Service
}

// MemoryItem is the storefs-facing memory record type.
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

func New(fs *fsops.Service) *Service {
	return &Service{fs: fs}
}

func (s *Service) PersistMemories(ctx context.Context, botID string, items []MemoryItem, filters map[string]any) error {
	if s.fs == nil {
		return ErrNotConfigured
	}
	if len(items) == 0 {
		return nil
	}
	manifest, err := s.ReadManifest(ctx, botID)
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
		if current, ok := manifest.Entries[item.ID]; ok && strings.TrimSpace(current.FilePath) != "" && current.FilePath != filePath {
			if toRemoveFromOld[current.FilePath] == nil {
				toRemoveFromOld[current.FilePath] = map[string]struct{}{}
			}
			toRemoveFromOld[current.FilePath][item.ID] = struct{}{}
		}
		if touched[filePath] == nil {
			touched[filePath] = make(map[string]MemoryItem)
		}
		touched[filePath][item.ID] = item
		manifest.Entries[item.ID] = ManifestEntry{
			Hash:      item.Hash,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			Date:      date,
			FilePath:  filePath,
			Filters:   copyFilters(filters),
		}
	}

	for filePath, incoming := range touched {
		existing, readErr := s.readMemoryDay(ctx, botID, filePath)
		if readErr != nil {
			return readErr
		}
		merged := toItemMap(existing)
		for id, item := range incoming {
			merged[id] = item
		}
		if err := s.writeMemoryDay(botID, filePath, mapToItems(merged)); err != nil {
			return err
		}
	}
	if err := s.removeIDsFromFiles(ctx, botID, toRemoveFromOld); err != nil {
		return err
	}
	if err := s.writeManifest(ctx, botID, manifest); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RebuildFiles(ctx context.Context, botID string, items []MemoryItem, filters map[string]any) error {
	if s.fs == nil {
		return ErrNotConfigured
	}
	delErr := s.fs.Delete(botID, memoryDirPath(), true)
	if delErr != nil {
		if fsErr, ok := fsops.AsError(delErr); !ok || fsErr.Code != http.StatusNotFound {
			return delErr
		}
	}
	manifest := &Manifest{
		Version:   manifestVersion,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   make(map[string]ManifestEntry, len(items)),
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
		manifest.Entries[item.ID] = ManifestEntry{
			Hash:      item.Hash,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			Date:      date,
			FilePath:  filePath,
			Filters:   copyFilters(filters),
		}
	}
	for filePath, dayItems := range grouped {
		if err := s.writeMemoryDay(botID, filePath, dayItems); err != nil {
			return err
		}
	}
	if err := s.writeManifest(ctx, botID, manifest); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RemoveMemories(ctx context.Context, botID string, ids []string) error {
	if s.fs == nil {
		return ErrNotConfigured
	}
	if len(ids) == 0 {
		return nil
	}
	manifest, err := s.ReadManifest(ctx, botID)
	if err != nil {
		return err
	}
	removals := make(map[string]map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		entry := manifest.Entries[id]
		targets := make([]string, 0, 2)
		if strings.TrimSpace(entry.FilePath) != "" {
			targets = append(targets, entry.FilePath)
		} else if strings.TrimSpace(entry.Date) != "" {
			targets = append(targets, memoryDayPath(entry.Date))
		}
		targets = append(targets, memoryLegacyItemPath(id))
		for _, target := range targets {
			if removals[target] == nil {
				removals[target] = map[string]struct{}{}
			}
			removals[target][id] = struct{}{}
		}
		delete(manifest.Entries, id)
	}
	if err := s.removeIDsFromFiles(ctx, botID, removals); err != nil {
		return err
	}
	if err := s.writeManifest(ctx, botID, manifest); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) RemoveAllMemories(ctx context.Context, botID string) error {
	if s.fs == nil {
		return ErrNotConfigured
	}
	delErr := s.fs.Delete(botID, memoryDirPath(), true)
	if delErr != nil {
		if fsErr, ok := fsops.AsError(delErr); !ok || fsErr.Code != http.StatusNotFound {
			return delErr
		}
	}
	if err := s.writeManifest(ctx, botID, &Manifest{
		Version:   manifestVersion,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]ManifestEntry{},
	}); err != nil {
		return err
	}
	return s.SyncOverview(ctx, botID)
}

func (s *Service) ReadAllMemoryFiles(ctx context.Context, botID string) ([]MemoryItem, error) {
	if s.fs == nil {
		return nil, ErrNotConfigured
	}
	list, err := s.fs.List(ctx, botID, memoryDirPath())
	if err != nil {
		if fsErr, ok := fsops.AsError(err); ok && fsErr.Code == http.StatusNotFound {
			return []MemoryItem{}, nil
		}
		return nil, err
	}
	items := make([]MemoryItem, 0, len(list.Entries))
	seen := map[string]struct{}{}
	for _, entry := range list.Entries {
		if entry.IsDir || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		content, readErr := s.fs.ReadRaw(ctx, botID, entry.Path)
		if readErr != nil {
			continue
		}
		parsed, parseErr := parseMemoryDayMD(content.Content)
		if parseErr != nil {
			legacy, legacyErr := parseLegacyMemoryMD(content.Content)
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

// SyncOverview rebuilds /data/MEMORY.md from memory day files.
func (s *Service) SyncOverview(ctx context.Context, botID string) error {
	if s.fs == nil {
		return ErrNotConfigured
	}
	items, err := s.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return err
	}
	overview := formatMemoryOverviewMD(items)
	return s.fs.Write(botID, memoryOverviewPath(), overview)
}

func (s *Service) ReadManifest(ctx context.Context, botID string) (*Manifest, error) {
	if s.fs == nil {
		return nil, ErrNotConfigured
	}
	resp, err := s.fs.ReadRaw(ctx, botID, memoryManifestPath())
	if err != nil {
		if fsErr, ok := fsops.AsError(err); ok && fsErr.Code == http.StatusNotFound {
			return &Manifest{
				Version: manifestVersion,
				Entries: map[string]ManifestEntry{},
			}, nil
		}
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(resp.Content), &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Entries == nil {
		manifest.Entries = map[string]ManifestEntry{}
	}
	if manifest.Version == 0 {
		manifest.Version = manifestVersion
	}
	now := time.Now().UTC()
	for id, entry := range manifest.Entries {
		if strings.TrimSpace(entry.Date) == "" {
			entry.Date = memoryDateFromRaw(entry.CreatedAt, now)
		}
		if strings.TrimSpace(entry.FilePath) == "" {
			entry.FilePath = memoryDayPath(entry.Date)
		}
		manifest.Entries[id] = entry
	}
	return &manifest, nil
}

func (s *Service) writeManifest(_ context.Context, botID string, manifest *Manifest) error {
	if manifest == nil {
		manifest = &Manifest{
			Version: manifestVersion,
			Entries: map[string]ManifestEntry{},
		}
	}
	if manifest.Entries == nil {
		manifest.Entries = map[string]ManifestEntry{}
	}
	if manifest.Version == 0 {
		manifest.Version = manifestVersion
	}
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return s.fs.Write(botID, memoryManifestPath(), string(data))
}

func memoryManifestPath() string {
	return path.Join(config.DefaultDataMount, "index", "manifest.json")
}

func memoryOverviewPath() string {
	return path.Join(config.DefaultDataMount, "MEMORY.md")
}

func memoryDirPath() string {
	return path.Join(config.DefaultDataMount, "memory")
}

func memoryDayPath(date string) string {
	return path.Join(memoryDirPath(), strings.TrimSpace(date)+".md")
}

func memoryLegacyItemPath(id string) string {
	return path.Join(memoryDirPath(), strings.TrimSpace(id)+".md")
}

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
		meta := map[string]string{
			"id": item.ID,
		}
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
		return nil, fmt.Errorf("empty memory file")
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
		return nil, fmt.Errorf("no memory entries found")
	}
	return items, nil
}

func parseLegacyMemoryMD(content string) (MemoryItem, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return MemoryItem{}, fmt.Errorf("missing frontmatter")
	}
	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 2 {
		return MemoryItem{}, fmt.Errorf("incomplete frontmatter")
	}
	frontmatter := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])

	item := MemoryItem{Memory: body}
	for _, line := range strings.Split(frontmatter, "\n") {
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
		return MemoryItem{}, fmt.Errorf("missing id in frontmatter")
	}
	return item, nil
}

func (s *Service) readMemoryDay(ctx context.Context, botID, filePath string) ([]MemoryItem, error) {
	resp, err := s.fs.ReadRaw(ctx, botID, filePath)
	if err != nil {
		if fsErr, ok := fsops.AsError(err); ok && fsErr.Code == http.StatusNotFound {
			return []MemoryItem{}, nil
		}
		return nil, err
	}
	items, parseErr := parseMemoryDayMD(resp.Content)
	if parseErr == nil {
		return items, nil
	}
	legacy, legacyErr := parseLegacyMemoryMD(resp.Content)
	if legacyErr != nil {
		return []MemoryItem{}, nil
	}
	return []MemoryItem{legacy}, nil
}

func (s *Service) writeMemoryDay(botID, filePath string, items []MemoryItem) error {
	date := strings.TrimSuffix(path.Base(filePath), ".md")
	return s.fs.Write(botID, filePath, formatMemoryDayMD(date, items))
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
			delErr := s.fs.Delete(botID, filePath, false)
			if delErr != nil {
				if fsErr, ok := fsops.AsError(delErr); !ok || fsErr.Code != http.StatusNotFound {
					return delErr
				}
			}
			continue
		}
		if err := s.writeMemoryDay(botID, filePath, filtered); err != nil {
			return err
		}
	}
	return nil
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

func copyFilters(filters map[string]any) map[string]any {
	if len(filters) == 0 {
		return nil
	}
	out := make(map[string]any, len(filters))
	for k, v := range filters {
		out[k] = v
	}
	return out
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
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		memoryDateLayout,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format(memoryDateLayout)
		}
	}
	if len(raw) >= len(memoryDateLayout) {
		candidate := raw[:len(memoryDateLayout)]
		if t, err := time.Parse(memoryDateLayout, candidate); err == nil {
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
		lines := strings.Split(body, "\n")
		for idx, line := range lines {
			lines[idx] = strings.TrimSpace(line)
		}
		body = strings.Join(lines, " ")
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
