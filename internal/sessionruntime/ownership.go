package sessionruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (m *Manager) StreamRef(ctx context.Context, botID, sessionID, streamID string) (StreamRef, bool, error) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	if m == nil || m.backend == nil || botID == "" || sessionID == "" || streamID == "" {
		return StreamRef{}, false, nil
	}
	if ctrl := m.localControlForScope(botID, sessionID, streamID); ctrl != nil {
		ownerID := ""
		if m.distributed != nil {
			ownerID = m.ownerID
		}
		return StreamRef{BotID: ctrl.botID, SessionID: ctrl.sessionID, StreamID: ctrl.streamID, OwnerID: ownerID, Generation: ctrl.generation}, true, nil
	}
	if m.distributed == nil {
		return StreamRef{}, false, nil
	}
	return m.distributed.LoadStreamRef(ctx, Key{BotID: botID, SessionID: sessionID}, streamID)
}

func runHandleForCommand(cmd Command) RunHandle {
	return RunHandle{BotID: cmd.BotID, SessionID: cmd.SessionID, StreamID: cmd.StreamID, Generation: cmd.Generation}.normalized()
}

// ValidateRunOwnership fails closed before durable side effects when this
// process no longer owns the active runtime run.
func (m *Manager) ValidateRunOwnership(ctx context.Context, handle RunHandle) error {
	if m == nil || m.backend == nil {
		return errors.New("session runtime manager is not configured")
	}
	handle = handle.normalized()
	if !handle.valid() {
		return ErrRunOwnershipLost
	}
	ctrl := m.localControlForHandle(handle)
	if ctrl == nil {
		return ErrRunOwnershipLost
	}
	key := handle.key()
	if m.distributed == nil {
		snapshot, ok, err := m.backend.Load(ctx, key)
		if err != nil {
			return fmt.Errorf("validate runtime ownership: %w", err)
		}
		if !ok || !runMatchesHandle(snapshot.CurrentRunView, handle) || !m.runOwnerMatches(snapshot.CurrentRunView) || !isActiveRunStatus(snapshot.CurrentRunView.Status) {
			return ErrRunOwnershipLost
		}
		return nil
	}
	if !ctrl.leaseIsValidAt(time.Now()) {
		return ErrRunOwnershipLost
	}
	ref := StreamRef{BotID: handle.BotID, SessionID: handle.SessionID, StreamID: handle.StreamID, OwnerID: m.ownerID, Generation: handle.Generation}
	if err := m.distributed.ValidateRunOwnership(ctx, key, ref); err != nil {
		if errors.Is(err, ErrRunOwnershipLost) {
			return ErrRunOwnershipLost
		}
		return fmt.Errorf("validate runtime ownership: %w", err)
	}
	// A Redis round trip can consume the final part of the conservative local
	// lease window. Recheck after the atomic server-side decision before any
	// durable side effect begins.
	if !ctrl.leaseIsValidAt(time.Now()) || m.localControlForHandle(handle) != ctrl {
		return ErrRunOwnershipLost
	}
	return nil
}
