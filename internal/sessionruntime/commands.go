package sessionruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/conversation"
)

func (m *Manager) StreamRef(ctx context.Context, streamID string) (StreamRef, bool, error) {
	streamID = strings.TrimSpace(streamID)
	if m == nil || m.backend == nil || streamID == "" {
		return StreamRef{}, false, nil
	}
	if ctrl := m.localControl(streamID); ctrl != nil {
		return StreamRef{BotID: ctrl.botID, SessionID: ctrl.sessionID, StreamID: ctrl.streamID}, true, nil
	}
	if m.distributed == nil {
		return StreamRef{}, false, nil
	}
	return m.distributed.LoadStreamRef(ctx, streamID)
}

// ValidateRunOwnership fails closed before durable side effects when this
// process no longer owns the active runtime run.
func (m *Manager) ValidateRunOwnership(ctx context.Context, botID, sessionID, streamID string) error {
	if m == nil || m.backend == nil {
		return errors.New("session runtime manager is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	ctrl := m.localControl(streamID)
	if ctrl == nil || ctrl.botID != strings.TrimSpace(botID) || ctrl.sessionID != strings.TrimSpace(sessionID) {
		return ErrRunOwnershipLost
	}
	key := Key{BotID: strings.TrimSpace(botID), SessionID: strings.TrimSpace(sessionID)}
	if m.distributed == nil {
		snapshot, ok, err := m.backend.Load(ctx, key)
		if err != nil {
			return fmt.Errorf("validate runtime ownership: %w", err)
		}
		if !ok || snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != streamID || !m.runOwnerMatches(snapshot.CurrentRunView) || !isEventAcceptingRunStatus(snapshot.CurrentRunView.Status) {
			return ErrRunOwnershipLost
		}
		return nil
	}
	if !ctrl.leaseIsValidAt(time.Now()) {
		return ErrRunOwnershipLost
	}
	now, err := m.backend.Now(ctx)
	if err != nil {
		return fmt.Errorf("validate runtime ownership time: %w", err)
	}
	ref, refOK, err := m.distributed.LoadStreamRef(ctx, streamID)
	if err != nil {
		return fmt.Errorf("validate runtime stream lease: %w", err)
	}
	if !refOK || ref.BotID != key.BotID || ref.SessionID != key.SessionID || ref.StreamID != streamID || ref.OwnerID != m.ownerID {
		return ErrRunOwnershipLost
	}
	snapshot, ok, err := m.backend.Load(ctx, key)
	if err != nil {
		return fmt.Errorf("validate runtime ownership: %w", err)
	}
	if !ok || snapshot.CurrentRunView == nil {
		return ErrRunOwnershipLost
	}
	run := snapshot.CurrentRunView
	if run.StreamID != streamID || !m.runOwnerMatches(run) || !isEventAcceptingRunStatus(run.Status) || m.leaseExpired(run, now) {
		return ErrRunOwnershipLost
	}
	return nil
}

func (m *Manager) Abort(ctx context.Context, streamID string) (bool, error) {
	streamID = strings.TrimSpace(streamID)
	if m == nil || m.backend == nil || streamID == "" {
		return false, nil
	}
	if ctrl := m.localControl(streamID); ctrl != nil {
		return m.abortLocal(ctx, ctrl)
	}
	if m.distributed == nil {
		return false, nil
	}
	ref, ok, err := m.distributed.LoadStreamRef(ctx, streamID)
	if err != nil || !ok || strings.TrimSpace(ref.OwnerID) == "" {
		return false, err
	}
	if err := m.distributed.PublishCommand(ctx, ref.OwnerID, Command{
		Type:      CommandAbort,
		BotID:     ref.BotID,
		SessionID: ref.SessionID,
		StreamID:  ref.StreamID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return false, err
	}
	if err := m.waitForAbortAck(ctx, ref); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) abortLocal(ctx context.Context, ctrl *runControl) (bool, error) {
	if ctrl == nil || m.localControl(ctrl.streamID) != ctrl {
		return false, ErrCommandTargetNotActive
	}
	if m.distributed != nil && !ctrl.leaseIsValidAt(time.Now()) {
		m.cancelRunControl(ctrl)
		return false, ErrCommandTargetNotActive
	}
	if err := m.requestAbort(ctx, ctrl); err != nil {
		return false, err
	}
	select {
	case ctrl.abortCh <- struct{}{}:
	default:
	}
	if ctrl.cancel != nil {
		ctrl.cancel()
	}
	return true, nil
}

// DispatchActiveCommand routes a response for a UI request embedded in the
// current run to that run's owner. A false handled result means the target is
// not part of the active run and the caller may use its deferred flow.
func (m *Manager) DispatchActiveCommand(ctx context.Context, botID, sessionID, commandType, targetID string, payload []byte) (bool, error) {
	if m == nil || m.backend == nil {
		return false, nil
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	targetID = strings.TrimSpace(targetID)
	if botID == "" || sessionID == "" || targetID == "" {
		return false, nil
	}
	if commandType != CommandToolApprovalResponse && commandType != CommandUserInputResponse {
		return false, fmt.Errorf("unsupported active runtime command %q", commandType)
	}
	snapshot, err := m.Snapshot(ctx, botID, sessionID)
	if err != nil {
		return false, err
	}
	run := snapshot.CurrentRunView
	if run == nil || !isActiveRunStatus(run.Status) || !runtimeCommandTargetPresent(run, commandType, targetID) {
		return false, nil
	}
	if m.distributed == nil {
		commandCtx, cancel := context.WithTimeout(ctx, m.commandTimeout())
		defer cancel()
		return true, m.applyRoutedCommand(commandCtx, Command{
			Type: commandType, ID: uuid.NewString(), BotID: botID, SessionID: sessionID,
			StreamID: strings.TrimSpace(run.StreamID), TargetID: targetID,
			Payload: append([]byte(nil), payload...), CreatedAt: time.Now().UTC(),
		})
	}
	ownerID := strings.TrimSpace(run.OwnerID)
	if ownerID == "" {
		return true, errors.New("target runtime owner is unknown")
	}
	cmd := Command{
		Type:         commandType,
		ID:           uuid.NewString(),
		ReplyOwnerID: m.ownerID,
		BotID:        botID,
		SessionID:    sessionID,
		StreamID:     strings.TrimSpace(run.StreamID),
		TargetID:     targetID,
		Payload:      append([]byte(nil), payload...),
		CreatedAt:    time.Now().UTC(),
	}
	if ownerID == m.ownerID {
		commandCtx, cancel := context.WithTimeout(ctx, m.commandTimeout())
		defer cancel()
		return true, m.applyRoutedCommand(commandCtx, cmd)
	}

	result := make(chan error, 1)
	m.mu.Lock()
	if m.isClosed() {
		m.mu.Unlock()
		return true, ErrManagerClosed
	}
	m.pendingCommands[cmd.ID] = result
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if m.pendingCommands[cmd.ID] == result {
			delete(m.pendingCommands, cmd.ID)
		}
		m.mu.Unlock()
	}()
	if err := m.distributed.PublishCommand(ctx, ownerID, cmd); err != nil {
		return true, err
	}
	timeout := m.commandTimeout()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return true, err
	case <-ctx.Done():
		return true, ctx.Err()
	case <-timer.C:
		return true, errors.New("runtime command was not acknowledged")
	}
}

func (m *Manager) requestAbort(ctx context.Context, ctrl *runControl) error {
	if ctrl == nil {
		return nil
	}
	_, _, err := m.updateActiveAndPublish(ctx, Key{BotID: ctrl.botID, SessionID: ctrl.sessionID}, ctrl.streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		run := snapshot.CurrentRunView
		if run.StreamID != ctrl.streamID || !m.runOwnerMatches(run) || !isActiveRunStatus(run.Status) {
			return snapshot, false, nil
		}
		if strings.EqualFold(run.Status, RunStatusAborting) {
			return snapshot, false, nil
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		run.Status = RunStatusAborting
		run.UpdatedAt = now
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, true, false, false, false)
	})
	return err
}

func (m *Manager) Steer(ctx context.Context, botID, sessionID, streamID, text string) (SteerState, error) {
	if m == nil || m.backend == nil {
		return SteerState{}, errors.New("session runtime manager is not configured")
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	text = strings.TrimSpace(text)
	if botID == "" || sessionID == "" || text == "" {
		return SteerState{}, errors.New("bot_id, session_id, and text are required")
	}
	if streamID == "" {
		snapshot, err := m.Snapshot(ctx, botID, sessionID)
		if err != nil {
			return SteerState{}, err
		}
		if snapshot.CurrentRunView == nil {
			return SteerState{}, errors.New("no active runtime run")
		}
		streamID = strings.TrimSpace(snapshot.CurrentRunView.StreamID)
	}
	var steer SteerState
	var ownerID string
	var commandCreatedAt time.Time
	_, _, err := m.updateActiveAndPublish(ctx, Key{BotID: botID, SessionID: sessionID}, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
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
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, false, false, true, false)
	})
	if err != nil {
		return SteerState{}, err
	}

	cmd := Command{Type: CommandSteer, BotID: botID, SessionID: sessionID, StreamID: streamID, SteerID: steer.ID, Text: text, CreatedAt: commandCreatedAt}
	if ctrl := m.localControl(streamID); ctrl != nil {
		m.applyCommand(ctx, cmd)
	} else {
		if ownerID == "" {
			return steer, errors.New("target runtime owner is unknown")
		}
		if m.distributed == nil {
			return steer, errors.New("active runtime is not local")
		}
		if err := m.distributed.PublishCommand(ctx, ownerID, cmd); err != nil {
			_ = m.updateSteerStatus(context.WithoutCancel(ctx), botID, sessionID, streamID, steer.ID, SteerStatusRejected, err.Error())
			return steer, err
		}
	}
	m.rejectPendingSteerAfterTimeout(context.WithoutCancel(ctx), botID, sessionID, streamID, steer.ID)
	return steer, nil
}

