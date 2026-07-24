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
	runState       runStateBackend
	distributed    DistributedBackend
	ownerID        string
	stateTTL       time.Duration
	ownerLeaseTTL  time.Duration
	commandAckTTL  time.Duration
	abortGraceTTL  time.Duration
	commandWorkers int
	logger         *slog.Logger
	newEpoch       func() string
	newGeneration  func() string

	mu                     sync.Mutex
	controls               map[runControlKey]*runControl
	commandHandler         func(context.Context, Command) error
	commandReconciler      func(context.Context, Command) (bool, error)
	pendingCommands        map[string]map[*commandWaiter]struct{}
	inflightCommandTargets map[string]struct{}
	commandExecutions      map[string]chan struct{}
	admittedCommands       map[string]struct{}

	commandCancel       context.CancelFunc
	commandDone         chan struct{}
	subscriptionsMu     sync.Mutex
	subscriptionsClosed bool
	subscriptionsWG     sync.WaitGroup
	closeCh             chan struct{}
	closeOnce           sync.Once
	shutdownOnce        sync.Once
	shutdownDone        chan struct{}
	shutdownErr         error
	backgroundWG        sync.WaitGroup
}

type commandWaiter struct {
	result      chan error
	payloadHash string
}

type runControl struct {
	botID             string
	sessionID         string
	streamID          string
	generation        string
	abortCh           chan<- struct{}
	cancel            context.CancelFunc
	admissionCancel   context.CancelFunc
	lifecycleCtx      context.Context
	lifecycleCancel   context.CancelFunc
	injectCh          chan<- conversation.InjectMessage
	injectMu          sync.Mutex
	injectClosed      bool
	converter         *conversation.UIMessageStreamConverter
	leaseStop         func()
	leaseDone         chan struct{}
	leaseLifecycleMu  sync.Mutex
	leaseMu           sync.RWMutex
	leaseValidUntil   time.Time
	leaseChanged      chan struct{}
	ready             chan struct{}
	readyOnce         sync.Once
	abortStateMu      sync.Mutex
	claimEstablished  bool
	admissionComplete bool
	abortRequested    bool
	abortFinalizing   bool
	abortGraceOnce    sync.Once
	abortGraceMu      sync.Mutex
	abortGraceTimer   *time.Timer
	abortGraceStopped bool
	finalizeRetryOnce sync.Once
	ownershipCancel   context.CancelCauseFunc
	ownershipOnce     sync.Once
}

type runControlKey struct {
	botID     string
	sessionID string
	streamID  string
}

func scopedRunControlKey(botID, sessionID, streamID string) runControlKey {
	return runControlKey{
		botID:     strings.TrimSpace(botID),
		sessionID: strings.TrimSpace(sessionID),
		streamID:  strings.TrimSpace(streamID),
	}
}

func (c *runControl) key() runControlKey {
	if c == nil {
		return runControlKey{}
	}
	return scopedRunControlKey(c.botID, c.sessionID, c.streamID)
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
	AbortGraceTimeout      time.Duration
	CommandWorkerLimit     int
	Logger                 *slog.Logger
	EpochGenerator         func() string
	RunGenerationGenerator func() string
}

