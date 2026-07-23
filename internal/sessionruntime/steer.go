package sessionruntime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/conversation"
)

func (m *Manager) Steer(ctx context.Context, botID, sessionID, streamID, text string) (SteerState, error) {
	return m.steer(ctx, botID, sessionID, streamID, "", text)
}

func (m *Manager) SteerRun(ctx context.Context, handle RunHandle, text string) (SteerState, error) {
	handle = handle.normalized()
	if !handle.valid() {
		return SteerState{}, ErrRunOwnershipLost
	}
	return m.steer(ctx, handle.BotID, handle.SessionID, handle.StreamID, handle.Generation, text)
}

func (m *Manager) steer(ctx context.Context, botID, sessionID, streamID, expectedGeneration, text string) (SteerState, error) {
	if m == nil || m.backend == nil {
		return SteerState{}, errors.New("session runtime manager is not configured")
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	expectedGeneration = strings.TrimSpace(expectedGeneration)
	text = strings.TrimSpace(text)
	if botID == "" || sessionID == "" || text == "" {
		return SteerState{}, errors.New("bot_id, session_id, and text are required")
	}
	snapshot, err := m.Snapshot(ctx, botID, sessionID)
	if err != nil {
		return SteerState{}, err
	}
	if snapshot.CurrentRunView == nil {
		return SteerState{}, errors.New("no active runtime run")
	}
	if streamID == "" {
		streamID = strings.TrimSpace(snapshot.CurrentRunView.StreamID)
	}
	if snapshot.CurrentRunView.StreamID != streamID {
		return SteerState{}, errors.New("target runtime run is not active")
	}
	if expectedGeneration != "" && strings.TrimSpace(snapshot.CurrentRunView.Generation) != expectedGeneration {
		return SteerState{}, ErrRunOwnershipLost
	}
	generation := strings.TrimSpace(snapshot.CurrentRunView.Generation)
	if expectedGeneration != "" {
		generation = expectedGeneration
	}
	handle := RunHandle{BotID: botID, SessionID: sessionID, StreamID: streamID, Generation: generation}.normalized()
	var steer SteerState
	var ownerID string
	var commandGeneration string
	var commandCreatedAt time.Time
	_, _, err = m.updateActiveAndPublish(ctx, handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if snapshot.CurrentRunView.StreamID != streamID || !strings.EqualFold(snapshot.CurrentRunView.Status, RunStatusRunning) {
			return snapshot, false, errors.New("target runtime run is not active")
		}
		if snapshot.CurrentRunView.Steer != nil && isPendingSteerStatus(snapshot.CurrentRunView.Steer.Status) {
			return snapshot, false, errors.New("another runtime steer command is still pending")
		}
		steer = SteerState{
			ID:        uuid.NewString(),
			Status:    SteerStatusPending,
			Text:      text,
			CreatedAt: now,
			UpdatedAt: now,
		}
		commandCreatedAt = now
		snapshot.Seq++
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView.Steer = &steer
		snapshot.CurrentRunView.UpdatedAt = now
		ownerID = strings.TrimSpace(snapshot.CurrentRunView.OwnerID)
		commandGeneration = strings.TrimSpace(snapshot.CurrentRunView.Generation)
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, false, false, true, false)
	})
	if err != nil {
		return SteerState{}, err
	}

	cmd := Command{
		Type: CommandSteer, BotID: botID, SessionID: sessionID, StreamID: streamID,
		Generation: commandGeneration, SteerID: steer.ID, Text: text, CreatedAt: commandCreatedAt,
		ExpiresAt: commandCreatedAt.Add(m.commandTimeout()),
	}
	if ctrl := m.localControlForHandle(handle); ctrl != nil {
		m.applyCommand(ctx, cmd)
	} else {
		if ownerID == "" {
			return steer, errors.New("target runtime owner is unknown")
		}
		if m.distributed == nil {
			return steer, errors.New("active runtime is not local")
		}
		if err := m.distributed.PublishCommand(ctx, ownerID, cmd); err != nil {
			m.logger.Warn("publish runtime steer command failed", slog.Any("error", err), slog.String("stream_id", streamID))
			_ = m.updateSteerStatus(context.WithoutCancel(ctx), handle, steer.ID, SteerStatusRejected, RuntimeErrorCodeUnavailable)
			return steer, err
		}
	}
	m.rejectPendingSteerAfterTimeout(context.WithoutCancel(ctx), handle, steer.ID)
	return steer, nil
}