func (m *Manager) applyCommand(ctx context.Context, cmd Command) {
	switch strings.TrimSpace(cmd.Type) {
	case CommandAbort:
		ctrl := m.localControl(strings.TrimSpace(cmd.StreamID))
		if ctrl == nil || ctrl.botID != strings.TrimSpace(cmd.BotID) || ctrl.sessionID != strings.TrimSpace(cmd.SessionID) {
			m.logger.Warn("ignore abort command for inactive local run", slog.String("stream_id", cmd.StreamID))
			return
		}
		if _, err := m.abortLocal(ctx, ctrl); err != nil {
			m.logger.Warn("apply abort command failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
		}
	case CommandSteer:
		m.applySteerCommand(ctx, cmd)
	case CommandToolApprovalResponse, CommandUserInputResponse:
		commandCtx, cancel := context.WithTimeout(ctx, m.commandTimeout())
		err := m.applyRoutedCommand(commandCtx, cmd)
		cancel()
		m.publishCommandResult(ctx, cmd, err)
	case CommandResult:
		m.completePendingCommand(cmd)
	}
}

func (m *Manager) applyRoutedCommand(ctx context.Context, cmd Command) error {
	ctrl := m.localControl(strings.TrimSpace(cmd.StreamID))
	if ctrl == nil || ctrl.botID != strings.TrimSpace(cmd.BotID) || ctrl.sessionID != strings.TrimSpace(cmd.SessionID) {
		return ErrCommandTargetNotActive
	}
	if err := m.ValidateRunOwnership(ctx, cmd.BotID, cmd.SessionID, cmd.StreamID); err != nil {
		return ErrCommandTargetNotActive
	}
	snapshot, ok, err := m.backend.Load(ctx, Key{BotID: cmd.BotID, SessionID: cmd.SessionID})
	if err != nil {
		return err
	}
	if !ok || snapshot.CurrentRunView == nil {
		return ErrCommandTargetNotActive
	}
	run := snapshot.CurrentRunView
	if run.StreamID != ctrl.streamID || !m.runOwnerMatches(run) || !isActiveRunStatus(run.Status) || !runtimeCommandTargetPresent(run, cmd.Type, cmd.TargetID) {
		return ErrCommandTargetNotActive
	}
	m.mu.Lock()
	handler := m.commandHandler
	m.mu.Unlock()
	if handler == nil {
		return errors.New("runtime command handler is not configured")
	}
	return handler(ctx, cmd)
}

func (m *Manager) publishCommandResult(ctx context.Context, request Command, err error) {
	replyOwnerID := strings.TrimSpace(request.ReplyOwnerID)
	if replyOwnerID == "" {
		return
	}
	if m.distributed == nil {
		return
	}
	result := Command{Type: CommandResult, ID: request.ID, CreatedAt: time.Now().UTC()}
	if err != nil {
		result.Error = err.Error()
		if errors.Is(err, ErrCommandTargetNotActive) {
			result.ErrorCode = "target_not_active"
		} else {
			result.ErrorCode = "runtime_command_failed"
		}
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.commandTimeout())
	defer cancel()
	if publishErr := m.distributed.PublishCommand(publishCtx, replyOwnerID, result); publishErr != nil {
		m.logger.Warn("publish runtime command result failed", slog.Any("error", publishErr), slog.String("command_id", request.ID))
	}
}

func (m *Manager) commandTimeout() time.Duration {
	if m != nil && m.commandAckTTL > 0 {
		return m.commandAckTTL
	}
	return defaultCommandAckTTL
}

func (m *Manager) completePendingCommand(result Command) {
	m.mu.Lock()
	pending := m.pendingCommands[strings.TrimSpace(result.ID)]
	if pending != nil {
		delete(m.pendingCommands, strings.TrimSpace(result.ID))
	}
	m.mu.Unlock()
	if pending == nil {
		return
	}
	var err error
	if strings.TrimSpace(result.Error) != "" {
		if result.ErrorCode == "target_not_active" {
			err = fmt.Errorf("%w: %s", ErrCommandTargetNotActive, result.Error)
		} else {
			err = errors.New(result.Error)
		}
	}
	pending <- err
}

func runtimeCommandTargetPresent(run *CurrentRunView, commandType, targetID string) bool {
	targetID = strings.TrimSpace(targetID)
	if run == nil || targetID == "" {
		return false
	}
	for _, message := range run.Messages {
		switch commandType {
		case CommandToolApprovalResponse:
			if message.Approval != nil && (strings.TrimSpace(message.Approval.ApprovalID) == targetID || strconv.Itoa(message.Approval.ShortID) == targetID) {
				return true
			}
		case CommandUserInputResponse:
			if message.UserInput != nil && (strings.TrimSpace(message.UserInput.UserInputID) == targetID || strconv.Itoa(message.UserInput.ShortID) == targetID) {
				return true
			}
		}
	}
	return false
}

func (m *Manager) applySteerCommand(ctx context.Context, cmd Command) {
	if err := m.ValidateRunOwnership(ctx, cmd.BotID, cmd.SessionID, cmd.StreamID); err != nil {
		_ = m.updateSteerStatus(context.WithoutCancel(ctx), cmd.BotID, cmd.SessionID, cmd.StreamID, cmd.SteerID, SteerStatusRejected, ErrRunOwnershipLost.Error())
		return
	}
	if !m.steerCommandIsPending(context.WithoutCancel(ctx), cmd) {
		return
	}
	now, err := m.backend.Now(ctx)
	if err != nil {
		m.logger.Warn("load runtime backend time for steer command failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
		return
	}
	if m.commandAckTTL > 0 && !cmd.CreatedAt.IsZero() && now.After(cmd.CreatedAt.Add(m.commandAckTTL)) {
		if err := m.updateSteerStatus(context.WithoutCancel(ctx), cmd.BotID, cmd.SessionID, cmd.StreamID, cmd.SteerID, SteerStatusRejected, steerNotAcknowledgedError); err != nil {
			m.logger.Warn("reject expired steer command failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
		}
		return
	}

	ctrl := m.localControl(cmd.StreamID)
	errText := ""
	if ctrl != nil && ctrl.injectCh != nil && strings.TrimSpace(cmd.Text) != "" {
		queued, err := m.transitionSteerStatus(context.WithoutCancel(ctx), cmd.BotID, cmd.SessionID, cmd.StreamID, cmd.SteerID, SteerStatusQueued, "")
		if err != nil {
			m.logger.Warn("acknowledge queued steer failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
			return
		}
		if !queued {
			return
		}
		select {
		case ctrl.injectCh <- conversation.InjectMessage{
			Text: strings.TrimSpace(cmd.Text),
			Applied: func() {
				if err := m.updateSteerStatus(context.WithoutCancel(ctx), cmd.BotID, cmd.SessionID, cmd.StreamID, cmd.SteerID, SteerStatusApplied, ""); err != nil {
					m.logger.Warn("acknowledge applied steer failed", slog.Any("error", err), slog.String("stream_id", cmd.StreamID))
				}
			},
		}:
			return
		case <-ctx.Done():
			errText = ctx.Err().Error()
		default:
			errText = "active runtime is not accepting steer messages"
		}
	} else {
		errText = "active runtime is not available"
	}
	if err := m.updateSteerStatus(context.WithoutCancel(ctx), cmd.BotID, cmd.SessionID, cmd.StreamID, cmd.SteerID, SteerStatusRejected, errText); err != nil {
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
	if !ok || snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != strings.TrimSpace(cmd.StreamID) {
		return false
	}
	steer := snapshot.CurrentRunView.Steer
	return steer != nil && steer.ID == strings.TrimSpace(cmd.SteerID) && strings.EqualFold(steer.Status, SteerStatusPending)
}

func (m *Manager) updateSteerStatus(ctx context.Context, botID, sessionID, streamID, steerID, status, errText string) error {
	_, err := m.transitionSteerStatus(ctx, botID, sessionID, streamID, steerID, status, errText)
	return err
}

func (m *Manager) transitionSteerStatus(ctx context.Context, botID, sessionID, streamID, steerID, status, errText string) (bool, error) {
	_, changed, err := m.updateActiveAndPublish(ctx, Key{BotID: botID, SessionID: sessionID}, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if snapshot.CurrentRunView.StreamID != streamID {
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
		snapshot.CurrentRunView.Steer.Error = strings.TrimSpace(errText)
		snapshot.CurrentRunView.Steer.UpdatedAt = now
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return runtimeRunPatch(snapshot, false, false, true, false)
	})
	return changed, err
}

const steerNotAcknowledgedError = "runtime steer command was not acknowledged"

func (m *Manager) rejectPendingSteerAfterTimeout(ctx context.Context, botID, sessionID, streamID, steerID string) {
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
		err := m.rejectUnacknowledgedSteer(ctx, botID, sessionID, streamID, steerID)
		if err != nil {
			m.logger.Warn("reject pending steer failed", slog.Any("error", err), slog.String("stream_id", streamID))
		}
	})
}

func (m *Manager) rejectUnacknowledgedSteer(ctx context.Context, botID, sessionID, streamID, steerID string) error {
	_, _, err := m.updateActiveAndPublish(ctx, Key{BotID: botID, SessionID: sessionID}, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if snapshot.CurrentRunView.StreamID != streamID {
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
		steer.Error = steerNotAcknowledgedError
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

func (m *Manager) waitForAbortAck(ctx context.Context, ref StreamRef) error {
	timeout := m.commandAckTTL
	if timeout <= 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(commandAckPollInterval(timeout))
	defer ticker.Stop()
	for {
		snapshot, err := m.Snapshot(waitCtx, ref.BotID, ref.SessionID)
		if err != nil {
			return err
		}
		if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != ref.StreamID || strings.EqualFold(snapshot.CurrentRunView.Status, RunStatusAborting) || !isActiveRunStatus(snapshot.CurrentRunView.Status) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return errors.New("runtime abort command was not acknowledged")
		case <-ticker.C:
		}
	}
}
