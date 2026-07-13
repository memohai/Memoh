package sessionruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

type Manager struct {
	backend       Backend
	distributed   DistributedBackend
	ownerID       string
	stateTTL      time.Duration
	ownerLeaseTTL time.Duration
	commandAckTTL time.Duration
	logger        *slog.Logger

	mu              sync.Mutex
	controls        map[string]*runControl
	commandHandler  func(context.Context, Command) error
	pendingCommands map[string]chan error

	commandCancel context.CancelFunc
	commandDone   chan struct{}
	closeCh       chan struct{}
	closeOnce     sync.Once
}

type runControl struct {
	botID           string
	sessionID       string
	streamID        string
	abortCh         chan<- struct{}
	cancel          context.CancelFunc
	injectCh        chan<- conversation.InjectMessage
	converter       *conversation.UIMessageStreamConverter
	leaseStop       func()
	leaseDone       chan struct{}
	leaseMu         sync.RWMutex
	leaseValidUntil time.Time
	ready           chan struct{}
	readyOnce       sync.Once
	finishRetryOnce sync.Once
}

type Options struct {
	OwnerID       string
	StateTTL      time.Duration
	OwnerLeaseTTL time.Duration
	CommandAckTTL time.Duration
	Logger        *slog.Logger
}

const defaultCommandAckTTL = 2 * time.Second

func NewManager(backend Backend, opts Options) *Manager {
	ownerID := strings.TrimSpace(opts.OwnerID)
	if ownerID == "" {
		ownerID = uuid.NewString()
	}
	stateTTL := opts.StateTTL
	if stateTTL <= 0 {
		stateTTL = 24 * time.Hour
	}
	leaseTTL := opts.OwnerLeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}
	commandAckTTL := opts.CommandAckTTL
	if commandAckTTL <= 0 {
		commandAckTTL = defaultCommandAckTTL
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	distributed, _ := backend.(DistributedBackend)
	return &Manager{
		backend:         backend,
		distributed:     distributed,
		ownerID:         ownerID,
		stateTTL:        stateTTL,
		ownerLeaseTTL:   leaseTTL,
		commandAckTTL:   commandAckTTL,
		logger:          log.With(slog.String("component", "session_runtime")),
		controls:        make(map[string]*runControl),
		pendingCommands: make(map[string]chan error),
		closeCh:         make(chan struct{}),
	}
}

func (m *Manager) isClosed() bool {
	if m == nil || m.closeCh == nil {
		return false
	}
	select {
	case <-m.closeCh:
		return true
	default:
		return false
	}
}

// SetCommandHandler installs the owner-local executor for routed runtime
// commands whose domain behavior lives outside the sessionruntime package.
func (m *Manager) SetCommandHandler(handler func(context.Context, Command) error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.commandHandler = handler
	m.mu.Unlock()
}

func (m *Manager) Start(ctx context.Context) error {
	if m == nil || m.distributed == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	sub, err := m.distributed.SubscribeCommands(ctx, m.ownerID)
	if err != nil {
		cancel()
		if m.isClosed() {
			return ErrManagerClosed
		}
		return err
	}
	stopCommands := sync.OnceFunc(func() {
		cancel()
		sub.Close()
	})
	commandDone := make(chan struct{})
	m.mu.Lock()
	if m.isClosed() {
		m.mu.Unlock()
		stopCommands()
		return ErrManagerClosed
	}
	if m.commandCancel != nil || m.commandDone != nil {
		m.mu.Unlock()
		stopCommands()
		return errors.New("session runtime manager is already started")
	}
	m.commandCancel = stopCommands
	m.commandDone = commandDone
	m.mu.Unlock()
	go func() {
		defer close(commandDone)
		for {
			select {
			case <-ctx.Done():
				return
			case cmd, ok := <-sub.C:
				if !ok {
					return
				}
				m.applyCommand(context.WithoutCancel(ctx), cmd)
			}
		}
	}()
	return nil
}

func (m *Manager) Close() error {
	return m.CloseContext(context.Background())
}

