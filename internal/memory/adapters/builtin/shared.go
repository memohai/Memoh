package builtin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

func canonicalStoreItem(item storefs.MemoryItem) storefs.MemoryItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Memory = strings.TrimSpace(item.Memory)
	if item.Memory != "" && strings.TrimSpace(item.Hash) == "" {
		item.Hash = runtimeHash(item.Memory)
	}
	return item
}

func runtimePayload(botID string, item storefs.MemoryItem) map[string]string {
	item = canonicalStoreItem(item)
	payload := map[string]string{
		"memory":          item.Memory,
		"bot_id":          strings.TrimSpace(botID),
		"source_entry_id": item.ID,
		"hash":            item.Hash,
	}
	if item.CreatedAt != "" {
		payload["created_at"] = item.CreatedAt
	}
	if item.UpdatedAt != "" {
		payload["updated_at"] = item.UpdatedAt
	}
	for _, key := range []string{"profile_user_id", "profile_channel_identity_id", "profile_display_name", "profile_ref"} {
		if v, ok := item.Metadata[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				payload[key] = strings.TrimSpace(s)
			}
		}
	}
	return payload
}

func payloadMatches(existing, expected map[string]string) bool {
	for key, value := range expected {
		if strings.TrimSpace(existing[key]) != strings.TrimSpace(value) {
			return false
		}
	}
	return true
}

func storeItemFromMemoryItem(item adapters.MemoryItem) storefs.MemoryItem {
	return canonicalStoreItem(storefs.MemoryItem{
		ID:        item.ID,
		Memory:    item.Memory,
		Hash:      item.Hash,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		Score:     item.Score,
		Metadata:  item.Metadata,
		BotID:     item.BotID,
		AgentID:   item.AgentID,
		RunID:     item.RunID,
	})
}

func memoryItemFromStore(item storefs.MemoryItem) adapters.MemoryItem {
	item = canonicalStoreItem(item)
	return adapters.MemoryItem{
		ID:        item.ID,
		Memory:    item.Memory,
		Hash:      item.Hash,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		Score:     item.Score,
		Metadata:  item.Metadata,
		BotID:     item.BotID,
		AgentID:   item.AgentID,
		RunID:     item.RunID,
	}
}

func resultToItem(r qdrantclient.SearchResult) adapters.MemoryItem {
	item := adapters.MemoryItem{
		ID:    r.ID,
		Score: r.Score,
	}
	if r.Payload != nil {
		if sourceID := strings.TrimSpace(r.Payload["source_entry_id"]); sourceID != "" {
			item.ID = sourceID
		}
		item.Memory = r.Payload["memory"]
		item.Hash = r.Payload["hash"]
		item.BotID = r.Payload["bot_id"]
		item.CreatedAt = r.Payload["created_at"]
		item.UpdatedAt = r.Payload["updated_at"]
		meta := map[string]any{}
		for _, key := range []string{"profile_user_id", "profile_channel_identity_id", "profile_display_name", "profile_ref"} {
			if v := strings.TrimSpace(r.Payload[key]); v != "" {
				meta[key] = v
			}
		}
		if len(meta) > 0 {
			item.Metadata = meta
		}
	}
	return item
}

func runtimeBotID(botID string, filters map[string]any) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		botID = strings.TrimSpace(runtimeFilterString(filters, "bot_id"))
	}
	if botID == "" {
		botID = strings.TrimSpace(runtimeFilterString(filters, "scopeId"))
	}
	if botID == "" {
		return "", errors.New("bot_id is required")
	}
	return botID, nil
}

func runtimeBotIDFromMemoryID(memoryID string) string {
	parts := strings.SplitN(strings.TrimSpace(memoryID), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func runtimeText(message string, messages []adapters.Message) string {
	text := strings.TrimSpace(message)
	if text == "" && len(messages) > 0 {
		parts := make([]string, 0, len(messages))
		for _, m := range messages {
			content := strings.TrimSpace(m.Content)
			if content == "" {
				continue
			}
			role := strings.ToUpper(strings.TrimSpace(m.Role))
			if role == "" {
				role = "MESSAGE"
			}
			parts = append(parts, "["+role+"] "+content)
		}
		text = strings.Join(parts, "\n")
	}
	return strings.TrimSpace(text)
}

func runtimeMemoryID(botID string, now time.Time) string {
	return botID + ":" + "mem_" + strconv.FormatInt(now.UnixNano(), 10)
}

func runtimePointID(botID, sourceID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.TrimSpace(botID)+"\n"+strings.TrimSpace(sourceID))).String()
}

func runtimeFilterString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
