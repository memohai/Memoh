package inbound

import (
	"log/slog"
	"strings"
	"time"
)

func (d *RouteDispatcher) TryAcquireActiveForSession(routeID, sessionID string) (<-chan InjectMessage, bool) {
	return d.tryAcquireActive(routeID, sessionID)
}

func (d *RouteDispatcher) tryAcquireActive(routeID, sessionID string) (<-chan InjectMessage, bool) {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return nil, false
	}
	rs := d.getOrCreate(routeID)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.activeOwners != 0 {
		return nil, false
	}
	rs.activeOwners = 1
	rs.activeSessionID = strings.TrimSpace(sessionID)
	rs.lastUsed = time.Now()
	return rs.injectCh, true
}

func (d *RouteDispatcher) ActiveInjectChannelForSession(routeID, sessionID string) <-chan InjectMessage {
	return d.activeInjectChannel(routeID, sessionID)
}

func (d *RouteDispatcher) activeInjectChannel(routeID, sessionID string) <-chan InjectMessage {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return nil
	}
	rs := d.getOrCreate(routeID)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.activeOwners == 0 || !activeSessionMatches(rs.activeSessionID, sessionID) {
		return nil
	}
	return rs.injectCh
}

func (d *RouteDispatcher) InjectForSession(routeID, sessionID string, msg InjectMessage) InjectResult {
	return d.inject(routeID, sessionID, msg)
}

func (d *RouteDispatcher) inject(routeID, sessionID string, msg InjectMessage) InjectResult {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return InjectUnavailable
	}
	rs := d.getOrCreate(routeID)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.activeOwners == 0 {
		return InjectUnavailable
	}
	if !activeSessionMatches(rs.activeSessionID, sessionID) {
		return InjectSessionMismatch
	}
	eventID := strings.TrimSpace(msg.Source.EventID)
	if eventID != "" {
		if _, exists := rs.injectedMessages[eventID]; exists {
			return InjectDuplicate
		}
	}
	select {
	case rs.injectCh <- msg:
		if eventID != "" {
			if rs.injectedMessages == nil {
				rs.injectedMessages = make(map[string]InjectMessage)
			}
			rs.injectedMessages[eventID] = msg
		}
		if d.logger != nil {
			d.logger.Info("message injected into active stream",
				slog.String("route_id", routeID),
				slog.String("session_id", strings.TrimSpace(sessionID)),
			)
		}
		return InjectAccepted
	default:
		if d.logger != nil {
			d.logger.Warn("inject channel full, message dropped",
				slog.String("route_id", routeID),
				slog.String("session_id", strings.TrimSpace(sessionID)),
			)
		}
		return InjectUnavailable
	}
}

func activeSessionMatches(activeSessionID, requestedSessionID string) bool {
	activeSessionID = strings.TrimSpace(activeSessionID)
	requestedSessionID = strings.TrimSpace(requestedSessionID)
	return activeSessionID == "" || requestedSessionID == "" || activeSessionID == requestedSessionID
}
