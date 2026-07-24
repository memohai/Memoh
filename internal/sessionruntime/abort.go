package sessionruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (m *Manager) Abort(ctx context.Context, botID, sessionID, streamID string) (bool, error) {
	return m.abort(ctx, botID, sessionID, streamID, "")
}

func (m *Manager) AbortRun(ctx context.Context, handle RunHandle) (bool, error) {
	handle = handle.normalized()
	if !handle.valid() {
		return false, ErrRunOwnershipLost
	}
	return m.abort(ctx, handle.BotID, handle.SessionID, handle.StreamID, handle.Generation)
}

func (m *Manager) abort(ctx context.Context, botID, sessionID, streamID, expectedGeneration string) (bool, error) {
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	expectedGeneration = strings.TrimSpace(expectedGeneration)
	if m == nil || m.backend == nil || botID == "" || sessionID == "" || streamID == "" {
		return false, nil
	}
	if ctrl := m.localControlForScope(botID, sessionID, streamID); ctrl != nil {
		if ctrl.botID != botID || ctrl.sessionID != sessionID {
			return false, ErrCommandTargetMismatch
		}
		if expectedGeneration != "" && ctrl.generation != expectedGeneration {
			if m.distributed == nil {
				return false, ErrRunOwnershipLost
			}
		} else {
			aborted, abortErr := m.abortLocal(ctx, ctrl)
			if expectedGeneration != "" || abortErr == nil && aborted || !errors.Is(abortErr, ErrCommandTargetNotActive) {
				return aborted, abortErr
			}
			// A generationless request may have reached a server whose stale
			// local control has already lost its lease. Resolve the current
			// owner below instead of letting that stale control shadow it.
		}
	}
	if m.distributed == nil {
		return false, nil
	}
	ref, ok, err := m.distributed.LoadStreamRef(ctx, Key{BotID: botID, SessionID: sessionID}, streamID)
	if err != nil || !ok || strings.TrimSpace(ref.OwnerID) == "" {
		return false, err
	}
	if ref.BotID != botID || ref.SessionID != sessionID {
		return false, ErrCommandTargetMismatch
	}
	if expectedGeneration != "" && ref.Generation != expectedGeneration {
		return false, ErrRunOwnershipLost
	}
	createdAt, err := m.backend.Now(ctx)
	if err != nil {
		return false, fmt.Errorf("load runtime command time: %w", err)
	}
	cmd := Command{
		Type:       CommandAbort,
		ID:         "abort-" + uuid.NewString(),
		BotID:      ref.BotID,
		SessionID:  ref.SessionID,
		StreamID:   ref.StreamID,
		Generation: ref.Generation,
		CreatedAt:  createdAt,
		ExpiresAt:  createdAt.Add(m.commandTimeout()),
	}
	if err := m.dispatchRemoteCommand(ctx, ref.OwnerID, cmd); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) abortLocal(ctx context.Context, ctrl *runControl) (bool, error) {
	if ctrl == nil || m.localControlForHandle(ctrl.handle()) != ctrl {
		return false, ErrCommandTargetNotActive
	}
	phase, abortOwner := ctrl.requestAbortPhase()
	if phase != runAbortPhasePreClaim && m.distributed != nil && !ctrl.leaseIsValidAt(time.Now()) {
		m.cancelRunControl(ctrl)
		return false, ErrCommandTargetNotActive
	}
	if phase == runAbortPhasePreClaim {
		if acknowledged, err := m.requestAbort(ctx, ctrl); err != nil && !errors.Is(err, ErrRunOwnershipLost) && !errors.Is(err, ErrCommandTargetNotActive) {
			return false, err
		} else if acknowledged {
			if ctrl.claimVisibleAndTakeAbort() {
				if err := m.abortClaimedAdmission(context.WithoutCancel(ctx), ctrl); err != nil {
					return false, err
				}
			}
			return true, nil
		}
		if claimed, owner := ctrl.takeAbortIfClaimed(); claimed {
			if owner {
				if err := m.abortClaimedAdmission(context.WithoutCancel(ctx), ctrl); err != nil {
					return false, err
				}
			}
			return true, nil
		}
		select {
		case ctrl.abortCh <- struct{}{}:
		default:
		}
		if ctrl.admissionCancel != nil {
			ctrl.admissionCancel()
		}
		if ctrl.cancel != nil {
			ctrl.cancel()
		}
		return true, nil
	}
	if phase == runAbortPhaseAdmission {
		if abortOwner {
			if err := m.abortClaimedAdmission(context.WithoutCancel(ctx), ctrl); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	acknowledged, err := m.requestAbort(ctx, ctrl)
	if err != nil {
		return false, err
	}
	if !acknowledged {
		return false, ErrCommandTargetNotActive
	}
	ctrl.stopCommands()
	select {
	case ctrl.abortCh <- struct{}{}:
	default:
	}
	if ctrl.cancel != nil {
		ctrl.cancel()
	}
	m.scheduleAbortGrace(context.WithoutCancel(ctx), ctrl)
	return true, nil
}

func (m *Manager) scheduleAbortGrace(ctx context.Context, ctrl *runControl) {
	if m == nil || ctrl == nil || m.abortGraceTTL <= 0 {
		return
	}
	ctrl.abortGraceOnce.Do(func() {
		ctrl.abortGraceMu.Lock()
		defer ctrl.abortGraceMu.Unlock()
		if ctrl.abortGraceStopped {
			return
		}
		ctrl.abortGraceTimer = time.AfterFunc(m.abortGraceTTL, func() {
			m.forceAbortAfterGrace(ctx, ctrl)
		})
	})
}

func (m *Manager) forceAbortAfterGrace(ctx context.Context, ctrl *runControl) {
	if ctrl == nil || m.localControlForHandle(ctrl.handle()) != ctrl {
		return
	}
	finishCtx, cancel := context.WithTimeout(ctx, m.commandTimeout())
	err := m.FinishRun(finishCtx, ctrl.handle(), RunStatusAborted, "")
	cancel()
	if err == nil {
		return
	}
	m.logger.Warn("force runtime abort after grace period failed", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
	stopCtx, stopCancel := context.WithTimeout(ctx, m.commandTimeout())
	stopErr := m.stopLeaseRenewalContext(stopCtx, ctrl)
	stopCancel()
	if stopErr != nil {
		m.logger.Warn("stop runtime owner lease after abort grace period failed", slog.Any("error", stopErr), slog.String("stream_id", ctrl.streamID))
	}
	m.removeLocalControl(ctrl.streamID, ctrl)
}

const (
	runAbortPhasePreClaim = iota
	runAbortPhaseAdmission
	runAbortPhaseRunning
)

func (c *runControl) requestAbortPhase() (int, bool) {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	c.abortRequested = true
	if c.admissionComplete {
		return runAbortPhaseRunning, false
	}
	if !c.claimEstablished {
		return runAbortPhasePreClaim, false
	}
	if !c.abortFinalizing {
		c.abortFinalizing = true
		return runAbortPhaseAdmission, true
	}
	return runAbortPhaseAdmission, false
}

func (c *runControl) establishClaimForAbort() (bool, bool) {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	c.claimEstablished = true
	if !c.abortRequested {
		return false, false
	}
	if !c.abortFinalizing {
		c.abortFinalizing = true
		return true, true
	}
	return true, false
}

func (c *runControl) claimVisibleAndTakeAbort() bool {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	c.claimEstablished = true
	if c.abortFinalizing {
		return false
	}
	c.abortFinalizing = true
	return true
}

func (c *runControl) takeAbortIfClaimed() (bool, bool) {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	if !c.claimEstablished {
		return false, false
	}
	if !c.abortFinalizing {
		c.abortFinalizing = true
		return true, true
	}
	return true, false
}

func (c *runControl) completeAdmissionForAbort() bool {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	if c.abortRequested || c.abortFinalizing {
		return false
	}
	c.admissionComplete = true
	return true
}

func (c *runControl) abortWasRequested() bool {
	c.abortStateMu.Lock()
	defer c.abortStateMu.Unlock()
	return c.abortRequested
}

func (m *Manager) abortClaimedAdmission(ctx context.Context, ctrl *runControl) error {
	if ctrl == nil || m.localControlForHandle(ctrl.handle()) != ctrl {
		return ErrCommandTargetNotActive
	}
	acknowledged, err := m.requestAbort(ctx, ctrl)
	if err != nil {
		return err
	}
	if !acknowledged {
		return ErrCommandTargetNotActive
	}
	ctrl.stopCommands()
	select {
	case ctrl.abortCh <- struct{}{}:
	default:
	}
	if ctrl.cancel != nil {
		ctrl.cancel()
	}
	ctrl.markReady()
	return m.FinishRun(ctx, ctrl.handle(), RunStatusAborted, "")
}

func (m *Manager) requestAbort(ctx context.Context, ctrl *runControl) (bool, error) {
	if ctrl == nil {
		return false, nil
	}
	acknowledged := false
	_, _, err := m.updateActiveAndPublish(ctx, ctrl.handle(), func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		run := snapshot.CurrentRunView
		if run == nil {
			return snapshot, false, nil
		}
		if run.StreamID != ctrl.streamID || !m.runOwnerMatches(run) || !isActiveRunStatus(run.Status) {
			return snapshot, false, nil
		}
		acknowledged = true
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
	return acknowledged, err
}
