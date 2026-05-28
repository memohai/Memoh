package acpagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const idleTimeout = 30 * time.Minute

type SessionPool struct {
	logger  *slog.Logger
	runner  sessionRunner
	bots    botGetter
	store   sessionGetter
	timeout time.Duration

	mu       sync.RWMutex
	sessions map[string]*pooledSession
	locks    sync.Map // sessionID -> *sync.Mutex; retained to preserve per-session serialization.
}

type sessionRunner interface {
	WorkspaceInfo(ctx context.Context, botID string) (bridge.WorkspaceInfo, error)
	StartSession(ctx context.Context, req acpclient.StartRequest, sink acpclient.EventSink) (*acpclient.Session, error)
}

type botGetter interface {
	Get(ctx context.Context, botID string) (bots.Bot, error)
}

type sessionGetter interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
}

type pooledSession struct {
	session     *acpclient.Session
	agentID     string
	projectPath string
	status      string
	lastActive  time.Time
	startCancel context.CancelFunc
}

type PromptInput struct {
	BotID       string
	SessionID   string
	AgentID     string
	ProjectPath string
	Prompt      string
	Sink        acpclient.EventSink
}

// RuntimeStatus describes the live state of a pooled ACP session as exposed
// to API clients.
//
// State takes one of the following values:
//   - "idle":   no in-flight prompt/model change (the default when started)
//   - "active": a prompt or model change is currently executing
//
// The previous schema also exposed redundant `status` / `turn_status` fields
// that mirrored `state`; those were dropped in favour of a single canonical
// field so clients don't have to fall back through multiple names.
type RuntimeStatus struct {
	SessionID   string                `json:"session_id"`
	AgentID     string                `json:"agent_id,omitempty"`
	ProjectPath string                `json:"project_path,omitempty"`
	State       string                `json:"state"`
	ACPSession  string                `json:"acp_session_id,omitempty"`
	Models      *acpclient.ModelState `json:"models,omitempty"`
}

const (
	stateIdle   = "idle"
	stateActive = "active"
)

func NewSessionPool(log *slog.Logger, runner *acpclient.Runner, botService *bots.Service, sessionServices ...*session.Service) *SessionPool {
	var sessionService sessionGetter
	if len(sessionServices) > 0 {
		sessionService = sessionServices[0]
	}
	return newSessionPool(log, runner, botService, sessionService)
}

func newSessionPool(log *slog.Logger, runner sessionRunner, botService botGetter, sessionServices ...sessionGetter) *SessionPool {
	if log == nil {
		log = slog.Default()
	}
	var sessionService sessionGetter
	if len(sessionServices) > 0 {
		sessionService = sessionServices[0]
	}
	return &SessionPool{
		logger:   log.With(slog.String("service", "acp_session_pool")),
		runner:   runner,
		bots:     botService,
		store:    sessionService,
		timeout:  idleTimeout,
		sessions: map[string]*pooledSession{},
	}
}

// prepareInput validates pool wiring and required input fields, returning
// the input with session metadata applied. Callers must check the returned
// error before using the resolved input.
func (p *SessionPool) prepareInput(ctx context.Context, input PromptInput) (PromptInput, error) {
	if p == nil || p.runner == nil || p.bots == nil {
		return PromptInput{}, errors.New("ACP session pool is not configured")
	}
	if strings.TrimSpace(input.SessionID) == "" {
		return PromptInput{}, errors.New("session_id is required")
	}
	resolved, err := p.resolveSessionMetadata(ctx, input)
	if err != nil {
		return PromptInput{}, err
	}
	if strings.TrimSpace(resolved.BotID) == "" {
		return PromptInput{}, errors.New("bot_id is required")
	}
	return resolved, nil
}

