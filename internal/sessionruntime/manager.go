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
	backend        Backend
	distributed    DistributedBackend
	ownerID        string
	stateTTL       time.Duration
	ownerLeaseTTL  time.Duration
	commandAckTTL  time.Duration
	commandWorkers int
	logger         *slog.Logger
	newEpoch       func() string
	newGeneration  func() string

	mu                     sync.Mutex
	controls               map[string]*runControl
	commandHandler         func(context.Context, Command) error
	pendingCommands        map[string]map[*commandWaiter]struct{}
	inflightCommandTargets map[string]struct{}
	commandExecutions      map[string]chan struct{}
	admittedCommands       map[string]struct{}

	commandCancel context.CancelFunc
	commandDone   chan struct{}
	closeCh       chan struct{}
	closeOnce     sync.Once
}

type commandWaiter struct {
	result      chan error
	payloadHash string
}

type runControl struct {
	botID            string
	sessionID        string
	streamID         string
	generation       string
	abortCh          chan<- struct{}
	cancel           context.CancelFunc
	lifecycleCtx     context.Context
	lifecycleCancel  context.CancelFunc
	injectCh         chan<- conversation.InjectMessage
	injectMu         sync.Mutex
	injectClosed     bool
	converter        *conversation.UIMessageStreamConverter
	leaseStop        func()
	leaseDone        chan struct{}
	leaseLifecycleMu sync.Mutex
	leaseMu          sync.RWMutex
	leaseValidUntil  time.Time
	leaseChanged     chan struct{}
	ready            chan struct{}
	readyOnce        sync.Once
	finishRetryOnce  sync.Once
	ownershipCancel  context.CancelCauseFunc
	ownershipOnce    sync.Once
}

func (c *runControl) handle() RunHandle {
	if c == nil {
		return RunHandle{}
	}
	return RunHandle{BotID: c.botID, SessionID: c.sessionID, StreamID: c.streamID, Generation: c.generation}
}

type Options struct {
	OwnerID                string
	StateTTL               time.Duration
	OwnerLeaseTTL          time.Duration
	CommandAckTTL          time.Duration
	CommandWorkerLimit     int
	Logger                 *slog.Logger
	EpochGenerator         func() string
	RunGenerationGenerator func() string
}

const (
	defaultCommandAckTTL        = 2 * time.Second
	defaultCommandWorkerLimit   = 32
	defaultCommandRejectBacklog = 256
)

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
	commandWorkers := opts.CommandWorkerLimit
	if commandWorkers <= 0 {
		commandWorkers = defaultCommandWorkerLimit
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	distributed, _ := backend.(DistributedBackend)
	newEpoch := opts.EpochGenerator
	if newEpoch == nil {
		newEpoch = uuid.NewString
	}
	newGeneration := opts.RunGenerationGenerator
	if newGeneration == nil {
		newGeneration = uuid.NewString
	}
	return &Manager{
		backend:                backend,
		distributed:            distributed,
		ownerID:                ownerID,
		stateTTL:               stateTTL,
		ownerLeaseTTL:          leaseTTL,
		commandAckTTL:          commandAckTTL,
		commandWorkers:         commandWorkers,
		logger:                 log.With(slog.String("component", "session_runtime")),
		newEpoch:               newEpoch,
		newGeneration:          newGeneration,
		controls:               make(map[string]*runControl),
		pendingCommands:        make(map[string]map[*commandWaiter]struct{}),
		inflightCommandTargets: make(map[string]struct{}),
		commandExecutions:      make(map[string]chan struct{}),
		admittedCommands:       make(map[string]struct{}),
		closeCh:                make(chan struct{}),
	}
}

