package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
)

const (
	memoryDateLayout      = "2006-01-02"
	memEntryStartPrefix   = "<!-- MEMOH:ENTRY "
	memEntryStartSuffix   = " -->"
	memEntryEndMarker     = "<!-- /MEMOH:ENTRY -->"
	memFileHeaderTemplate = "# Memory %s\n\n"
)

type writeRecord struct {
	Topic     string `json:"topic"`
	ID        string `json:"id"`
	Memory    string `json:"memory"`
	Text      string `json:"text"`
	Content   string `json:"content"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// NormalizeMemoryDayContent converts user/LLM writes to canonical memory day format.
// Non-memory-day paths are returned unchanged.
func NormalizeMemoryDayContent(containerPath, raw string) string {
	if !isMemoryDayMarkdownPath(containerPath) {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	if strings.Contains(trimmed, memEntryStartPrefix) && strings.Contains(trimmed, memEntryEndMarker) {
		return raw
	}
	date := strings.TrimSuffix(path.Base(containerPath), ".md")
	records := parseStructuredRecords(trimmed)
	if len(records) == 0 {
		records = []writeRecord{buildFallbackRecord(trimmed, date, time.Now().UTC())}
	}
	return formatDayMarkdown(date, records)
}

// RenderMemoryDayForDisplay converts canonical memory day markdown into
// a user-facing timeline view. Non-memory-day paths are returned unchanged.
func RenderMemoryDayForDisplay(containerPath, raw string) string {
	if !isMemoryDayMarkdownPath(containerPath) {
		return raw
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	date := strings.TrimSuffix(path.Base(containerPath), ".md")
	records := parseCanonicalDayRecords(trimmed)
	if len(records) == 0 {
		return raw
	}
	sort.Slice(records, func(i, j int) bool {
		ti := recordTime(records[i])
		tj := recordTime(records[j])
		if ti.Equal(tj) {
			return records[i].ID < records[j].ID
		}
		return ti.Before(tj)
	})
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(date)
	b.WriteString("\n\n")
	for idx, r := range records {
		if idx > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## ")
		b.WriteString(formatRecordTime(r))
		b.WriteString(" - ")
		b.WriteString(recordTitle(r))
		b.WriteString("\n")
		b.WriteString(formatRecordBody(r.Memory))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func isMemoryDayMarkdownPath(containerPath string) bool {
	clean := path.Clean("/" + strings.TrimSpace(containerPath))
	memoryDir := path.Clean(config.DefaultDataMount+"/memory") + "/"
	if !strings.HasPrefix(clean, memoryDir) || !strings.HasSuffix(clean, ".md") {
		return false
	}
	datePart := strings.TrimSuffix(path.Base(clean), ".md")
	_, err := time.Parse(memoryDateLayout, datePart)
	return err == nil
}

func parseStructuredRecords(content string) []writeRecord {
	now := time.Now().UTC()
	normalize := func(in []writeRecord) []writeRecord {
		out := make([]writeRecord, 0, len(in))
		for _, r := range in {
			nr, ok := normalizeRecord(r, now)
			if ok {
				out = append(out, nr)
			}
		}
		return out
	}

	var list []writeRecord
	if err := json.Unmarshal([]byte(content), &list); err == nil {
		return normalize(list)
	}
	var obj writeRecord
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		return normalize([]writeRecord{obj})
	}
	var wrapped struct {
		Items []writeRecord `json:"items"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil {
		return normalize(wrapped.Items)
	}
	return nil
}

func parseCanonicalDayRecords(content string) []writeRecord {
	lines := strings.Split(content, "\n")
	out := make([]writeRecord, 0, 8)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, memEntryStartPrefix) || !strings.HasSuffix(line, memEntryStartSuffix) {
			continue
		}
		metaJSON := strings.TrimSuffix(strings.TrimPrefix(line, memEntryStartPrefix), memEntryStartSuffix)
		var rec writeRecord
		if err := json.Unmarshal([]byte(metaJSON), &rec); err != nil {
			continue
		}
		start := i + 1
		end := start
		for ; end < len(lines); end++ {
			if strings.TrimSpace(lines[end]) == memEntryEndMarker {
				break
			}
		}
		if end >= len(lines) {
			break
		}
		rec.Memory = strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		out = append(out, rec)
		i = end
	}
	return out
}