// Prompt sends a prompt to the ACP session identified by input.SessionID.
//
// Idle reaping and dropped-session cleanup ultimately invoke
// (*acpclient.Session).Close, which uses its own short-lived background
// context so cleanup always runs even if the caller's ctx was cancelled.
// That intentional disconnect trips contextcheck within this function, so we
// silence it here.
//
//nolint:contextcheck // lifecycle close intentionally uses background ctx.
func (p *SessionPool) Prompt(ctx context.Context, input PromptInput) (acpclient.PromptResult, error) {
	input, err := p.prepareInput(ctx, input)
	if err != nil {
		return acpclient.PromptResult{}, err
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return acpclient.PromptResult{}, errors.New("prompt is required")
	}

	p.reapIdle(time.Now())
	unlock := p.lockSession(input.SessionID)
	defer unlock()
	p.setStatus(input.SessionID, stateActive)

	sess, err := p.getOrStart(ctx, input)
	if err != nil {
		p.dropSession(input.SessionID, nil)
		return acpclient.PromptResult{}, err
	}

	result, err := sess.Prompt(ctx, input.Prompt, input.Sink)
	if err != nil {
		// Prompt failures usually indicate the ACP process is in a bad state
		// (transport hang, agent crash); drop the underlying session so the
		// next call starts fresh.
		p.dropSession(input.SessionID, sess)
		return result, err
	}
	p.setStatus(input.SessionID, stateIdle)
	return result, nil
}

//nolint:contextcheck // lifecycle close intentionally uses background ctx.
func (p *SessionPool) Ensure(ctx context.Context, input PromptInput) (RuntimeStatus, error) {
	input, err := p.prepareInput(ctx, input)
	if err != nil {
		return RuntimeStatus{}, err
	}

	p.reapIdle(time.Now())
	unlock := p.lockSession(input.SessionID)
	defer unlock()

	sess, err := p.getOrStart(ctx, input)
	if err != nil {
		p.dropSession(input.SessionID, nil)
		return RuntimeStatus{}, err
	}
	p.setStatus(input.SessionID, stateIdle)
	return p.RuntimeStatus(input.SessionID, input.AgentID, sess.ProjectPath()), nil
}

//nolint:contextcheck // lifecycle close intentionally uses background ctx.
func (p *SessionPool) SetModel(ctx context.Context, input PromptInput, modelID string) (RuntimeStatus, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return RuntimeStatus{}, acpclient.ErrModelIDRequired
	}
	input, err := p.prepareInput(ctx, input)
	if err != nil {
		return RuntimeStatus{}, err
	}

	p.reapIdle(time.Now())
	unlock := p.lockSession(input.SessionID)
	defer unlock()
	p.setStatus(input.SessionID, stateActive)

	sess, err := p.getOrStart(ctx, input)
	if err != nil {
		p.dropSession(input.SessionID, nil)
		return RuntimeStatus{}, err
	}
	if _, err := sess.SetModel(ctx, modelID); err != nil {
		// Model selection errors are validation/protocol issues, not process
		// failures; keep the session alive so the user can pick another model.
		p.setStatus(input.SessionID, stateIdle)
		return RuntimeStatus{}, err
	}
	p.setStatus(input.SessionID, stateIdle)
	return p.RuntimeStatus(input.SessionID, input.AgentID, sess.ProjectPath()), nil
}

func (p *SessionPool) resolveSessionMetadata(ctx context.Context, input PromptInput) (PromptInput, error) {
	if p == nil || p.store == nil {
		return input, nil
	}
	sess, err := p.store.Get(ctx, input.SessionID)
	if err != nil {
		return input, fmt.Errorf("load ACP session metadata: %w", err)
	}
	if sess.Type != session.TypeACPAgent {
		return input, fmt.Errorf("session %s is not an ACP agent session", input.SessionID)
	}
	if input.BotID != "" && sess.BotID != "" && input.BotID != sess.BotID {
		return input, fmt.Errorf("session %s does not belong to bot %s", input.SessionID, input.BotID)
	}
	if input.BotID == "" {
		input.BotID = sess.BotID
	}
	if agentID := metadataString(sess.Metadata, "acp_agent_id"); agentID != "" {
		input.AgentID = agentID
	} else if agentID := metadataString(sess.Metadata, "agent_id"); agentID != "" {
		input.AgentID = agentID
	}
	if projectPath := metadataString(sess.Metadata, "project_path"); projectPath != "" {
		input.ProjectPath = projectPath
	}
	return input, nil
}