// IsDistributed reports whether this manager coordinates owners through a
// cross-process backend. Memory managers intentionally return false.
func (m *Manager) IsDistributed() bool {
	return m != nil && m.distributed != nil
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
	if m.isClosed() {
		return ErrManagerClosed
	}
	if checker, ok := m.distributed.(startupHealthChecker); ok {
		if err := checker.CheckHealth(ctx); err != nil {
			return err
		}
	}
	commandCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopStartupCancellation := context.AfterFunc(ctx, cancel)
	sub, err := m.distributed.SubscribeCommands(commandCtx, m.ownerID)
	if err != nil {
		stopStartupCancellation()
		cancel()
		if m.isClosed() {
			return ErrManagerClosed
		}
		if startupErr := ctx.Err(); startupErr != nil {
			return startupErr
		}
		return err
	}
	stopCommands := sync.OnceFunc(func() {
		cancel()
		sub.Close()
	})
	if !stopStartupCancellation() {
		stopCommands()
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
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
		jobs := make(chan Command, m.commandWorkers)
		rejected := make(chan Command, defaultCommandRejectBacklog)
		var workers sync.WaitGroup
		for range m.commandWorkers {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for cmd := range jobs {
					m.applyCommand(commandCtx, cmd)
					m.releaseCommandAdmission(cmd)
				}
			}()
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			for cmd := range rejected {
				m.publishCommandResult(commandCtx, cmd, ErrCommandBusy)
				m.releaseCommandAdmission(cmd)
			}
		}()
		defer func() {
			close(jobs)
			close(rejected)
			workers.Wait()
			close(commandDone)
		}()
		for {
			select {
			case <-commandCtx.Done():
				return
			case cmd, ok := <-sub.C:
				if !ok {
					return
				}
				if strings.TrimSpace(cmd.Type) == CommandResult {
					m.applyCommand(commandCtx, cmd)
					continue
				}
				if !m.admitCommand(cmd) {
					continue
				}
				select {
				case jobs <- cmd:
				default:
					select {
					case rejected <- cmd:
					case <-commandCtx.Done():
						m.releaseCommandAdmission(cmd)
						return
					}
				}
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
	pendingCommands := make([]*commandWaiter, 0, len(m.pendingCommands))
	for commandID, waiters := range m.pendingCommands {
		for pending := range waiters {
			pendingCommands = append(pendingCommands, pending)
		}
		delete(m.pendingCommands, commandID)
	}
	m.mu.Unlock()
	if commandCancel != nil {
		commandCancel()
	}
	for _, pending := range pendingCommands {
		pending.result <- ErrManagerClosed
	}
	releaseErr := m.releaseAllLocalRuns(ctx)
	controlErr := m.stopAllLocalControls(ctx)
	var commandErr error
	if commandDone != nil {
		select {
		case <-commandDone:
		case <-ctx.Done():
			commandErr = ctx.Err()
		}
	}
	var backendErr error
	if m.backend != nil {
		backendErr = m.backend.Close()
	}
	return errors.Join(releaseErr, controlErr, commandErr, backendErr)
}

func (m *Manager) StartRun(ctx context.Context, botID, sessionID, streamID string, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	_, err := m.StartRunHandle(ctx, botID, sessionID, streamID, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunHandle(ctx context.Context, botID, sessionID, streamID string, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	return m.StartRunWithAdmissionHandle(ctx, botID, sessionID, streamID, RunAdmissionView{}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithOperation(ctx context.Context, botID, sessionID, streamID string, operation *RunOperationView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	_, err := m.StartRunWithOperationHandle(ctx, botID, sessionID, streamID, operation, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunWithOperationHandle(ctx context.Context, botID, sessionID, streamID string, operation *RunOperationView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	return m.StartRunWithAdmissionHandle(ctx, botID, sessionID, streamID, RunAdmissionView{Operation: operation}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithAdmission(ctx context.Context, botID, sessionID, streamID string, admission RunAdmissionView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	_, err := m.StartRunWithAdmissionHandle(ctx, botID, sessionID, streamID, admission, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunWithAdmissionHandle(ctx context.Context, botID, sessionID, streamID string, admission RunAdmissionView, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	return m.StartRunWithAdmissionBuilderHandle(ctx, botID, sessionID, streamID, func(context.Context, RunHandle) (RunAdmissionView, error) {
		return admission, nil
	}, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithOperationBuilder(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context) (*RunOperationView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	if builder == nil {
		return errors.New("runtime operation builder is required")
	}
	_, err := m.StartRunWithOperationBuilderHandle(ctx, botID, sessionID, streamID, func(ctx context.Context, _ RunHandle) (*RunOperationView, error) {
		return builder(ctx)
	}, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunWithOperationBuilderHandle(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context, RunHandle) (*RunOperationView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	if builder == nil {
		return RunHandle{}, errors.New("runtime operation builder is required")
	}
	return m.StartRunWithAdmissionBuilderHandle(ctx, botID, sessionID, streamID, func(ctx context.Context, handle RunHandle) (RunAdmissionView, error) {
		operation, err := builder(ctx, handle)
		return RunAdmissionView{Operation: operation}, err
	}, abortCh, cancel, injectCh)
}

// StartRunWithAdmissionBuilder reserves the cross-server run before executing
// builder, then publishes the running view only after the canonical request
// turn and optional replacement operation are ready.
func (m *Manager) StartRunWithAdmissionBuilder(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context) (RunAdmissionView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	if builder == nil {
		return errors.New("runtime admission builder is required")
	}
	_, err := m.StartRunWithAdmissionBuilderHandle(ctx, botID, sessionID, streamID, func(ctx context.Context, _ RunHandle) (RunAdmissionView, error) {
		return builder(ctx)
	}, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunWithAdmissionBuilderAndOwnership(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context) (RunAdmissionView, error), ownershipCancel context.CancelCauseFunc, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	if builder == nil {
		if ownershipCancel != nil {
			ownershipCancel(ErrRunOwnershipLost)
		}
		return errors.New("runtime admission builder is required")
	}
	_, err := m.StartRunWithAdmissionBuilderAndOwnershipHandle(ctx, botID, sessionID, streamID, func(ctx context.Context, _ RunHandle) (RunAdmissionView, error) {
		return builder(ctx)
	}, ownershipCancel, abortCh, cancel, injectCh)
	return err
}

func (m *Manager) StartRunWithAdmissionBuilderHandle(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context, RunHandle) (RunAdmissionView, error), abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	return m.startRunWithAdmissionBuilder(ctx, botID, sessionID, streamID, builder, nil, abortCh, cancel, injectCh)
}

func (m *Manager) StartRunWithAdmissionBuilderAndOwnershipHandle(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context, RunHandle) (RunAdmissionView, error), ownershipCancel context.CancelCauseFunc, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	return m.startRunWithAdmissionBuilder(ctx, botID, sessionID, streamID, builder, ownershipCancel, abortCh, cancel, injectCh)
}

func (m *Manager) startRunWithAdmissionBuilder(ctx context.Context, botID, sessionID, streamID string, builder func(context.Context, RunHandle) (RunAdmissionView, error), ownershipCancel context.CancelCauseFunc, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) (RunHandle, error) {
	if m == nil || m.backend == nil {
		if ownershipCancel != nil {
			ownershipCancel(ErrRunOwnershipLost)
		}
		return RunHandle{}, nil
	}
	botID = strings.TrimSpace(botID)
	sessionID = strings.TrimSpace(sessionID)
	streamID = strings.TrimSpace(streamID)
	if botID == "" || sessionID == "" || streamID == "" {
		if ownershipCancel != nil {
			ownershipCancel(ErrRunOwnershipLost)
		}
		return RunHandle{}, errors.New("bot_id, session_id, and stream_id are required")
	}
	if builder == nil {
		if ownershipCancel != nil {
			ownershipCancel(ErrRunOwnershipLost)
		}
		return RunHandle{}, errors.New("runtime admission builder is required")
	}

	runGeneration := m.newGeneration()
	handle := RunHandle{BotID: botID, SessionID: sessionID, StreamID: streamID, Generation: runGeneration}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.WithoutCancel(ctx))
	ctrl := &runControl{
		botID:           botID,
		sessionID:       sessionID,
		streamID:        streamID,
		generation:      runGeneration,
		abortCh:         abortCh,
		cancel:          cancel,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		injectCh:        injectCh,
		converter:       conversation.NewUIMessageStreamConverter(),
		leaseChanged:    make(chan struct{}, 1),
		ready:           make(chan struct{}),
		ownershipCancel: ownershipCancel,
	}
	defer ctrl.markReady()
	m.mu.Lock()
	select {
	case <-m.closeCh:
		m.mu.Unlock()
		ctrl.revokeOwnership(ErrRunOwnershipLost)
		return RunHandle{}, ErrManagerClosed
	default:
	}
	if _, exists := m.controls[streamID]; exists {
		m.mu.Unlock()
		ctrl.revokeOwnership(ErrRunOwnershipLost)
		return RunHandle{}, fmt.Errorf("stream_id %q is already owned by this runtime manager", streamID)
	}
	m.controls[streamID] = ctrl
	m.mu.Unlock()

	localLeaseStarted := time.Now()
	now, err := m.backend.Now(ctx)
	if err != nil {
		m.removeLocalControl(streamID, ctrl)
		return RunHandle{}, fmt.Errorf("load runtime backend time: %w", err)
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
		ctrl.setLeaseValidUntil(localLeaseStarted.Add(m.ownerLeaseTTL))
	}
	epoch := m.newEpoch()
	var expiredRef StreamRef
	claim := func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		if snapshot.CurrentRunView != nil && isActiveRunStatus(snapshot.CurrentRunView.Status) && snapshot.CurrentRunView.StreamID != streamID {
			if m.distributed == nil || !m.markLostIfExpired(&snapshot, now) {
				return snapshot, false, fmt.Errorf("session %q already has an active runtime run", sessionID)
			}
			expiredRef = streamRefForRun(snapshot.BotID, snapshot.SessionID, snapshot.CurrentRunView)
		}
		snapshot.BotID = botID
		snapshot.SessionID = sessionID
		if strings.TrimSpace(snapshot.Epoch) == "" {
			snapshot.Epoch = epoch
		}
		snapshot.Seq++
		snapshot.Queue = nonNilQueue(snapshot.Queue)
		snapshot.UpdatedAt = now
		snapshot.CurrentRunView = &CurrentRunView{
			StreamID:            streamID,
			Generation:          runGeneration,
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
			BotID: botID, SessionID: sessionID, StreamID: streamID, OwnerID: m.ownerID, Generation: runGeneration,
		}, claim)
	} else {
		claimedSnapshot, changed, err = m.backend.Update(ctx, key, claim)
	}
	if err != nil {
		m.removeLocalControl(streamID, ctrl)
		return RunHandle{}, err
	}
	if !changed {
		m.removeLocalControl(streamID, ctrl)
		return RunHandle{}, nil
	}
	if err := m.publishRuntimeDelta(context.WithoutCancel(ctx), claimedSnapshot, streamID, RuntimeDelta{CurrentRunView: claimedSnapshot.CurrentRunView}); err != nil {
		m.logger.Warn("publish admitting runtime checkpoint failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", streamID))
	}
	if expiredRef.StreamID != "" && m.distributed != nil {
		_, _ = m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), expiredRef)
	}
	if m.distributed != nil {
		localRenewalStarted := time.Now()
		confirmCtx, confirmCancel := context.WithDeadline(context.WithoutCancel(ctx), ctrl.leaseDeadline())
		renewedAt, renewErr := m.backend.Now(confirmCtx)
		if renewErr == nil {
			renewErr = m.distributed.RenewLease(confirmCtx, key, streamID, m.ownerID, runGeneration, renewedAt, renewedAt.Add(m.ownerLeaseTTL))
		}
		confirmCancel()
		if renewErr != nil {
			ctrl.markReady()
			if m.isClosed() {
				m.removeLocalControl(streamID, ctrl)
				return RunHandle{}, ErrManagerClosed
			}
			if errors.Is(renewErr, ErrRunOwnershipLost) || !ctrl.leaseIsValidAt(time.Now()) {
				m.removeLocalControl(streamID, ctrl)
				renewErr = ErrRunOwnershipLost
			} else {
				_ = m.FinishRun(context.WithoutCancel(ctx), handle, RunStatusErrored, renewErr.Error())
			}
			return RunHandle{}, fmt.Errorf("confirm runtime owner lease: %w", renewErr)
		}
		deadline := localRenewalStarted.Add(m.ownerLeaseTTL)
		if !time.Now().Before(deadline) {
			ctrl.markReady()
			m.removeLocalControl(streamID, ctrl)
			if m.isClosed() {
				return RunHandle{}, ErrManagerClosed
			}
			return RunHandle{}, fmt.Errorf("confirm runtime owner lease: %w", ErrRunOwnershipLost)
		}
		ctrl.setLeaseValidUntil(deadline)
		m.startLeaseRenewal(context.WithoutCancel(ctx), ctrl)
	}
	admission, err := builder(ctx, handle)
	if err != nil {
		ctrl.markReady()
		status := RunStatusErrored
		message := err.Error()
		if errors.Is(err, context.Canceled) {
			status = RunStatusAborted
			message = ""
		}
		_ = m.FinishRun(context.WithoutCancel(ctx), handle, status, message)
		return RunHandle{}, err
	}
	if m.isClosed() {
		return RunHandle{}, ErrManagerClosed
	}
	if m.localControlForHandle(handle) != ctrl {
		return RunHandle{}, ErrRunOwnershipLost
	}
	admission, err = normalizeRunAdmission(admission)
	if err != nil {
		ctrl.markReady()
		_ = m.FinishRun(context.WithoutCancel(ctx), handle, RunStatusErrored, err.Error())
		return RunHandle{}, err
	}
	_, changed, err = m.updateActiveAndPublish(context.WithoutCancel(ctx), handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		if m.isClosed() {
			return snapshot, false, ErrManagerClosed
		}
		if m.localControlForHandle(handle) != ctrl {
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
			return RunHandle{}, err
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
		_ = m.FinishRun(context.WithoutCancel(ctx), handle, status, message)
		return RunHandle{}, err
	}
	ctrl.markReady()
	return handle, nil
}

func (m *Manager) FinishRun(ctx context.Context, handle RunHandle, status, message string) error {
	if m == nil || m.backend == nil {
		return nil
	}
	handle = handle.normalized()
	if !handle.valid() {
		return ErrRunOwnershipLost
	}
	ctrl := m.localControlForHandle(handle)
	if ctrl != nil {
		ctrl.stopCommands()
	}
	status = strings.TrimSpace(status)
	finishMessage := strings.TrimSpace(message)
	changed, err := m.finishRunState(ctx, handle, status, finishMessage)
	if err == nil || changed {
		m.cleanupFinishedRun(context.WithoutCancel(ctx), handle)
		return err
	}
	if errors.Is(err, ErrRunOwnershipLost) {
		snapshot, ok, loadErr := m.backend.Load(context.WithoutCancel(ctx), handle.key())
		if loadErr == nil && ok && runMatchesHandle(snapshot.CurrentRunView, handle) && !isActiveRunStatus(snapshot.CurrentRunView.Status) {
			m.cleanupFinishedRun(context.WithoutCancel(ctx), handle)
			return nil
		}
		m.forgetLocalControlForHandle(context.WithoutCancel(ctx), handle)
		return err
	}
	if ctrl != nil && m.localControlForHandle(handle) == ctrl {
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

func (m *Manager) finishRunState(ctx context.Context, handle RunHandle, status, finishMessage string) (bool, error) {
	admissionTerminal := false
	_, changed, err := m.releaseActiveAndPublish(ctx, handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
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
		changed, err := m.finishRunState(ctx, ctrl.handle(), status, message)
		if err == nil || changed {
			if err != nil {
				m.logger.Warn("publish runtime finish failed after state commit", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
			}
			m.cleanupFinishedRun(ctx, ctrl.handle())
			return
		}
		if errors.Is(err, ErrRunOwnershipLost) {
			m.forgetLocalControlForHandle(ctx, ctrl.handle())
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

func (m *Manager) cleanupFinishedRun(ctx context.Context, handle RunHandle) {
	handle = handle.normalized()
	ctrl := m.localControlForHandle(handle)
	ref := m.streamRefForControl(ctrl)
	m.forgetLocalControlForHandle(ctx, handle)
	if m.distributed == nil {
		return
	}
	if ref.StreamID == "" {
		ref = StreamRef{BotID: handle.BotID, SessionID: handle.SessionID, StreamID: handle.StreamID, OwnerID: m.ownerID, Generation: handle.Generation}
	}
	if _, err := m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), ref); err != nil {
		m.logger.Warn("delete finished runtime stream reference failed", slog.Any("error", err), slog.String("stream_id", handle.StreamID))
	}
}

func (m *Manager) HandleAgentEvent(ctx context.Context, handle RunHandle, event agentpkg.StreamEvent) ([]conversation.UIMessage, error) {
	if m == nil || m.backend == nil {
		return nil, nil
	}
	handle = handle.normalized()
	if !handle.valid() {
		return nil, ErrRunOwnershipLost
	}
	ctrl := m.localControlForHandle(handle)
	if ctrl == nil {
		return nil, ErrRunOwnershipLost
	}
	if err := waitRunControlReady(ctx, ctrl); err != nil {
		return nil, err
	}
	if m.localControlForHandle(handle) != ctrl {
		return nil, ErrRunOwnershipLost
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

	_, changed, err := m.updateActiveAndPublish(ctx, handle, func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		run := snapshot.CurrentRunView
		if !runMatchesHandle(run, handle) || !m.runOwnerMatches(run) || !isEventAcceptingRunStatus(run.Status) {
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
	if !ok || strings.TrimSpace(snapshot.Epoch) == "" {
		now, nowErr := m.backend.Now(ctx)
		if nowErr != nil {
			return Snapshot{}, fmt.Errorf("load runtime backend time: %w", nowErr)
		}
		epoch := m.newEpoch()
		snapshot, _, err = m.backend.Update(ctx, key, func(snapshot Snapshot, ok bool) (Snapshot, bool, error) {
			if ok && strings.TrimSpace(snapshot.Epoch) != "" {
				return snapshot, false, nil
			}
			if !ok {
				snapshot = EmptySnapshot(key.BotID, key.SessionID)
			}
			snapshot.BotID = key.BotID
			snapshot.SessionID = key.SessionID
			snapshot.Epoch = epoch
			snapshot.UpdatedAt = now
			return snapshot, true, nil
		})
		if err != nil {
			return Snapshot{}, err
		}
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
		lostRef := streamRefForRun(snapshot.BotID, snapshot.SessionID, snapshot.CurrentRunView)
		updated, changed, err := m.updateAndPublish(ctx, key, lostRef.StreamID, func(current Snapshot, ok bool) (Snapshot, bool, error) {
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
		if changed && lostRef.StreamID != "" {
			_, _ = m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), lostRef)
			lostHandle := RunHandle{BotID: lostRef.BotID, SessionID: lostRef.SessionID, StreamID: lostRef.StreamID, Generation: lostRef.Generation}
			if ctrl := m.localControlForHandle(lostHandle); ctrl != nil {
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
		var lastEpoch string
		var lastSeq int64
		events := backendSub.C
		for {
			select {
			case <-m.closeCh:
				return
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
				eventEpoch := runtimeEventEpoch(event)
				if lastEpoch != "" && eventEpoch == "" {
					enqueueRuntimeEvent(out, Event{
						Type:      EventRuntimeDropped,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Epoch:     lastEpoch,
						Seq:       lastSeq,
						Message:   "runtime event is missing epoch",
					})
					continue
				}
				if lastEpoch != "" && eventEpoch != "" && eventEpoch != lastEpoch {
					resetSnapshot, err := m.Snapshot(subCtx, key.BotID, key.SessionID)
					if err != nil {
						enqueueRuntimeEvent(out, Event{
							Type:      EventRuntimeDropped,
							BotID:     key.BotID,
							SessionID: key.SessionID,
							Seq:       lastSeq,
							Epoch:     lastEpoch,
							Message:   "runtime epoch changed",
						})
						continue
					}
					event = Event{
						Type:      EventRuntimeSnapshot,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Epoch:     resetSnapshot.Epoch,
						Seq:       resetSnapshot.Seq,
						Snapshot:  &resetSnapshot,
					}
					eventEpoch = resetSnapshot.Epoch
				}
				if eventEpoch == lastEpoch && event.Seq < lastSeq {
					continue
				}
				selfContained := event.Delta != nil && event.Delta.CurrentRunView != nil
				if event.Type == EventRuntimeDelta && !selfContained && lastSeq > 0 && event.Seq > lastSeq+1 {
					enqueueRuntimeEvent(out, Event{
						Type:      EventRuntimeDropped,
						BotID:     key.BotID,
						SessionID: key.SessionID,
						Epoch:     lastEpoch,
						Seq:       lastSeq,
						Message:   "runtime delta sequence gap",
					})
				}
				if eventEpoch != "" {
					lastEpoch = eventEpoch
				}
				if event.Seq > 0 {
					lastSeq = event.Seq
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
				if snapshot.Epoch == lastEpoch && snapshot.Seq <= lastSeq {
					continue
				}
				lastEpoch = snapshot.Epoch
				lastSeq = snapshot.Seq
				enqueueRuntimeEvent(out, Event{
					Type:      EventRuntimeSnapshot,
					BotID:     key.BotID,
					SessionID: key.SessionID,
					Epoch:     snapshot.Epoch,
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

func (m *Manager) updateActiveAndPublish(ctx context.Context, handle RunHandle, update ActiveRunUpdate, buildDelta func(Snapshot) RuntimeDelta) (Snapshot, bool, error) {
	handle = handle.normalized()
	if !handle.valid() {
		return Snapshot{}, false, ErrRunOwnershipLost
	}
	key := handle.key()
	streamID := handle.StreamID
	var snapshot Snapshot
	var changed bool
	var err error
	if m.distributed != nil {
		snapshot, changed, err = m.distributed.UpdateActiveRun(ctx, key, streamID, handle.Generation, update)
	} else {
		snapshot, changed, err = m.backend.Update(ctx, key, func(snapshot Snapshot, ok bool) (Snapshot, bool, error) {
			if !ok || !runMatchesHandle(snapshot.CurrentRunView, handle) || !isActiveRunStatus(snapshot.CurrentRunView.Status) {
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

func (m *Manager) releaseActiveAndPublish(ctx context.Context, handle RunHandle, update ActiveRunUpdate, buildDelta func(Snapshot) RuntimeDelta) (Snapshot, bool, error) {
	handle = handle.normalized()
	if m.distributed == nil {
		return m.updateActiveAndPublish(ctx, handle, update, buildDelta)
	}
	if !handle.valid() {
		return Snapshot{}, false, ErrRunOwnershipLost
	}
	ref := StreamRef{
		BotID: handle.BotID, SessionID: handle.SessionID, StreamID: handle.StreamID,
		OwnerID: m.ownerID, Generation: handle.Generation,
	}
	snapshot, changed, err := m.distributed.ReleaseRun(ctx, handle.key(), ref, update)
	if err != nil || !changed {
		return snapshot, changed, err
	}
	delta := RuntimeDelta{}
	if buildDelta != nil {
		delta = buildDelta(snapshot)
	}
	if err := m.publishRuntimeDelta(ctx, snapshot, handle.StreamID, delta); err != nil {
		m.logger.Warn("publish runtime release delta failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", handle.StreamID))
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
		Epoch:     snapshot.Epoch,
		StreamID:  eventStreamID,
		Seq:       snapshot.Seq,
		UpdatedAt: &updatedAt,
		Delta:     &delta,
	})
}