func (m *Manager) applySteerCommand(ctx context.Context, cmd Command) {
	handle := runHandleForCommand(cmd)
	if err := m.ValidateRunOwnership(ctx, handle); err != nil {
		_ = m.updateSteerStatus(context.WithoutCancel(ctx), handle, cmd.SteerID, SteerStatusRejected, RuntimeErrorCodeTargetInactive)
		return
	}
	if !m.steerCommandIsPending(ctx, cmd) {
		return
	}
	ctrl := m.localControlForScope(cmd.BotID, cmd.SessionID, cmd.StreamID)
	if ctrl == nil || ctrl.generation != strings.TrimSpace(cmd.Generation) {
		_ = m.updateSteerStatus(context.WithoutCancel(ctx), handle, cmd.SteerID, SteerStatusRejected, RuntimeErrorCodeTargetInactive)
		return
	}
	errorCode := RuntimeErrorCodeCommandFailed
	if ctrl.injectCh != nil && strings.TrimSpace(cmd.Text) != "" {
		queued, err := m.transitionSteerStatus(ctx, handle, cmd.SteerID, SteerStatusQueued, "")
		if err != nil {
			m.logger.Warn("acknowledge queued steer failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
			return
		}
		if !queued {
			return
		}
		sent, sendError := ctrl.sendInject(ctx, conversation.InjectMessage{
			Text: strings.TrimSpace(cmd.Text),
			Applied: func() {
				if err := m.updateSteerStatus(context.WithoutCancel(ctx), handle, cmd.SteerID, SteerStatusApplied, ""); err != nil {
					m.logger.Warn("acknowledge applied steer failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
				}
			},
		})
		if sent {
			return
		}
		if sendError != "" {
			m.logger.Warn("deliver runtime steer command failed", slog.String("error", sendError), slog.String("stream_id", cmd.StreamID))
		}
	} else {
		errorCode = RuntimeErrorCodeTargetInactive
	}
	if err := m.updateSteerStatus(context.WithoutCancel(ctx), handle, cmd.SteerID, SteerStatusRejected, errorCode); err != nil {
		m.logger.Warn("update steer status failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
	}
}

func (m *Manager) steerCommandIsPending(ctx context.Context, cmd Command) bool {
	if strings.TrimSpace(cmd.SteerID) == "" {
		return false
	}
	snapshot, ok, err := m.backend.Load(ctx, Key{BotID: cmd.BotID, SessionID: cmd.SessionID})
	if err != nil {
		m.logger.Warn("load steer state failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
		return false
	}
	if !ok || !runMatchesHandle(snapshot.CurrentRunView, runHandleForCommand(cmd)) {
		return false
	}
	steer := snapshot.CurrentRunView.Steer
	return steer != nil && steer.ID == strings.TrimSpace(cmd.SteerID) && strings.EqualFold(steer.Status, SteerStatusPending)
}

func (m *Manager) updateSteerStatus(ctx context.Context, handle RunHandle, steerID, status, errorCode string) error {
	_, err := m.transitionSteerStatus(ctx, handle, steerID, status, errorCode)
	return err
}

func (m *Manager) transitionSteerStatus(ctx context.Context, handle RunHandle, steerID, status, errorCode string) (bool, error) {
	_, changed, err := m.updateActiveAndPublish(ctx, handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if !runMatchesHandle(snapshot.CurrentRunView, handle) {
			return snapshot, false, nil
		}
		if snapshot.CurrentRunView.Steer == nil || snapshot.CurrentRunView.Steer.ID != steerID {
			return snapshot, false, nil
		}
		currentStatus := snapshot.CurrentRunView.Steer.Status
		if !validSteerTransition(currentStatus, status) {
			return snapshot, false, nil
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView.UpdatedAt = now
		snapshot.CurrentRunView.Steer.Status = status
		if strings.EqualFold(status, SteerStatusRejected) {
			setRuntimeSteerError(snapshot.CurrentRunView.Steer, errorCode)
		} else {
			clearRuntimeSteerError(snapshot.CurrentRunView.Steer)
		}
		snapshot.CurrentRunView.Steer.UpdatedAt = now
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, false, false, true, false)
	})
	return changed, err
}

func (m *Manager) rejectPendingSteerAfterTimeout(ctx context.Context, handle RunHandle, steerID string) {
	timeout := m.commandAckTTL
	if timeout <= 0 {
		return
	}
	time.AfterFunc(timeout, func() {
		select {
		case <-m.closeCh:
			return
		default:
		}
		err := m.rejectUnacknowledgedSteer(ctx, handle, steerID)
		if err != nil {
			m.logger.Warn("reject pending steer failed", slog.Any("error", err), slog.String("stream_id", handle.StreamID))
		}
	})
}

func (m *Manager) rejectUnacknowledgedSteer(ctx context.Context, handle RunHandle, steerID string) error {
	_, _, err := m.updateActiveAndPublish(ctx, handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if !runMatchesHandle(snapshot.CurrentRunView, handle) {
			return snapshot, false, nil
		}
		steer := snapshot.CurrentRunView.Steer
		if steer == nil || steer.ID != steerID || !strings.EqualFold(steer.Status, SteerStatusPending) {
			return snapshot, false, nil
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView.UpdatedAt = now
		steer.Status = SteerStatusRejected
		setRuntimeSteerError(steer, RuntimeErrorCodeCommandFailed)
		steer.UpdatedAt = now
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, false, false, true, false)
	})
	return err
}

func isPendingSteerStatus(status string) bool {
	return strings.EqualFold(status, SteerStatusPending) || strings.EqualFold(status, SteerStatusQueued)
}

func validSteerTransition(current, next string) bool {
	switch strings.ToLower(strings.TrimSpace(next)) {
	case SteerStatusQueued:
		return strings.EqualFold(current, SteerStatusPending)
	case SteerStatusRejected:
		return isPendingSteerStatus(current)
	case SteerStatusApplied:
		return isPendingSteerStatus(current)
	default:
		return false
	}
}