func (p *SessionPool) RuntimeStatus(sessionID, agentID, projectPath string) RuntimeStatus {
	sessionID = strings.TrimSpace(sessionID)
	idle := RuntimeStatus{
		SessionID:   sessionID,
		AgentID:     strings.TrimSpace(agentID),
		ProjectPath: strings.TrimSpace(projectPath),
		State:       stateIdle,
	}
	if p == nil {
		return idle
	}
	p.mu.RLock()
	state := p.sessions[sessionID]
	var sess *acpclient.Session
	var currentAgentID, currentProjectPath, currentState string
	if state != nil {
		sess = state.session
		currentAgentID = state.agentID
		currentProjectPath = state.projectPath
		currentState = state.status
	}
	p.mu.RUnlock()
	if state == nil {
		return idle
	}
	acpSessionID := ""
	var models *acpclient.ModelState
	if sess != nil {
		acpSessionID = sess.ID()
		modelState := sess.ModelState()
		models = &modelState
	}
	currentState = strings.TrimSpace(currentState)
	if currentState == "" {
		currentState = stateIdle
	}
	return RuntimeStatus{
		SessionID:   sessionID,
		AgentID:     currentAgentID,
		ProjectPath: currentProjectPath,
		State:       currentState,
		ACPSession:  acpSessionID,
		Models:      models,
	}
}

func (p *SessionPool) IsSessionActive(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if p == nil || sessionID == "" {
		return false
	}
	if value, ok := p.locks.Load(sessionID); ok {
		mu := value.(*sync.Mutex)
		if !mu.TryLock() {
			return true
		}
		mu.Unlock()
	}
	p.mu.RLock()
	state := p.sessions[sessionID]
	active := state != nil && state.status == stateActive
	p.mu.RUnlock()
	return active
}

func (p *SessionPool) StartReaper(ctx context.Context) {
	if p == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.reapIdle(time.Now()) //nolint:contextcheck // reaper close uses its own background ctx.
			case <-ctx.Done():
				return
			}
		}
	}()
}

//nolint:contextcheck // lifecycle close intentionally uses background ctx so cleanup runs after caller cancels.
func (p *SessionPool) CloseSession(sessionID string) error {
	if p == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	unlock := p.lockSession(sessionID)
	defer unlock()

	p.mu.Lock()
	state := p.sessions[sessionID]
	delete(p.sessions, sessionID)
	p.mu.Unlock()
	if state != nil && state.startCancel != nil {
		state.startCancel()
	}
	if state != nil && state.session != nil {
		return state.session.Close()
	}
	return nil
}

func (p *SessionPool) CloseAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	states := make(map[string]*pooledSession, len(p.sessions))
	for id, state := range p.sessions {
		delete(p.sessions, id)
		if state != nil {
			states[id] = state
		}
	}
	p.mu.Unlock()
	for _, state := range states {
		if state.startCancel != nil {
			state.startCancel()
		}
		if state.session != nil {
			if err := state.session.Close(); err != nil {
				p.logger.Warn("failed to close ACP session", slog.Any("error", err))
			}
		}
	}
}

func (p *SessionPool) dropSession(sessionID string, sess *acpclient.Session) {
	if p == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	p.mu.Lock()
	state := p.sessions[sessionID]
	var removedState *pooledSession
	if state != nil && (sess == nil || state.session == sess) {
		delete(p.sessions, sessionID)
		removedState = state
	}
	p.mu.Unlock()
	if removedState != nil && removedState.startCancel != nil {
		removedState.startCancel()
	}
	if sess != nil {
		if err := sess.Close(); err != nil {
			p.logger.Warn("failed to close failed ACP session", slog.Any("error", err), slog.String("session_id", sessionID))
		}
	}
}