const (
	defaultCommandAckTTL        = 2 * time.Second
	defaultAbortGraceTimeout    = 30 * time.Second
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
	abortGraceTTL := opts.AbortGraceTimeout
	if abortGraceTTL <= 0 {
		abortGraceTTL = defaultAbortGraceTimeout
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
	var runState runStateBackend = localRunStateBackend{backend: backend}
	if distributed != nil {
		runState = distributedRunStateBackend{backend: distributed}
	}
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
		runState:               runState,
		distributed:            distributed,
		ownerID:                ownerID,
		stateTTL:               stateTTL,
		ownerLeaseTTL:          leaseTTL,
		commandAckTTL:          commandAckTTL,
		abortGraceTTL:          abortGraceTTL,
		commandWorkers:         commandWorkers,
		logger:                 log.With(slog.String("component", "session_runtime")),
		newEpoch:               newEpoch,
		newGeneration:          newGeneration,
		controls:               make(map[runControlKey]*runControl),
		pendingCommands:        make(map[string]map[*commandWaiter]struct{}),
		inflightCommandTargets: make(map[string]struct{}),
		commandExecutions:      make(map[string]chan struct{}),
		admittedCommands:       make(map[string]struct{}),
		closeCh:                make(chan struct{}),
		shutdownDone:           make(chan struct{}),
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

// SetCommandReconciler installs a read-only domain result checker. Unlike the
// owner-local command handler, it may run on any server after the owner or its
// local control disappears.
func (m *Manager) SetCommandReconciler(reconciler func(context.Context, Command) (bool, error)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.commandReconciler = reconciler
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
	var reaper ExpiredRunBackend
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
	if candidate, ok := m.distributed.(ExpiredRunBackend); ok {
		reaper = candidate
		m.backgroundWG.Add(1)
	}
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
	if reaper != nil {
		go func() {
			defer m.backgroundWG.Done()
			m.runExpiredRunReaper(commandCtx, reaper)
		}()
	}
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
	shutdownCtx := context.WithoutCancel(ctx)
	m.shutdownOnce.Do(func() {
		go func() {
			m.shutdownErr = m.shutdown(shutdownCtx)
			close(m.shutdownDone)
		}()
	})
	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	default:
	}
	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) shutdown(ctx context.Context) error {
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
	if commandDone != nil {
		<-commandDone
	}
	m.backgroundWG.Wait()
	m.subscriptionsMu.Lock()
	m.subscriptionsClosed = true
	m.subscriptionsMu.Unlock()
	var backendErr error
	if m.backend != nil {
		backendErr = m.backend.Close()
	}
	m.subscriptionsWG.Wait()
	return errors.Join(releaseErr, controlErr, backendErr)
}

const expiredRunReaperBatchSize = 128

func (m *Manager) runExpiredRunReaper(ctx context.Context, reaper ExpiredRunBackend) {
	interval := runtimeReconcileInterval(m.ownerLeaseTTL)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			keys, err := reaper.ListExpiredRunKeys(ctx, expiredRunReaperBatchSize)
			if err != nil {
				if ctx.Err() == nil {
					m.logger.Warn("list expired runtime runs failed", slog.Any("error", err))
				}
				continue
			}
			for _, key := range keys {
				if _, err := m.Snapshot(ctx, key.BotID, key.SessionID); err != nil && ctx.Err() == nil {
					m.logger.Warn("reap expired runtime run failed", slog.Any("error", err), slog.String("session_id", key.SessionID))
				}
			}
		}
	}
}

func (m *Manager) StartRun(ctx context.Context, botID, sessionID, streamID string, abortCh chan<- struct{}, cancel context.CancelFunc, injectCh chan<- conversation.InjectMessage) error {
	_, err := m.StartRunWithOptions(ctx, RunStartOptions{
		BotID: botID, SessionID: sessionID, StreamID: streamID,
		AbortCh: abortCh, Cancel: cancel, InjectCh: injectCh,
	})
	return err
}

