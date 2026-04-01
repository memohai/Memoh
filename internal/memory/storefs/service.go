package storefs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	memoryDateLayout   = "2006-01-02"
	entryHeadingPrefix = "## Entry "
	yamlFence          = "```yaml"
	codeFence          = "```"
)

var ErrNotConfigured = errors.New("memory filesystem not configured")

// scanEntry maps a memory ID to the file that contains it.
type scanEntry struct {
	FilePath string
}

type Service struct {
	provider bridge.Provider
	logger   *slog.Logger
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

type memoryEntryMeta struct {
	ID        string         `yaml:"id"`
	Hash      string         `yaml:"hash,omitempty"`
	CreatedAt string         `yaml:"created_at,omitempty"`
	UpdatedAt string         `yaml:"updated_at,omitempty"`
	Metadata  map[string]any `yaml:"metadata,omitempty"`
}

func New(log *slog.Logger, provider bridge.Provider) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{provider: provider, logger: log.With(slog.String("component", "storefs"))}
}

func (s *Service) client(ctx context.Context, botID string) (*bridge.Client, error) {
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
	entries, err := c.ListDirAll(ctx, memoryDirPath(), false)
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
			s.logger.Warn("buildScanIndex: failed to read memory file",
				slog.String("bot_id", botID), slog.String("path", entryPath), slog.Any("error", readErr))
			continue
		}
		parsed, parseErr := parseMemoryDayMD(content)
		if parseErr != nil {
			jsonItems, jsonErr := parseJSONMemoryItems(content)
			if jsonErr != nil {
				s.logger.Warn("buildScanIndex: failed to parse memory file",
					slog.String("bot_id", botID), slog.String("path", entryPath), slog.Any("error", parseErr))
				continue
			}
			if err := s.writeMemoryDay(ctx, botID, entryPath, jsonItems); err != nil {
				return nil, err
			}
			parsed = jsonItems
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
		targets := make([]string, 0, 1)
		if entry, ok := index[id]; ok {
			targets = append(targets, entry.FilePath)
		}
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
	entries, err := c.ListDirAll(ctx, memoryDirPath(), false)
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
			jsonItems, jsonErr := parseJSONMemoryItems(content)
			if jsonErr != nil {
				continue
			}
			if err := s.writeMemoryDay(ctx, botID, entryPath, jsonItems); err != nil {
				return nil, err
			}
			parsed = jsonItems
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

func (s *Service) CountMemoryFiles(ctx context.Context, botID string) (int, error) {
	if s.provider == nil {
		return 0, ErrNotConfigured
	}
	c, err := s.client(ctx, botID)
	if err != nil {
		return 0, err
	}
	entries, err := c.ListDirAll(ctx, memoryDirPath(), false)
	if err != nil {
		if isNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.GetIsDir() || !strings.HasSuffix(entry.GetPath(), ".md") {
			continue
		}
		count++
	}
	return count, nil
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
	jsonItems, jsonErr := parseJSONMemoryItems(content)
	if jsonErr != nil {
		return []MemoryItem{}, nil
	}
	if err := s.writeMemoryDay(ctx, botID, filePath, jsonItems); err != nil {
		return nil, err
	}
	return jsonItems, nil
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
		meta := memoryEntryMeta{
			ID:        item.ID,
			Hash:      item.Hash,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			Metadata:  item.Metadata,
		}
		rawMeta, _ := yaml.Marshal(meta)
		b.WriteString(entryHeadingPrefix)
		b.WriteString(item.ID)
		b.WriteString("\n\n")
		b.WriteString(yamlFence)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(string(rawMeta)))
		b.WriteString("\n")
		b.WriteString(codeFence)
		b.WriteString("\n\n")
		b.WriteString(item.Memory)
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
		if !strings.HasPrefix(line, entryHeadingPrefix) {
			continue
		}
		entryID := strings.TrimSpace(strings.TrimPrefix(line, entryHeadingPrefix))
		j := i + 1
		for ; j < len(lines) && strings.TrimSpace(lines[j]) == ""; j++ {
		}
		if j >= len(lines) || strings.TrimSpace(lines[j]) != yamlFence {
			continue
		}
		metaStart := j + 1
		metaEnd := metaStart
		for ; metaEnd < len(lines); metaEnd++ {
			if strings.TrimSpace(lines[metaEnd]) == codeFence {
				break
			}
		}
		if metaEnd >= len(lines) {
			break
		}
		var meta memoryEntryMeta
		if err := yaml.Unmarshal([]byte(strings.Join(lines[metaStart:metaEnd], "\n")), &meta); err != nil {
			continue
		}
		bodyStart := metaEnd + 1
		if bodyStart < len(lines) && strings.TrimSpace(lines[bodyStart]) == "" {
			bodyStart++
		}
		bodyEnd := bodyStart
		for ; bodyEnd < len(lines); bodyEnd++ {
			if strings.HasPrefix(strings.TrimSpace(lines[bodyEnd]), entryHeadingPrefix) {
				break
			}
		}
		item := MemoryItem{
			ID:        firstNonEmpty(meta.ID, entryID),
			Hash:      strings.TrimSpace(meta.Hash),
			CreatedAt: strings.TrimSpace(meta.CreatedAt),
			UpdatedAt: strings.TrimSpace(meta.UpdatedAt),
			Metadata:  meta.Metadata,
			Memory:    strings.TrimSpace(strings.Join(lines[bodyStart:bodyEnd], "\n")),
		}
		if item.ID != "" && item.Memory != "" {
			items = append(items, item)
		}
		i = bodyEnd - 1
	}
	if len(items) == 0 {
		return nil, errors.New("no memory entries found")
	}
	return items, nil
}