func (p *SessionPool) lockSession(sessionID string) func() {
	value, _ := p.locks.LoadOrStore(strings.TrimSpace(sessionID), &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (p *SessionPool) reapIdle(now time.Time) int {
	if p == nil || p.timeout <= 0 {
		return 0
	}
	type staleSession struct {
		id      string
		session *acpclient.Session
	}
	var stale []staleSession
	p.mu.Lock()
	for id, state := range p.sessions {
		if state == nil || state.status == stateActive || state.lastActive.IsZero() {
			continue
		}
		if now.Sub(state.lastActive) <= p.timeout {
			continue
		}
		stale = append(stale, staleSession{id: id, session: state.session})
		delete(p.sessions, id)
	}
	p.mu.Unlock()

	for _, item := range stale {
		if item.session != nil {
			if err := item.session.Close(); err != nil {
				p.logger.Warn("failed to close idle ACP session", slog.Any("error", err), slog.String("session_id", item.id))
			}
		}
	}
	return len(stale)
}

//nolint:contextcheck // startup failure cleanup intentionally uses background ctx.
func (p *SessionPool) getOrStart(ctx context.Context, input PromptInput) (*acpclient.Session, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	agentID := acpprofile.NormalizeAgentID(input.AgentID)
	if agentID == "" {
		agentID = acpprofile.AgentCodexID
	}
	projectPath := strings.TrimSpace(input.ProjectPath)

	p.mu.RLock()
	existing := p.sessions[sessionID]
	var existingSession *acpclient.Session
	var existingAgentID, existingProjectPath string
	if existing != nil {
		existingSession = existing.session
		existingAgentID = existing.agentID
		existingProjectPath = existing.projectPath
	}
	p.mu.RUnlock()
	if existingSession != nil && existingAgentID == agentID && existingProjectPath == projectPath {
		return existingSession, nil
	}
	if existing != nil {
		_ = p.CloseSession(sessionID)
	}
	startCtx, cancelStart := context.WithCancel(ctx)
	defer cancelStart()
	starting := &pooledSession{
		agentID:     agentID,
		projectPath: projectPath,
		status:      stateActive,
		lastActive:  time.Now(),
		startCancel: cancelStart,
	}
	p.mu.Lock()
	p.sessions[sessionID] = starting
	p.mu.Unlock()

	bot, err := p.bots.Get(startCtx, input.BotID)
	if err != nil {
		p.dropSession(sessionID, nil)
		return nil, fmt.Errorf("load bot ACP setup: %w", err)
	}
	setup := acpprofile.ParseAgentSetup(bot.Metadata, agentID)
	if !setup.Enabled {
		p.dropSession(sessionID, nil)
		return nil, fmt.Errorf("ACP agent %q is not enabled for this bot", agentID)
	}
	profile, ok := acpprofile.Lookup(agentID)
	if !ok {
		p.dropSession(sessionID, nil)
		return nil, fmt.Errorf("unknown ACP agent %q", agentID)
	}
	workspaceInfo, err := p.runner.WorkspaceInfo(startCtx, input.BotID)
	if err != nil {
		p.dropSession(sessionID, nil)
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}

	mode := acpclient.SetupMode(setup.Mode)
	if mode == "" {
		mode = acpclient.SetupModeAPIKey
	}
	if workspaceInfo.Backend != "local" && mode != acpclient.SetupModeSelf {
		if err := validateManagedFields(profile, setup.Managed, mode); err != nil {
			p.dropSession(sessionID, nil)
			return nil, err
		}
	}

	sess, err := p.runner.StartSession(startCtx, acpclient.StartRequest{
		AgentID:      agentID,
		BotID:        input.BotID,
		ProjectPath:  projectPath,
		Command:      profile.Command,
		Args:         profile.Args,
		LocalCommand: profile.LocalCommand,
		LocalArgs:    profile.LocalArgs,
		SetupMode:    mode,
		Timeout:      0,
	}, input.Sink)
	if err != nil {
		p.dropSession(sessionID, nil)
		return nil, err
	}

	p.mu.Lock()
	if p.sessions[sessionID] != starting {
		p.mu.Unlock()
		if closeErr := sess.Close(); closeErr != nil {
			p.logger.Warn("failed to close ACP session after startup cancellation", slog.Any("error", closeErr), slog.String("session_id", sessionID))
		}
		return nil, fmt.Errorf("ACP session %s was closed during startup", sessionID)
	}
	p.sessions[sessionID] = &pooledSession{
		session:     sess,
		agentID:     agentID,
		projectPath: projectPath,
		status:      stateIdle,
		lastActive:  time.Now(),
	}
	p.mu.Unlock()
	return sess, nil
}

func validateManagedFields(profile acpprofile.Profile, values map[string]string, mode acpclient.SetupMode) error {
	if profile.ID == acpprofile.AgentCodexID {
		switch mode {
		case acpclient.SetupModeOAuth:
			return nil
		default:
			if strings.TrimSpace(values["api_key"]) == "" {
				return fmt.Errorf("api_key required for %s api_key setup", profile.DisplayName)
			}
			return nil
		}
	}
	for _, field := range profile.ManagedFields {
		if !field.Required {
			continue
		}
		if strings.TrimSpace(values[field.ID]) == "" {
			return fmt.Errorf("%s required for %s %s setup", field.ID, profile.DisplayName, mode)
		}
	}
	return nil
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func (p *SessionPool) setStatus(sessionID, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state := p.sessions[strings.TrimSpace(sessionID)]; state != nil {
		state.status = status
		state.lastActive = time.Now()
	}
}
