package handlers

import (
	"sync"

	"github.com/memohai/memoh/internal/conversation"
)

// activeStreamCache stores intermediate UI messages for streams that are
// still running. This allows the ListMessages endpoint to return in-progress
// content when a user reconnects after closing their browser.
var activeStreamCache struct {
	mu   sync.RWMutex
	data map[string][]conversation.UIMessage // key: "botID:sessionID"
}

func init() {
	activeStreamCache.data = make(map[string][]conversation.UIMessage)
}

func streamCacheKey(botID, sessionID string) string {
	return botID + ":" + sessionID
}

// SetActiveStream stores the current snapshot of UI messages for an active stream.
func SetActiveStream(botID, sessionID string, messages []conversation.UIMessage) {
	key := streamCacheKey(botID, sessionID)
	activeStreamCache.mu.Lock()
	activeStreamCache.data[key] = messages
	activeStreamCache.mu.Unlock()
}

// GetActiveStream retrieves cached UI messages for an active stream.
// Returns nil if no active stream exists.
func GetActiveStream(botID, sessionID string) []conversation.UIMessage {
	key := streamCacheKey(botID, sessionID)
	activeStreamCache.mu.RLock()
	defer activeStreamCache.mu.RUnlock()
	return activeStreamCache.data[key]
}

// ClearActiveStream removes the cached stream data (called when stream completes).
func ClearActiveStream(botID, sessionID string) {
	key := streamCacheKey(botID, sessionID)
	activeStreamCache.mu.Lock()
	delete(activeStreamCache.data, key)
	activeStreamCache.mu.Unlock()
}
