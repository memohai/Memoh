package event

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultBufferSize is the default per-subscriber channel buffer.
	DefaultBufferSize = 64
)

// EventType identifies the event category published by the message event hub.
type EventType string

const (
	// EventTypeMessageCreated is emitted after a message is persisted successfully.
	EventTypeMessageCreated EventType = "message_created"
	// EventTypeSessionCreated is emitted after a new user-facing session is
	// created. Consumers use it to surface a fresh session in sidebars before
	// the first message arrives.
	EventTypeSessionCreated EventType = "session_created"
	// EventTypeSessionTitleUpdated is emitted after a session title is auto-generated.
	EventTypeSessionTitleUpdated EventType = "session_title_updated"
	// EventTypeBackgroundTask is emitted for live background exec task updates.
	EventTypeBackgroundTask EventType = "background_task"
	// EventTypeAgentStream is emitted for server-initiated agent stream updates.
	EventTypeAgentStream EventType = "agent_stream"
)

// Event is the normalized payload emitted by the in-process message event hub.
type Event struct {
	Type  EventType       `json:"type"`
	BotID string          `json:"bot_id"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Publisher publishes events to subscribers.
type Publisher interface {
	Publish(event Event)
}

// Subscriber subscribes to bot-scoped events.
type Subscriber interface {
	Subscribe(botID string, buffer int) (string, <-chan Event, func())
}

// dropLogInterval rate-limits the "subscriber buffer full" log so a sustained
// burst doesn't flood the log file. The atomic counter still records the
// exact drop count between log lines.
const dropLogInterval = 5 * time.Second

// Hub is an in-process pub/sub dispatcher for bot-scoped message events.
type Hub struct {
	mu      sync.RWMutex
	streams map[string]map[string]chan Event

	dropped     atomic.Int64
	lastLoggedNS atomic.Int64
}

// NewHub creates an empty message event hub.
func NewHub() *Hub {
	return &Hub{
		streams: map[string]map[string]chan Event{},
	}
}

// Publish broadcasts one event to all subscribers under the same bot ID.
// Slow subscribers are dropped non-blockingly to avoid back-pressure on the
// persistence path; drops are counted and logged at most once per interval.
func (h *Hub) Publish(event Event) {
	if h == nil {
		return
	}
	botID := strings.TrimSpace(event.BotID)
	if botID == "" {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.streams[botID] {
		select {
		case ch <- event:
		default:
			h.recordDrop(botID, event.Type)
		}
	}
}

// recordDrop bumps the drop counter and, when the rate-limit window has
// elapsed, logs the count since the last line.
func (h *Hub) recordDrop(botID string, typ EventType) {
	h.dropped.Add(1)
	now := time.Now().UnixNano()
	last := h.lastLoggedNS.Load()
	if now-last < int64(dropLogInterval) {
		return
	}
	if !h.lastLoggedNS.CompareAndSwap(last, now) {
		return
	}
	dropped := h.dropped.Swap(0)
	slog.Warn("message event hub dropped events on full subscriber buffer",
		slog.Int64("dropped_since_last", dropped),
		slog.String("bot_id", botID),
		slog.String("last_event_type", string(typ)),
	)
}

// Subscribe registers one subscriber under a bot ID.
// It returns a stream ID, read-only event channel, and a cancel function.
func (h *Hub) Subscribe(botID string, buffer int) (string, <-chan Event, func()) {
	if h == nil {
		ch := make(chan Event)
		close(ch)
		return "", ch, func() {}
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		ch := make(chan Event)
		close(ch)
		return "", ch, func() {}
	}
	if buffer <= 0 {
		buffer = DefaultBufferSize
	}

	streamID := uuid.NewString()
	ch := make(chan Event, buffer)

	h.mu.Lock()
	streams, ok := h.streams[botID]
	if !ok {
		streams = map[string]chan Event{}
		h.streams[botID] = streams
	}
	streams[streamID] = ch
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			streams := h.streams[botID]
			if streams != nil {
				if current, ok := streams[streamID]; ok {
					delete(streams, streamID)
					close(current)
				}
				if len(streams) == 0 {
					delete(h.streams, botID)
				}
			}
			h.mu.Unlock()
		})
	}

	return streamID, ch, cancel
}