// StartRunWithOptions reserves the backend run before executing the optional
// admission builder, then publishes the running view only after admission is
// ready. This is the single advanced run-start entry point.
func (m *Manager) StartRunWithOptions(ctx context.Context, options RunStartOptions) (RunHandle, error) {
	if options.AdmissionBuilder != nil && (options.Admission.RequestUserTurn != nil || options.Admission.Operation != nil) {
		if options.OwnershipCancel != nil {
			options.OwnershipCancel(ErrRunOwnershipLost)
		}
		return RunHandle{}, errors.New("runtime admission and admission builder cannot both be set")
	}
	builder := options.AdmissionBuilder
	if builder == nil {
		builder = func(context.Context, RunHandle) (RunAdmissionView, error) {
			return options.Admission, nil
		}
	}
	return m.startRunWithAdmissionBuilder(
		ctx,
		options.BotID,
		options.SessionID,
		options.StreamID,
		builder,
		options.OwnershipCancel,
		options.AbortCh,
		options.Cancel,
		options.InjectCh,
	)
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
	if err := ctx.Err(); err != nil {
		if ownershipCancel != nil {
			ownershipCancel(ErrRunOwnershipLost)
		}
		return RunHandle{}, err
	}

	admissionCtx, admissionCancel := context.WithCancel(ctx)
	defer admissionCancel()
	ctx = admissionCtx

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
		admissionCancel: admissionCancel,
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
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		ctrl.revokeOwnership(ErrRunOwnershipLost)
		return RunHandle{}, err
	}
	select {
	case <-m.closeCh:
		m.mu.Unlock()
		ctrl.revokeOwnership(ErrRunOwnershipLost)
		return RunHandle{}, ErrManagerClosed
	default:
	}
	controlKey := scopedRunControlKey(botID, sessionID, streamID)
	if _, exists := m.controls[controlKey]; exists {
		m.mu.Unlock()
		ctrl.revokeOwnership(ErrRunOwnershipLost)
		return RunHandle{}, fmt.Errorf("stream_id %q is already owned by this runtime manager", streamID)
	}
	m.controls[controlKey] = ctrl
	m.mu.Unlock()

	localLeaseStarted := time.Now()
	key := Key{BotID: botID, SessionID: sessionID}
	ownerID := ""
	if m.distributed != nil {
		ownerID = m.ownerID
		// The backend claim below is authoritative, but a local deadline must
		// exist before it returns so an abort can cancel blocked admission.
		ctrl.setLeaseValidUntil(localLeaseStarted.Add(m.ownerLeaseTTL))
	}
	epoch := m.newEpoch()
	var expiredRef StreamRef
	claim := func(snapshot Snapshot, now time.Time) (Snapshot, bool, error) {
		expiredRef = StreamRef{}
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
		var leaseExpiresAt *time.Time
		if m.distributed != nil {
			expiresAt := now.Add(m.ownerLeaseTTL)
			leaseExpiresAt = &expiresAt
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
	claimedSnapshot, changed, err := m.runState.StartRun(ctx, key, StreamRef{
		BotID: botID, SessionID: sessionID, StreamID: streamID, OwnerID: ownerID, Generation: runGeneration,
	}, claim)
	if err != nil {
		if ctrl.abortWasRequested() {
			reconcileErr := m.reconcileCanceledRunClaim(context.WithoutCancel(ctx), ctrl)
			if reconcileErr == nil {
				return RunHandle{}, context.Canceled
			}
			m.removeLocalControl(streamID, ctrl)
			return RunHandle{}, errors.Join(err, reconcileErr)
		}
		m.removeLocalControl(streamID, ctrl)
		return RunHandle{}, err
	}
	if !changed {
		m.removeLocalControl(streamID, ctrl)
		return RunHandle{}, nil
	}
	abortRequested, abortOwner := ctrl.establishClaimForAbort()
	if abortRequested {
		if abortOwner {
			abortErr := m.abortClaimedAdmission(context.WithoutCancel(ctx), ctrl)
			if abortErr != nil {
				return RunHandle{}, abortErr
			}
		}
		return RunHandle{}, context.Canceled
	}
	if err := m.publishRuntimeDelta(context.WithoutCancel(ctx), claimedSnapshot, streamID, RuntimeDelta{CurrentRunView: claimedSnapshot.CurrentRunView}); err != nil {
		m.logger.Warn("publish admitting runtime checkpoint failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", streamID))
	}
	if expiredRef.StreamID != "" && m.distributed != nil {
		_, _ = m.distributed.DeleteStreamRef(context.WithoutCancel(ctx), expiredRef)
	}
	if m.distributed != nil {
		localRenewalStarted := time.Now()
		deadline := localRenewalStarted.Add(m.ownerLeaseTTL)
		ctrl.setLeaseValidUntil(deadline)
		confirmCtx, confirmCancel := context.WithDeadline(context.WithoutCancel(ctx), deadline)
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
	if !ctrl.completeAdmissionForAbort() {
		return RunHandle{}, context.Canceled
	}
	ctrl.markReady()
	return handle, nil
}

func (m *Manager) reconcileCanceledRunClaim(ctx context.Context, ctrl *runControl) error {
	if ctrl == nil {
		return nil
	}
	snapshot, ok, err := m.backend.Load(ctx, Key{BotID: ctrl.botID, SessionID: ctrl.sessionID})
	if err != nil {
		return fmt.Errorf("reconcile canceled runtime claim: %w", err)
	}
	if !ok || !runMatchesHandle(snapshot.CurrentRunView, ctrl.handle()) {
		m.removeLocalControl(ctrl.streamID, ctrl)
		return nil
	}
	if !isActiveRunStatus(snapshot.CurrentRunView.Status) {
		m.cleanupFinishedRun(ctx, ctrl.handle())
		return nil
	}
	if ctrl.claimVisibleAndTakeAbort() {
		return m.abortClaimedAdmission(ctx, ctrl)
	}
	return nil
}

func (m *Manager) FinishRun(ctx context.Context, handle RunHandle, status, message string) error {
	return m.finalizeRun(ctx, handle, runFinalization{
		Status: strings.TrimSpace(status),
		Error:  strings.TrimSpace(message),
	}, false)
}

type runFinalization struct {
	Status           string
	ErrorCode        string
	Error            string
	Messages         []conversation.UIMessage
	HistoryCommitted bool
	CanonicalReady   bool
}

func (m *Manager) FinalizeAgentEvent(ctx context.Context, handle RunHandle, event agentpkg.StreamEvent, canonicalReady bool, finalizationError string) ([]conversation.UIMessage, error) {
	if event.Type != agentpkg.EventAgentEnd && event.Type != agentpkg.EventAgentAbort {
		return nil, errors.New("terminal agent event is required")
	}
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
	messages := ctrl.converter.ConvertTerminalMessages(event.Messages)
	status := RunStatusCompleted
	if event.Type == agentpkg.EventAgentAbort {
		status = RunStatusAborted
	}
	finalizationError = strings.TrimSpace(finalizationError)
	if finalizationError != "" {
		status = RunStatusErrored
	}
	outcome := runFinalization{
		Status:           status,
		Error:            finalizationError,
		Messages:         messages,
		HistoryCommitted: event.HistoryCommitted,
		CanonicalReady:   canonicalReady && event.HistoryCommitted,
	}
	if err := m.finalizeRun(ctx, handle, outcome, true); err != nil {
		return messages, err
	}
	return messages, nil
}

func (m *Manager) finalizeRun(ctx context.Context, handle RunHandle, outcome runFinalization, terminalCommit bool) error {
	if m == nil || m.backend == nil {
		return nil
	}
	handle = handle.normalized()
	if !handle.valid() {
		return ErrRunOwnershipLost
	}
	outcome.Status = strings.ToLower(strings.TrimSpace(outcome.Status))
	outcome = sanitizeRunFinalization(outcome)
	if !isValidFinalRunStatus(outcome.Status) {
		return fmt.Errorf("invalid runtime final status %q", outcome.Status)
	}
	outcome.CanonicalReady = outcome.CanonicalReady && outcome.HistoryCommitted
	ctrl := m.localControlForHandle(handle)
	if ctrl != nil {
		ctrl.stopCommands()
	}
	changed, err := m.finalizeRunState(ctx, handle, outcome)
	if err == nil || changed {
		m.cleanupFinishedRun(context.WithoutCancel(ctx), handle)
		return err
	}
	if errors.Is(err, ErrRunOwnershipLost) {
		committed, loadErr := m.finalizationCommitted(context.WithoutCancel(ctx), handle, outcome)
		if loadErr == nil && committed {
			m.cleanupFinishedRun(context.WithoutCancel(ctx), handle)
			return nil
		}
		m.forgetLocalControlForHandle(context.WithoutCancel(ctx), handle)
		return err
	}
	if ctrl != nil && m.localControlForHandle(handle) == ctrl {
		retryCtx := context.WithoutCancel(ctx)
		clonedMessages, cloneErr := cloneUIMessages(outcome.Messages)
		if cloneErr == nil {
			outcome.Messages = clonedMessages
		}
		ctrl.finalizeRetryOnce.Do(func() {
			go m.retryFinalizeRun(retryCtx, ctrl, outcome)
		})
		if terminalCommit {
			return errors.Join(ErrTerminalCommitPending, err)
		}
	}
	return err
}

const steerRunFinishedError = runtimeTargetInactiveMessage

func rejectPendingSteerOnRunFinish(run *CurrentRunView, now time.Time) {
	if run == nil || run.Steer == nil || !isPendingSteerStatus(run.Steer.Status) {
		return
	}
	run.Steer.Status = SteerStatusRejected
	setRuntimeSteerError(run.Steer, RuntimeErrorCodeTargetInactive)
	run.Steer.UpdatedAt = now
}

func (m *Manager) finishRunState(ctx context.Context, handle RunHandle, status, finishMessage string) (bool, error) {
	return m.finalizeRunState(ctx, handle, runFinalization{Status: status, Error: finishMessage})
}

func (m *Manager) finalizeRunState(ctx context.Context, handle RunHandle, outcome runFinalization) (bool, error) {
	outcome = sanitizeRunFinalization(outcome)
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
		finalStatus := strings.TrimSpace(outcome.Status)
		if finalStatus == "" {
			finalStatus = RunStatusCompleted
			if strings.EqualFold(snapshot.CurrentRunView.Status, RunStatusAborting) {
				finalStatus = RunStatusAborted
			} else if strings.TrimSpace(snapshot.CurrentRunView.Error) != "" {
				finalStatus = RunStatusErrored
			}
		}
		if finalStatus == RunStatusCompleted && strings.EqualFold(run.Status, RunStatusAborting) {
			finalStatus = RunStatusAborted
		}
		if strings.TrimSpace(run.Error) != "" && outcome.Error == "" && finalStatus != RunStatusLost {
			finalStatus = RunStatusErrored
		}
		snapshot.Seq++
		snapshot.UpdatedAt = now
		for _, message := range outcome.Messages {
			run.Messages = upsertUIMessage(run.Messages, message)
		}
		run.Status = finalStatus
		run.UpdatedAt = now
		run.HistoryCommitted = run.HistoryCommitted || outcome.HistoryCommitted
		run.CanonicalReady = run.CanonicalReady || outcome.CanonicalReady
		switch {
		case outcome.Error != "":
			run.ErrorCode = outcome.ErrorCode
			run.Error = outcome.Error
		case finalStatus == RunStatusCompleted || finalStatus == RunStatusAborted:
			clearRuntimeRunError(run)
		}
		run.OwnerLeaseExpiresAt = nil
		rejectPendingSteerOnRunFinish(run, now)
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		if admissionTerminal {
			return RuntimeDelta{CurrentRunView: snapshot.CurrentRunView}
		}
		delta := runtimeRunPatch(snapshot, true, true, true, m.distributed != nil)
		delta.MessageUpserts = append([]conversation.UIMessage(nil), outcome.Messages...)
		return delta
	})
	return changed, err
}

