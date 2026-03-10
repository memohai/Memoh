package adapters

import (
	"strings"
)

// TruncateSnippet truncates a string to n runes, appending "..." if truncated.
func TruncateSnippet(s string, n int) string {
	trimmed := strings.TrimSpace(s)
	runes := []rune(trimmed)
	if len(runes) <= n {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:n])) + "..."
}

// DeduplicateItems removes duplicate MemoryItems by ID.
func DeduplicateItems(items []MemoryItem) []MemoryItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]MemoryItem, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Memory)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, item)
	}
	return result
}

// StringFromConfig extracts a trimmed string value from a config map.
func StringFromConfig(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	v, ok := config[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