func (m *Manager) CloseContext(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if m.closeCh != nil {
		m.closeOnce.Do(func() { close(m.closeCh) })
	}
	m.mu.Lock()
	commandCancel := m.commandCancel
	commandDone := m.commandDone
	m.commandCancel = nil
	m.commandDone = nil
	pendingCommands := make([]chan error, 0, len(m.pendingCommands))
	for commandID, pending := range m.pendingCommands {
		pendingCommands = append(pendingCommands, pending)
		delete(m.pendingCommands, commandID)
	}
	m.mu.Unlock()
	if commandCancel != nil {
		commandCancel()
	}
	for _, pending := range pendingCommands {
		pending <- ErrManagerClosed
	}
	m.stopAllLocalControls(ctx)
	if commandDone != nil {
		select {
		case <-commandDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.backend != nil {
		return m.backend.Close()
	}
	return nil
}

func (m *Manager) StartRun(ctx context.Context, botID, sessionID, streamID string, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	return m.StartRunWithAdmission(ctx, botID, sessionID, streamID, RunAdmissionView{}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithOperation(ctx context.Context, botID, sessionID, streamID string, operation *RunOperationView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	return m.StartRunWithAdmission(ctx, botID, sessionID, streamID, RunAdmissionView{Operation: operation}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithAdmission(ctx context.Context, botID, sessionID, streamID string, admission RunAdmissionView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	return m.StartRunWithAdmissionBuilder(ctx, botID, sessionID, streamID, func(context.Context) (RunAdmissionView, error) {
		return admission, nil
	}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithOperationBuilder(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context) (*RunOperationView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	if builder == nil {
		return errors.New("runtime operation builder is required")
	}
	return m.StartRunWithAdmissionBuilder(ctx, botID, sessionID, streamID, func(ctx context.Context) (RunAdmissionView, error) {
		operation, err := builder(ctx)
		return RunAdmissionView{Operation: operation}, err
	}, abortCh, cancel, injectCh)
}

// StartRunWithAdmissionBuilder reserves the cross-server run before executing
// builder, then publishes the running view only after the canonical request
// turn and optional replacement operation are ready.
func (m *Manager) StartRunWithAdmissionBuilder(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context) (RunAdmissionView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	if m == nil || m.backend == nil {
		return nil
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	if botID == "" || sessionID == "" || streamID == "" {
		return errors.New("bot_id, session_id, and stream_id are required")
	}
	if builder == nil {
		return errors.New("runtime admission builder is required")
	}

	ctrl := &runControl{
		botID:     botID,
		sessionID: sessionID,
		streamID:  streamID,
		abortCh:   abortCh,
		cancel:    cancel,
		injectCh:  injectCh,
		converter: conversation.NewUIMessageStreamConverter(),
		ready:     make(chan struct{}),
	}
	defer ctrl.markReady()
	m.mu.Lock()
	select {
	case <-m.closeCh:
		m.mu.Unlock()
		return ErrManagerClosed
	default:
	}
	if _, exists := m.controls[streamID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("stream_id %q is already owned by this runtime manager", streamID)
	}
	m.controls[streamID] = ctrl
	m.mu.Unlock()

	now, err := m.backend.Now(ctx)
	if err != nil {
		m.removeLocalControl(streamID, ctrl)
		return fmt.Errorf("load runtime backend time: %w", err)
	}
	key := Key{BotID: botID, SessionID: sessionID}
	ownerID := ""
	var leaseExpiresAt *time.Time
	if m.distributed != nil {
		ownerID = m.ownerID
		expiresAt := now.Add(m.ownerLeaseTTL)
		leaseExpiresAt = &expiresAt
		// The backend claim below is authoritative, but a local deadline must
		// exist before it returns so an abort can cancel blocked admission.
		ctrl.setLeaseValidUntil(time.Now().Add(m.ownerLeaseTTL))
	}
	var expiredStreamID string
	claim := func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		if snapshot.CurrentRunView != nil && isActiveRunStatus(snapshot.CurrentRunView.Status) && snapshot.CurrentRunView.StreamID != streamID {
			if m.distributed == nil || !m.markLostIfExpired(&snapshot, now) {
				return snapshot, false, fmt.Errorf("session %q already has an active runtime run", sessionID)
			}
			expiredStreamID = snapshot.CurrentRunView.StreamID
		}
		snapshot.BotID = botID
		snapshot.SessionID = sessionID
		snapshot.Seq++
		snapshot.Queue = nonNilQueue(snapshot.Queue)
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView = &CurrentRunView{
			StreamID:            streamID,
			Status:              RunStatusAdmitting,
			OwnerID:             ownerID,
			OwnerLeaseExpiresAt: leaseExpiresAt,
			StartedAt:           now,
			UpdatedAt:           now,
			Messages:            []conversation.UIMessage{},
		}
		return snapshot, true, nil
	}
	var claimedSnapshot Snapshot
	var changed bool
	if m.distributed != nil {
		claimedSnapshot, changed, err = m.distributed.StartRun(ctx, key, StreamRef{
			BotID: botID, SessionID: sessionID, StreamID: streamID, OwnerID: m.ownerID,
		}, claim)
	} else {
		claimedSnapshot, changed, err = m.backend.Update(ctx, key, claim)
	}
	if err != nil {
		m.removeLocalControl(streamID, ctrl)
		return err
	}
	if !changed {
		m.removeLocalControl(streamID, ctrl)
		return nil
	}
	if err := m.publishRuntimeDelta(context.WithoutCancel(ctx), claimedSnapshot, streamID, RuntimeDelta{CurrentRunView: claimedSnapshot.CurrentRunView}); err != nil {
		m.logger.Warn("publish admitting runtime checkpoint failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", streamID))
	}
	if expiredStreamID != "" && m.distributed != nil {
		_ = m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), expiredStreamID)
	}
	if m.distributed != nil {
		renewedAt, renewErr := m.backend.Now(context.WithoutCancel(ctx))
		if renewErr == nil {
			renewErr = m.distributed.RenewLease(context.WithoutCancel(ctx), key, streamID, m.ownerID, renewedAt, renewedAt.Add(m.ownerLeaseTTL))
		}
		if renewErr != nil {
			ctrl.markReady()
			if errors.Is(renewErr, ErrRunOwnershipLost) {
				m.forgetLocalControl(context.WithoutCancel(ctx), streamID)
			} else {
				_ = m.FinishRun(context.WithoutCancel(ctx), botID, sessionID, streamID, RunStatusErrored, renewErr.Error())
			}
			return fmt.Errorf("confirm runtime owner lease: %w", renewErr)
		}
		ctrl.setLeaseValidUntil(time.Now().Add(m.ownerLeaseTTL))
		m.startLeaseRenewal(context.WithoutCancel(ctx), ctrl)
	}
	admission, err := builder(ctx)
	if err != nil {
		ctrl.markReady()
		status := RunStatusErrored
		message := err.Error()
		if errors.Is(err, context.Canceled) {
			status = RunStatusAborted
			message = ""
		}
		_ = m.FinishRun(context.WithoutCancel(ctx), botID, sessionID, streamID, status, message)
		return err
	}
	if m.isClosed() {
		return ErrManagerClosed
	}
	if m.localControl(streamID) != ctrl {
		return ErrRunOwnershipLost
	}
	admission, err = normalizeRunAdmission(admission)
	if err != nil {
		ctrl.markReady()
		_ = m.FinishRun(context.WithoutCancel(ctx), botID, sessionID, streamID, RunStatusErrored, err.Error())
		return err
	}
	_, changed, err = m.updateActiveAndPublish(context.WithoutCancel(ctx), key, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if m.isClosed() {
			return snapshot, false, ErrManagerClosed
		}
		if m.localControl(streamID) != ctrl {
			return snapshot, false, ErrRunOwnershipLost
		}
		run := snapshot.CurrentRunView
		if run.StreamID == streamID && m.runOwnerMatches(run) && strings.EqualFold(run.Status, RunStatusAborting) {
			return snapshot, false, context.Canceled
		}
		if run.StreamID != streamID || !m.runOwnerMatches(run) || !strings.EqualFold(run.Status, RunStatusAdmitting) {
			return snapshot, false, errors.New("reserved runtime run is no longer owned by this manager")
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		run.Status = RunStatusRunning
		run.RequestUserTurn = admission.RequestUserTurn
		run.Operation = admission.Operation
		run.UpdatedAt = now
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		return RuntimeDelta{CurrentRunView: snapshot.CurrentRunView}
	})
	if err != nil || !changed {
		ctrl.markReady()
		if errors.Is(err, ErrManagerClosed) || errors.Is(err, ErrRunOwnershipLost) {
			return err
		}
		if err == nil {
			err = errors.New("reserved runtime run could not be activated")
		}
		status := RunStatusErrored
		message := err.Error()
		if errors.Is(err, context.Canceled) {
			status = RunStatusAborted
			message = ""
		}
		_ = m.FinishRun(context.WithoutCancel(ctx), botID, sessionID, streamID, status, message)
		return err
	}
	ctrl.markReady()
	return nil
}

func (m *Manager) FinishRun(ctx context.Context, botID, sessionID, streamID, status, message string) error {
	if m == nil || m.backend == nil {
		return nil
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	if botID == "" || sessionID == "" || streamID == "" {
		return nil
	}
	status = strings.TrimSpace(status)
	finishMessage := strings.TrimSpace(message)
	changed, err := m.finishRunState(ctx, botID, sessionID, streamID, status, finishMessage)
	if err == nil || changed {
		m.cleanupFinishedRun(context.WithoutCancel(ctx), streamID)
		return err
	}
	if errors.Is(err, ErrRunOwnershipLost) {
		snapshot, ok, loadErr := m.backend.Load(context.WithoutCancel(ctx), Key{BotID: botID, SessionID: sessionID})
		if loadErr == nil && ok && snapshot.CurrentRunView != nil && snapshot.CurrentRunView.StreamID == streamID && !isActiveRunStatus(snapshot.CurrentRunView.Status) {
			m.cleanupFinishedRun(context.WithoutCancel(ctx), streamID)
			return nil
		}
		m.forgetLocalControl(context.WithoutCancel(ctx), streamID)
		return err
	}
	if ctrl := m.localControl(streamID); ctrl != nil {
		retryCtx := context.WithoutCancel(ctx)
		ctrl.finishRetryOnce.Do(func() {
			go m.retryFinishRun(retryCtx, ctrl, status, finishMessage)
		})
	}
	return err
}

const steerRunFinishedError = "runtime run finished before steer was applied"

func rejectPendingSteerOnRunFinish(run *CurrentRunView, now time.Time) {
	if run == nil || run.Steer == nil || !isPendingSteerStatus(run.Steer.Status) {
		return
	}
	run.Steer.Status = SteerStatusRejected
	run.Steer.Error = steerRunFinishedError
	run.Steer.UpdatedAt = now
}

func (m *Manager) finishRunState(ctx context.Context, botID, sessionID, streamID, status, finishMessage string) (bool, error) {
	admissionTerminal := false
	_, changed, err := m.updateActiveAndPublish(ctx, Key{BotID: botID, SessionID: sessionID}, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		run := snapshot.CurrentRunView
		if !isActiveRunStatus(run.Status) {
			return snapshot, false, nil
		}
		if !m.runOwnerMatches(run) {
			return snapshot, false, ErrRunOwnershipLost
		}
		admissionTerminal = strings.EqualFold(run.Status, RunStatusAdmitting)
		finalStatus := status
		if finalStatus == "" {
			finalStatus = RunStatusCompleted
			if strings.EqualFold(snapshot.CurrentRunView.Status, RunStatusAborting) {
				finalStatus = RunStatusAborted
			} else if strings.TrimSpace(snapshot.CurrentRunView.Error) != "" {
				finalStatus = RunStatusErrored
			}
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView.Status = finalStatus
		snapshot.CurrentRunView.UpdatedAt = now
		switch {
		case finishMessage != "":
			snapshot.CurrentRunView.Error = finishMessage
		case finalStatus == RunStatusCompleted || finalStatus == RunStatusAborted:
			snapshot.CurrentRunView.Error = ""
		}
		snapshot.CurrentRunView.OwnerLeaseExpiresAt = nil
		rejectPendingSteerOnRunFinish(snapshot.CurrentRunView, now)
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		if admissionTerminal {
			return RuntimeDelta{CurrentRunView: snapshot.CurrentRunView}
		}
		return runtimeRunPatch(snapshot, true, true, true, m.distributed != nil)
	})
	return changed, err
}

func (m *Manager) retryFinishRun(ctx context.Context, ctrl *runControl, status, message string) {
	if ctrl == nil {
		return
	}
	delay := 100 * time.Millisecond
	for {
		select {
		case <-m.closeCh:
			return
		case <-time.After(delay):
		}
		if m.localControl(ctrl.streamID) != ctrl {
			return
		}
		changed, err := m.finishRunState(ctx, ctrl.botID, ctrl.sessionID, ctrl.streamID, status, message)
		if err == nil || changed {
			if err != nil {
				m.logger.Warn("publish runtime finish failed after state commit", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
			}
			m.cleanupFinishedRun(ctx, ctrl.streamID)
			return
		}
		if errors.Is(err, ErrRunOwnershipLost) {
			m.forgetLocalControl(ctx, ctrl.streamID)
			return
		}
		m.logger.Warn("retry runtime finish failed", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
		if delay < time.Second {
			delay *= 2
			if delay > time.Second {
				delay = time.Second
			}
		}
	}
}

func (m *Manager) cleanupFinishedRun(ctx context.Context, streamID string) {
	m.forgetLocalControl(ctx, streamID)
	if m.distributed == nil {
		return
	}
	if err := m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), streamID); err != nil {
		m.logger.Warn("delete finished runtime stream reference failed", slog.Any("error", err), slog.String("stream_id", streamID))
	}
}

func (m *Manager) HandleAgentEvent(ctx context.Context, botID, sessionID, streamID string, event agentpkg.StreamEvent) ([]conversation.UIMessage, error) {
	if m == nil || m.backend == nil {
		return nil, nil
	}
	ctrl := m.localControl(streamID)
	if ctrl == nil {
		return nil, nil
	}
	if err := waitRunControlReady(ctx, ctrl); err != nil {
		return nil, err
	}
	if m.localControl(streamID) != ctrl {
		return nil, nil
	}

	var messages []conversation.UIMessage
	switch event.Type {
	case agentpkg.EventAgentStart:
	case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
		messages = ctrl.converter.ConvertTerminalMessages(event.Messages)
	case agentpkg.EventError:
	default:
		messages = ctrl.converter.HandleEvent(conversation.UIStreamEventFromAgentEvent(event))
	}
	delta, visibleChange := runtimeDeltaForAgentEvent(event, messages)
	if !visibleChange {
		return messages, nil
	}

	_, changed, err := m.updateActiveAndPublish(ctx, Key{BotID: botID, SessionID: sessionID}, streamID, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		run := snapshot.CurrentRunView
		if run.StreamID != streamID || !m.runOwnerMatches(run) || !isEventAcceptingRunStatus(run.Status) {
			return snapshot, false, nil
		}
		snapshot.Seq++
		snapshot.Queue = nonNilQueue(snapshot.Queue)
		snapshot.UpdatedAt = now
		run.UpdatedAt = now
		if event.Type == agentpkg.EventRetry {
			run.Messages = []conversation.UIMessage{}
		}
		for _, msg := range messages {
			run.Messages = upsertUIMessage(run.Messages, msg)
		}
		switch event.Type {
		case agentpkg.EventAgentEnd:
			switch {
			case strings.TrimSpace(run.Error) != "":
				run.Status = RunStatusErrored
			case strings.EqualFold(run.Status, RunStatusAborting):
				run.Status = RunStatusAborted
			default:
				run.Status = RunStatusCompleted
			}
			run.OwnerLeaseExpiresAt = nil
			rejectPendingSteerOnRunFinish(run, now)
		case agentpkg.EventAgentAbort:
			if strings.TrimSpace(run.Error) != "" {
				run.Status = RunStatusErrored
			} else {
				run.Status = RunStatusAborted
			}
			run.OwnerLeaseExpiresAt = nil
			rejectPendingSteerOnRunFinish(run, now)
		case agentpkg.EventError:
			run.Error = strings.TrimSpace(event.Error)
			if run.Error == "" {
				run.Error = "stream error"
			}
		}
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		switch event.Type {
		case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
			delta.Run = runtimeRunPatch(snapshot, true, true, true, m.distributed != nil).Run
		case agentpkg.EventError:
			delta.Run = runtimeRunPatch(snapshot, false, true, false, false).Run
		}
		return delta
	})
	if err != nil {
		if errors.Is(err, ErrRunOwnershipLost) {
			return nil, err
		}
		return messages, err
	}
	if !changed {
		return nil, nil
	}
	return messages, nil
}

func (m *Manager) Snapshot(ctx context.Context, botID, sessionID string) (Snapshot, error) {
	if m == nil || m.backend == nil {
		return EmptySnapshot(botID, sessionID), nil
	}
	key := Key{BotID: strings.TrimSpace(botID), SessionID: strings.TrimSpace(sessionID)}
	snapshot, ok, err := m.backend.Load(ctx, key)
	if err != nil {
		return Snapshot{}, err
	}
	if !ok {
		return EmptySnapshot(key.BotID, key.SessionID), nil
	}
	if m.distributed != nil {
		now, err := m.backend.Now(ctx)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load runtime backend time: %w", err)
		}
		if !m.leaseExpired(snapshot.CurrentRunView, now) {
			snapshot.Queue = nonNilQueue(snapshot.Queue)
			return snapshot, nil
		}
		lostStreamID := snapshot.CurrentRunView.StreamID
		updated, changed, err := m.updateAndPublish(ctx, key, lostStreamID, func(current Snapshot, ok bool) (Snapshot, bool, error) {
			if !ok {
				return current, false, nil
			}
			if !m.markLostIfExpired(&current, now) {
				return current, false, nil
			}
			return current, true, nil
		}, func(snapshot Snapshot) RuntimeDelta {
			return runtimeRunPatch(snapshot, true, true, false, true)
		})
		if err != nil {
			return Snapshot{}, err
		}
		snapshot = updated
		if changed && lostStreamID != "" {
			_ = m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), lostStreamID)
			if ctrl := m.localControl(lostStreamID); ctrl != nil {
				m.cancelRunControl(ctrl)
			}
		}
	}
	snapshot.Queue = nonNilQueue(snapshot.Queue)
	return snapshot, nil
}

func (m *Manager) Subscribe(ctx context.Context, botID, sessionID string) (Subscription, error) {
	if m == nil || m.backend == nil {
		ch := make(chan Event)
		close(ch)
		return Subscription{C: ch, Close: func() {}}, nil
	}
	key := Key{BotID: strings.TrimSpace(botID), SessionID: strings.TrimSpace(sessionID)}
	subCtx, cancel := context.WithCancel(ctx)
	backendSub, err := m.backend.Subscribe(subCtx, key)
	if err != nil {
		cancel()
		return Subscription{}, err
	}

	out := make(chan Event, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		defer backendSub.Close()
		reconcileInterval := 2 * time.Second
		if m.distributed != nil {
			reconcileInterval = runtimeReconcileInterval(m.ownerLeaseTTL)
		}
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		var lastSeq int64
		var lastUpdatedAt time.Time
		events := backendSub.C
		for {
			select {
			case <-subCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					events = nil
					enqueueRuntimeEvent(out, Event{
						Type:      EventRuntimeDropped,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Message:   "runtime backend subscription closed",
					})
					continue
				}
				updatedAt := time.Time{}
				if event.UpdatedAt != nil {
					updatedAt = *event.UpdatedAt
				}
				if event.Snapshot != nil {
					updatedAt = event.Snapshot.UpdatedAt
				}
				if event.Seq < lastSeq {
					if !updatedAt.After(lastUpdatedAt) {
						continue
					}
					resetSnapshot, err := m.Snapshot(subCtx, key.BotID, key.SessionID)
					if err != nil {
						enqueueRuntimeEvent(out, Event{
							Type:      EventRuntimeDropped,
							BotID:     key.BotID,
							SessionID: key.SessionID,
							Seq:       lastSeq,
							Message:   "runtime sequence epoch changed",
						})
						continue
					}
					event = Event{
						Type:      EventRuntimeSnapshot,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Seq:       resetSnapshot.Seq,
						Snapshot:  &resetSnapshot,
					}
					updatedAt = resetSnapshot.UpdatedAt
				}
				selfContained := event.Delta != nil && event.Delta.CurrentRunView != nil
				if event.Type == EventRuntimeDelta && !selfContained && lastSeq > 0 && event.Seq > lastSeq+1 {
					enqueueRuntimeEvent(out, Event{
						Type:      EventRuntimeDropped,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Seq:       lastSeq,
						Message:   "runtime delta sequence gap",
					})
				}
				if event.Seq > 0 {
					lastSeq = event.Seq
				}
				if updatedAt.After(lastUpdatedAt) {
					lastUpdatedAt = updatedAt
				}
				enqueueRuntimeEvent(out, event)
			case <-ticker.C:
				snapshot, err := m.Snapshot(subCtx, key.BotID, key.SessionID)
				if err != nil {
					if subCtx.Err() == nil {
						m.logger.Warn("reconcile runtime subscription failed", slog.Any("error", err), slog.String("session_id", key.SessionID))
					}
					continue
				}
				reset := snapshot.Seq < lastSeq && (snapshot.CurrentRunView == nil || snapshot.UpdatedAt.After(lastUpdatedAt))
				if snapshot.Seq <= lastSeq && !reset {
					continue
				}
				lastSeq = snapshot.Seq
				lastUpdatedAt = snapshot.UpdatedAt
				enqueueRuntimeEvent(out, Event{
					Type:      EventRuntimeSnapshot,
					BotID:     key.BotID,
					SessionID: key.SessionID,
					Seq:       snapshot.Seq,
					Snapshot:  &snapshot,
				})
			}
		}
	}()
	var closeOnce sync.Once
	return Subscription{
		C: out,
		Close: func() {
			closeOnce.Do(func() {
				cancel()
				<-done
			})
		},
	}, nil
}

func (m *Manager) updateAndPublish(ctx context.Context, key Key, streamID string, update SnapshotUpdate, buildDelta func(Snapshot) RuntimeDelta) (Snapshot, bool, error) {
	snapshot, changed, err := m.backend.Update(ctx, key, update)
	if err != nil || !changed {
		return snapshot, changed, err
	}
	delta := RuntimeDelta{}
	if buildDelta != nil {
		delta = buildDelta(snapshot)
	}
	if err := m.publishRuntimeDelta(ctx, snapshot, streamID, delta); err != nil {
		m.logger.Warn("publish runtime delta failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", streamID))
	}
	return snapshot, true, nil
}

