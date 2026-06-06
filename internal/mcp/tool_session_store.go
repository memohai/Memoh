package mcp

import (
	"strings"
	"sync"
)

// ToolSessionContextStore keeps the latest per-prompt context for long-lived
// ACP MCP sessions whose HTTP headers are fixed when the agent process starts.
type ToolSessionContextStore struct {
	mu       sync.RWMutex
	sessions map[string]ToolSessionContext
	sinks    map[string]func(ToolStreamEvent)
}

func NewToolSessionContextStore() *ToolSessionContextStore {
	return &ToolSessionContextStore{
		sessions: map[string]ToolSessionContext{},
		sinks:    map[string]func(ToolStreamEvent){},
	}
}

// ToolStreamEvent is the stream-safe subset of MCP tools/call lifecycle
// events that ACP HTTP MCP calls record for live UI replay and persistence.
type ToolStreamEvent struct {
	Type       string `json:"type"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Input      any    `json:"input,omitempty"`
	Result     any    `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
	// Interactive request fields (Type "user_input_request"). Carrying the
	// pending interaction over the same channel as tool_call_start lets the UI
	// attach it to the existing tool call block instead of rendering a
	// separate synthetic message.
	UserInputID string         `json:"user_input_id,omitempty"`
	ShortID     int            `json:"short_id,omitempty"`
	Status      string         `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (s *ToolSessionContextStore) Put(session ToolSessionContext) {
	if s == nil {
		return
	}
	session.BotID = strings.TrimSpace(session.BotID)
	session.SessionID = strings.TrimSpace(session.SessionID)
	key := toolSessionContextKey(session.BotID, session.SessionID)
	if key == "" {
		return
	}
	s.mu.Lock()
	if existing, ok := s.sessions[key]; ok {
		session = mergeToolSessionContext(existing, session)
	}
	s.sessions[key] = session
	s.mu.Unlock()
}

func (s *ToolSessionContextStore) Merge(session ToolSessionContext) ToolSessionContext {
	if s == nil {
		return session
	}
	key := toolSessionContextKey(session.BotID, session.SessionID)
	if key == "" {
		return session
	}
	s.mu.RLock()
	latest, ok := s.sessions[key]
	s.mu.RUnlock()
	if !ok {
		return session
	}
	return mergeToolSessionContext(session, latest)
}

func (s *ToolSessionContextStore) CloseSession(sessionID string) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	for key := range s.sessions {
		if toolSessionKeyHasSessionID(key, sessionID) {
			delete(s.sessions, key)
		}
	}
	for key := range s.sinks {
		if toolStreamEventKeyHasSessionID(key, sessionID) {
			delete(s.sinks, key)
		}
	}
	s.mu.Unlock()
}

func (s *ToolSessionContextStore) AppendToolEvent(session ToolSessionContext, event ToolStreamEvent) bool {
	if s == nil {
		return false
	}
	key := toolStreamEventKey(session.BotID, session.SessionID, session.StreamID)
	if key == "" {
		return false
	}
	event.Type = strings.TrimSpace(event.Type)
	event.ToolCallID = strings.TrimSpace(event.ToolCallID)
	event.ToolName = strings.TrimSpace(event.ToolName)
	if event.Type == "" || event.ToolCallID == "" || event.ToolName == "" {
		return false
	}
	s.mu.RLock()
	sink := s.sinks[key]
	s.mu.RUnlock()
	if sink != nil {
		sink(event)
		return true
	}
	return false
}

func (s *ToolSessionContextStore) RegisterToolEventSink(session ToolSessionContext, sink func(ToolStreamEvent)) func() {
	if s == nil || sink == nil {
		return func() {}
	}
	key := toolStreamEventKey(session.BotID, session.SessionID, session.StreamID)
	if key == "" {
		return func() {}
	}
	s.mu.Lock()
	s.sinks[key] = sink
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		if current := s.sinks[key]; current != nil {
			delete(s.sinks, key)
		}
		s.mu.Unlock()
	}
}

func toolSessionContextKey(botID, sessionID string) string {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	if botID == "" || sessionID == "" {
		return ""
	}
	return botID + "\x00" + sessionID
}

func toolStreamEventKey(botID, sessionID, streamID string) string {
	sessionKey := toolSessionContextKey(botID, sessionID)
	streamID = strings.TrimSpace(streamID)
	if sessionKey == "" || streamID == "" {
		return ""
	}
	return sessionKey + "\x00" + streamID
}

func toolSessionKeyHasSessionID(key, sessionID string) bool {
	parts := strings.Split(key, "\x00")
	return len(parts) == 2 && parts[1] == sessionID
}

func toolStreamEventKeyHasSessionID(key, sessionID string) bool {
	parts := strings.Split(key, "\x00")
	return len(parts) == 3 && parts[1] == sessionID
}

func mergeToolSessionContext(base, latest ToolSessionContext) ToolSessionContext {
	merged := base
	if value := strings.TrimSpace(latest.BotID); value != "" {
		merged.BotID = value
	}
	if value := strings.TrimSpace(latest.ChatID); value != "" {
		merged.ChatID = value
	}
	if value := strings.TrimSpace(latest.SessionID); value != "" {
		merged.SessionID = value
	}
	if value := strings.TrimSpace(latest.StreamID); value != "" {
		merged.StreamID = value
	}
	if value := strings.TrimSpace(latest.SessionType); value != "" {
		merged.SessionType = value
	}
	if value := strings.TrimSpace(latest.RouteID); value != "" {
		merged.RouteID = value
	}
	if value := strings.TrimSpace(latest.ChannelIdentityID); value != "" {
		merged.ChannelIdentityID = value
	}
	if value := strings.TrimSpace(latest.SessionToken); value != "" {
		merged.SessionToken = value
	}
	if value := strings.TrimSpace(latest.CurrentPlatform); value != "" {
		merged.CurrentPlatform = value
	}
	if value := strings.TrimSpace(latest.ReplyTarget); value != "" {
		merged.ReplyTarget = value
	}
	if value := strings.TrimSpace(latest.ConversationType); value != "" {
		merged.ConversationType = value
	}
	if latest.IsSubagent {
		merged.IsSubagent = true
	}
	return merged
}