func (m *Manager) finalizationCommitted(ctx context.Context, handle RunHandle, outcome runFinalization) (bool, error) {
	snapshot, ok, err := m.backend.Load(ctx, handle.key())
	if err != nil || !ok || !runMatchesHandle(snapshot.CurrentRunView, handle) {
		return false, err
	}
	run := snapshot.CurrentRunView
	if isActiveRunStatus(run.Status) || !finalRunStatusMatches(outcome.Status, run.Status) {
		return false, nil
	}
	if outcome.HistoryCommitted && !run.HistoryCommitted {
		return false, nil
	}
	if outcome.CanonicalReady && !run.CanonicalReady {
		return false, nil
	}
	if outcome.Error != "" && (run.ErrorCode != outcome.ErrorCode || run.Error != outcome.Error) {
		return false, nil
	}
	for _, expected := range outcome.Messages {
		found := false
		for _, actual := range run.Messages {
			equal, equalErr := equalRuntimeJSON(actual, expected)
			if equalErr != nil {
				return false, equalErr
			}
			if equal {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func isValidFinalRunStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "", RunStatusCompleted, RunStatusAborted, RunStatusErrored, RunStatusLost:
		return true
	default:
		return false
	}
}

func finalRunStatusMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" {
		return !isActiveRunStatus(actual)
	}
	if expected == RunStatusCompleted && actual == RunStatusAborted {
		return true
	}
	return expected == actual
}