type jsonMemoryRecord struct {
	Topic     string `json:"topic"`
	ID        string `json:"id"`
	Memory    string `json:"memory"`
	Text      string `json:"text"`
	Content   string `json:"content"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func parseJSONMemoryItems(content string) ([]MemoryItem, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("empty memory file")
	}
	var list []jsonMemoryRecord
	if err := json.Unmarshal([]byte(content), &list); err == nil {
		return normalizeJSONMemoryItems(list), nil
	}
	var obj jsonMemoryRecord
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		return normalizeJSONMemoryItems([]jsonMemoryRecord{obj}), nil
	}
	var wrapped struct {
		Items []jsonMemoryRecord `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil {
		return normalizeJSONMemoryItems(wrapped.Items), nil
	}
	return nil, errors.New("not json memory format")
}

func normalizeJSONMemoryItems(records []jsonMemoryRecord) []MemoryItem {
	now := time.Now().UTC()
	items := make([]MemoryItem, 0, len(records))
	for _, record := range records {
		text := strings.TrimSpace(record.Memory)
		if text == "" {
			text = strings.TrimSpace(record.Content)
		}
		if text == "" {
			text = strings.TrimSpace(record.Text)
		}
		if text == "" {
			continue
		}
		item := MemoryItem{
			ID:        strings.TrimSpace(record.ID),
			Hash:      strings.TrimSpace(record.Hash),
			CreatedAt: strings.TrimSpace(record.CreatedAt),
			UpdatedAt: strings.TrimSpace(record.UpdatedAt),
			Memory:    text,
		}
		if item.ID == "" {
			item.ID = "mem_" + strconv.FormatInt(now.UnixNano(), 10)
		}
		if item.CreatedAt == "" {
			item.CreatedAt = now.Format(time.RFC3339)
		}
		if item.UpdatedAt == "" {
			item.UpdatedAt = item.CreatedAt
		}
		if item.Hash == "" {
			item.Hash = "json_" + strconv.FormatInt(now.UnixNano(), 10)
		}
		if topic := strings.TrimSpace(record.Topic); topic != "" {
			item.Metadata = map[string]any{"topic": topic}
		}
		items = append(items, item)
	}
	return items
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
	return errors.Is(err, bridge.ErrNotFound)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