func (m *Manager) updateActiveAndPublish(ctx context.Context, key Key, streamID string, update ActiveRunUpdate, buildDelta func(Snapshot) RuntimeDelta) (Snapshot, bool, error) {
	var snapshot Snapshot
	var changed bool
	var err error
	if m.distributed != nil {
		snapshot, changed, err = m.distributed.UpdateActiveRun(ctx, key, streamID, update)
	} else {
		if m.localControl(streamID) == nil {
			return Snapshot{}, false, ErrRunOwnershipLost
		}
		snapshot, changed, err = m.backend.Update(ctx, key, func(snapshot Snapshot, ok bool) (Snapshot, bool, error) {
			if !ok || snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != streamID || !isActiveRunStatus(snapshot.CurrentRunView.Status) {
				return snapshot, false, ErrRunOwnershipLost
			}
			now, nowErr := m.backend.Now(ctx)
			if nowErr != nil {
				return snapshot, false, nowErr
			}
			return update(snapshot, now)
		})
	}
	if err != nil || !changed {
		return snapshot, changed, err
	}
	delta := RuntimeDelta{}
	if buildDelta != nil {
		delta = buildDelta(snapshot)
	}
	if err := m.publishRuntimeDelta(ctx, snapshot, streamID, delta); err != nil {
		m.logger.Warn("publish runtime delta failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", streamID))
	}
	return snapshot, true, nil
}

func (m *Manager) runOwnerMatches(run *CurrentRunView) bool {
	if run == nil {
		return false
	}
	if m.distributed == nil {
		return strings.TrimSpace(run.OwnerID) == "" && run.OwnerLeaseExpiresAt == nil
	}
	return strings.TrimSpace(run.OwnerID) == m.ownerID
}

func (m *Manager) publishRuntimeDelta(ctx context.Context, snapshot Snapshot, streamID string, delta RuntimeDelta) error {
	eventStreamID := strings.TrimSpace(streamID)
	if eventStreamID == "" && snapshot.CurrentRunView != nil {
		eventStreamID = snapshot.CurrentRunView.StreamID
	}
	// No defensive clone here: both backends isolate on publish (memory clones
	// the event once for its subscribers, redis marshals it immediately).
	updatedAt := snapshot.UpdatedAt
	return m.backend.Publish(ctx, Event{
		Type:      EventRuntimeDelta,
		BotID:     snapshot.BotID,
		SessionID: snapshot.SessionID,
		StreamID:  eventStreamID,
		Seq:       snapshot.Seq,
		UpdatedAt: &updatedAt,
		Delta:     &delta,
	})
}