func (m *Manager) retryFinalizeRun(ctx context.Context, ctrl *runControl, outcome runFinalization) {
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
		if m.localControlForHandle(ctrl.handle()) != ctrl {
			return
		}
		changed, err := m.finalizeRunState(ctx, ctrl.handle(), outcome)
		if err == nil || changed {
			if err != nil {
				m.logger.Warn("publish runtime finish failed after state commit", slog.Any("error", err), slog.String("stream_id", ctrl.streamID))
			}
			m.cleanupFinishedRun(ctx, ctrl.handle())
			return
		}
		if errors.Is(err, ErrRunOwnershipLost) {
			committed, loadErr := m.finalizationCommitted(ctx, ctrl.handle(), outcome)
			if loadErr == nil && committed {
				m.cleanupFinishedRun(ctx, ctrl.handle())
			} else {
				m.forgetLocalControlForHandle(ctx, ctrl.handle())
			}
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
	if event.Type == agentpkg.EventAgentEnd || event.Type == agentpkg.EventAgentAbort {
		return m.FinalizeAgentEvent(ctx, handle, event, event.HistoryCommitted, "")
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
	case agentpkg.EventError:
	default:
		messages = ctrl.converter.HandleEvent(conversation.UIStreamEventFromAgentEvent(event))
	}
	delta, visibleChange := runtimeDeltaForAgentEvent(event, messages)
	if !visibleChange {
		return messages, nil
	}
	if len(delta.MessageAppends) == 1 {
		handled, err := m.appendActiveMessageAndPublish(ctx, handle, delta.MessageAppends[0], delta)
		if handled {
			if err != nil {
				if errors.Is(err, ErrRunOwnershipLost) {
					return nil, err
				}
				return messages, err
			}
			return messages, nil
		}
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
		if event.Type == agentpkg.EventError {
			if detail := strings.TrimSpace(event.Error); detail != "" {
				m.logger.Error("agent runtime event failed", slog.String("error", detail), slog.String("stream_id", handle.StreamID))
			}
			setRuntimeRunError(run, RunStatusErrored)
		}
		return snapshot, true, nil
	}, func(snapshot Snapshot) RuntimeDelta {
		if event.Type == agentpkg.EventError {
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

func (m *Manager) appendActiveMessageAndPublish(ctx context.Context, handle RunHandle, messageAppend RuntimeMessageAppend, delta RuntimeDelta) (bool, error) {
	appender, ok := m.distributed.(StreamingDeltaBackend)
	if !ok {
		return false, nil
	}
	revision, applied, err := appender.AppendActiveRunMessage(ctx, handle.key(), StreamRef{
		BotID: handle.BotID, SessionID: handle.SessionID, StreamID: handle.StreamID,
		OwnerID: m.ownerID, Generation: handle.Generation,
	}, messageAppend)
	if err != nil || !applied {
		return applied, err
	}
	snapshot := Snapshot{
		BotID: handle.BotID, SessionID: handle.SessionID,
		Epoch: revision.Epoch, Seq: revision.Seq, UpdatedAt: revision.UpdatedAt,
	}
	if err := m.publishRuntimeDelta(ctx, snapshot, handle.StreamID, delta); err != nil {
		m.logger.Warn("publish runtime message append failed; subscribers will reconcile from snapshot", slog.Any("error", err), slog.String("stream_id", handle.StreamID))
	}
	return true, nil
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
			sanitizeSnapshotErrors(&snapshot)
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
	sanitizeSnapshotErrors(&snapshot)
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
	baseline, err := m.Snapshot(subCtx, key.BotID, key.SessionID)
	if err != nil {
		backendSub.Close()
		cancel()
		return Subscription{}, err
	}

	out := make(chan Event, 64)
	out <- Event{
		Type:      EventRuntimeSnapshot,
		BotID:     key.BotID,
		SessionID: key.SessionID,
		Epoch:     baseline.Epoch,
		Seq:       baseline.Seq,
		Snapshot:  &baseline,
	}
	m.subscriptionsMu.Lock()
	if m.subscriptionsClosed || m.isClosed() {
		m.subscriptionsMu.Unlock()
		backendSub.Close()
		cancel()
		return Subscription{}, ErrManagerClosed
	}
	m.subscriptionsWG.Add(1)
	m.subscriptionsMu.Unlock()
	done := make(chan struct{})
	go func() {
		defer m.subscriptionsWG.Done()
		defer close(done)
		defer close(out)
		defer backendSub.Close()
		send := func(event Event) bool {
			publicEvent, err := sanitizeRuntimeEventErrors(event)
			if err != nil {
				m.logger.Warn("sanitize runtime subscription event failed", slog.Any("error", err), slog.String("session_id", key.SessionID))
				return false
			}
			select {
			case out <- publicEvent:
				return true
			case <-m.closeCh:
				return false
			case <-subCtx.Done():
				return false
			}
		}
		reconcileInterval := 2 * time.Second
		if m.distributed != nil {
			reconcileInterval = runtimeReconcileInterval(m.ownerLeaseTTL)
		}
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		lastEpoch := baseline.Epoch
		lastSeq := baseline.Seq
		terminalDrop := func(message string) {
			_ = send(Event{
				Type:      EventRuntimeDropped,
				BotID:     key.BotID,
				SessionID: key.SessionID,
				Epoch:     lastEpoch,
				Seq:       lastSeq,
				Message:   message,
			})
		}
		reconcile := func(observedEpoch string, observedSeq int64, reason string) bool {
			snapshot, err := m.Snapshot(subCtx, key.BotID, key.SessionID)
			if err != nil {
				if subCtx.Err() == nil {
					m.logger.Warn("reconcile runtime subscription failed", slog.Any("error", err), slog.String("session_id", key.SessionID), slog.String("reason", reason))
					terminalDrop(reason + ": snapshot unavailable")
				}
				return false
			}
			snapshotEpoch := strings.TrimSpace(snapshot.Epoch)
			observedEpoch = strings.TrimSpace(observedEpoch)
			if snapshotEpoch == "" {
				terminalDrop(reason + ": snapshot is missing epoch")
				return false
			}
			if observedEpoch != "" && snapshotEpoch == observedEpoch && snapshot.Seq < observedSeq {
				terminalDrop(reason + ": snapshot is behind observed event")
				return false
			}
			if snapshotEpoch == lastEpoch && snapshot.Seq < lastSeq {
				terminalDrop(reason + ": snapshot sequence regressed")
				return false
			}
			if snapshotEpoch == lastEpoch && snapshot.Seq == lastSeq {
				return true
			}
			lastEpoch = snapshotEpoch
			lastSeq = snapshot.Seq
			return send(Event{
				Type:      EventRuntimeSnapshot,
				BotID:     key.BotID,
				SessionID: key.SessionID,
				Epoch:     snapshotEpoch,
				Seq:       snapshot.Seq,
				Snapshot:  &snapshot,
			})
		}
		events := backendSub.C
		for {
			select {
			case <-m.closeCh:
				return
			case <-subCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					if subCtx.Err() == nil {
						terminalDrop("runtime backend subscription closed")
					}
					return
				}
				if event.Type == EventRuntimeDropped {
					if !reconcile(runtimeEventEpoch(event), event.Seq, strings.TrimSpace(event.Message)) {
						return
					}
					continue
				}
				if event.Type != EventRuntimeDelta || event.Delta == nil {
					if !reconcile(runtimeEventEpoch(event), event.Seq, "invalid runtime backend event") {
						return
					}
					continue
				}
				eventEpoch := runtimeEventEpoch(event)
				if lastEpoch != "" && eventEpoch == "" {
					if !reconcile(lastEpoch, event.Seq, "runtime event is missing epoch") {
						return
					}
					continue
				}
				if lastEpoch != "" && eventEpoch != "" && eventEpoch != lastEpoch {
					if !reconcile(eventEpoch, event.Seq, "runtime epoch changed") {
						return
					}
					continue
				}
				if eventEpoch == lastEpoch && event.Seq <= lastSeq {
					continue
				}
				if eventEpoch == lastEpoch && event.Seq != lastSeq+1 {
					if !reconcile(eventEpoch, event.Seq, "runtime delta sequence gap") {
						return
					}
					continue
				}
				if eventEpoch != "" {
					lastEpoch = eventEpoch
				}
				if event.Seq > 0 {
					lastSeq = event.Seq
				}
				if !send(event) {
					return
				}
			case <-ticker.C:
				if !reconcile("", 0, "periodic runtime reconciliation") {
					return
				}
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
	streamID := handle.StreamID
	snapshot, changed, err := m.runState.UpdateActiveRun(ctx, handle, update)
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
	if !handle.valid() {
		return Snapshot{}, false, ErrRunOwnershipLost
	}
	ref := StreamRef{
		BotID: handle.BotID, SessionID: handle.SessionID, StreamID: handle.StreamID,
		OwnerID: m.ownerID, Generation: handle.Generation,
	}
	snapshot, changed, err := m.runState.ReleaseRun(ctx, handle, ref, update)
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
	event, err := sanitizeRuntimeEventErrors(Event{
		Type:      EventRuntimeDelta,
		BotID:     snapshot.BotID,
		SessionID: snapshot.SessionID,
		Epoch:     snapshot.Epoch,
		StreamID:  eventStreamID,
		Seq:       snapshot.Seq,
		UpdatedAt: &updatedAt,
		Delta:     &delta,
	})
	if err != nil {
		return err
	}
	return m.backend.Publish(ctx, event)
}
