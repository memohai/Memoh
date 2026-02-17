// Package event provides in-memory event hubs for message delivery.
package event

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	// DefaultBufferSize is the default per-subscriber channel buffer.
	DefaultBufferSize = 64
)

// Type identifies the event category published by the message event hub.
type Type string

const (
	// TypeMessageCreated is emitted after a message is persisted successfully.
	TypeMessageCreated Type = "message_created"
)

// Event is the normalized payload emitted by the in-process message event hub.
type Event struct {
	Type  Type            `json:"type"`
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

// Hub is an in-process pub/sub dispatcher for bot-scoped message events.
type Hub struct {
	mu      sync.RWMutex
	streams map[string]map[string]chan Event
}

// NewHub creates an empty message event hub.
func NewHub() *Hub {
	return &Hub{
		streams: map[string]map[string]chan Event{},
	}
}

// Publish broadcasts one event to all subscribers under the same bot ID.
// Slow subscribers are dropped in a non-blocking way.
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
			// Drop if receiver is slow to avoid blocking persistence path.
		}
	}
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