func formatDayMarkdown(date string, records []writeRecord) string {
	sort.Slice(records, func(i, j int) bool {
		ti := parseRFC3339OrZero(records[i].CreatedAt)
		tj := parseRFC3339OrZero(records[j].CreatedAt)
		if ti.Equal(tj) {
			return records[i].ID < records[j].ID
		}
		return ti.Before(tj)
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf(memFileHeaderTemplate, date))
	for _, r := range records {
		meta := map[string]string{"id": r.ID}
		if r.Topic != "" {
			meta["topic"] = r.Topic
		}
		if r.Hash != "" {
			meta["hash"] = r.Hash
		}
		if r.CreatedAt != "" {
			meta["created_at"] = r.CreatedAt
		}
		if r.UpdatedAt != "" {
			meta["updated_at"] = r.UpdatedAt
		}
		rawMeta, _ := json.Marshal(meta)
		b.WriteString(memEntryStartPrefix)
		b.Write(rawMeta)
		b.WriteString(memEntryStartSuffix)
		b.WriteString("\n")
		b.WriteString(r.Memory)
		b.WriteString("\n")
		b.WriteString(memEntryEndMarker)
		b.WriteString("\n\n")
	}
	return b.String()
}

func parseRFC3339OrZero(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func recordTime(r writeRecord) time.Time {
	if t := parseRFC3339OrZero(r.CreatedAt); !t.IsZero() {
		return t
	}
	if t := parseRFC3339OrZero(r.UpdatedAt); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func formatRecordTime(r writeRecord) string {
	t := recordTime(r)
	if t.IsZero() {
		return "--:--"
	}
	return t.Format("03:04 PM")
}

func recordTitle(r writeRecord) string {
	if topic := strings.TrimSpace(r.Topic); topic != "" {
		return topic
	}
	return "Notes"
}

func formatRecordBody(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "1. ") {
			out = append(out, line)
			continue
		}
		out = append(out, "- "+line)
	}
	if len(out) == 0 {
		return "- (empty)"
	}
	return strings.Join(out, "\n")
}

func buildFallbackRecord(content, date string, now time.Time) writeRecord {
	record := writeRecord{
		ID:        fmt.Sprintf("mem_%d", now.UnixNano()),
		Memory:    sanitizeFallbackBody(content, date),
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}
	if legacy, ok := parseLegacyFrontmatterRecord(content); ok {
		if normalized, ok := normalizeRecord(legacy, now); ok {
			return normalized
		}
	}
	if record.Hash == "" {
		record.Hash = generateMemoryHash(record.Topic, record.Memory)
	}
	return record
}

func parseLegacyFrontmatterRecord(content string) (writeRecord, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return writeRecord{}, false
	}
	parts := strings.SplitN(trimmed[3:], "---", 2)
	if len(parts) < 2 {
		return writeRecord{}, false
	}
	frontmatter := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])
	record := writeRecord{Memory: body}
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "id":
			record.ID = value
		case "hash":
			record.Hash = value
		case "created_at":
			record.CreatedAt = value
		case "updated_at":
			record.UpdatedAt = value
		}
	}
	return record, true
}

func sanitizeFallbackBody(content, date string) string {
	body := strings.TrimSpace(content)
	header := "# Memory " + strings.TrimSpace(date)
	if strings.HasPrefix(body, header) {
		body = strings.TrimSpace(strings.TrimPrefix(body, header))
	}
	return body
}

func normalizeRecord(r writeRecord, now time.Time) (writeRecord, bool) {
	mem := strings.TrimSpace(r.Memory)
	if mem == "" {
		mem = strings.TrimSpace(r.Content)
	}
	if mem == "" {
		mem = strings.TrimSpace(r.Text)
	}
	if mem == "" {
		return writeRecord{}, false
	}
	topic := strings.TrimSpace(r.Topic)
	id := strings.TrimSpace(r.ID)
	if id == "" {
		id = fmt.Sprintf("mem_%d", now.UnixNano())
	}
	createdAt := strings.TrimSpace(r.CreatedAt)
	if createdAt == "" {
		createdAt = now.Format(time.RFC3339)
	}
	updatedAt := strings.TrimSpace(r.UpdatedAt)
	if updatedAt == "" {
		updatedAt = createdAt
	}
	hash := strings.TrimSpace(r.Hash)
	if hash == "" {
		hash = generateMemoryHash(topic, mem)
	}
	return writeRecord{
		Topic:     topic,
		ID:        id,
		Memory:    mem,
		Hash:      hash,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, true
}

func generateMemoryHash(topic, memory string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(topic) + "\n" + strings.TrimSpace(memory)))
	return hex.EncodeToString(sum[:])
}
