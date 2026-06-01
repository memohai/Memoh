package acpagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const idleTimeout = 30 * time.Minute

type SessionPool struct {
	logger   *slog.Logger
	runner   sessionRunner
	bots     botGetter
	store    sessionGetter
	tools    *mcp.ToolGatewayService
	contexts *mcp.ToolSessionContextStore
	timeout  time.Duration

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
	BotID             string
	ChatID            string
	SessionID         string
	StreamID          string
	SessionType       string
	RouteID           string
	AgentID           string
	ProjectPath       string
	Prompt            string
	ChannelIdentityID string
	SessionToken      string //nolint:gosec // runtime session credential, not a hardcoded secret.
	CurrentPlatform   string
	ReplyTarget       string
	ConversationType  string
	ToolHTTPURL       string
	ContextURI        string
	ContextMarkdown   string
	Sink              acpclient.EventSink
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

func (p *SessionPool) SetToolGateway(gateway *mcp.ToolGatewayService) {
	if p != nil {
		p.tools = gateway
	}
}

func (p *SessionPool) SetToolSessionContextStore(store *mcp.ToolSessionContextStore) {
	if p != nil {
		p.contexts = store
	}
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

	toolSink := newPromptToolEventSink(input.Sink)
	unregisterToolSink := p.registerToolEventSink(input, toolSink)
	defer unregisterToolSink()

	result, err := sess.PromptWithResources(ctx, input.Prompt, promptResources(input), toolSink)
	orderedEvents := toolSink.Events()
	if len(orderedEvents) > 0 {
		result.Events = orderedEvents
	}
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

func promptResources(input PromptInput) []acpclient.PromptResource {
	markdown := strings.TrimSpace(input.ContextMarkdown)
	if markdown == "" {
		return nil
	}
	uri := strings.TrimSpace(input.ContextURI)
	if uri == "" {
		uri = "memoh://context/current-turn"
	}
	return []acpclient.PromptResource{{
		URI:      uri,
		MimeType: "text/markdown",
		Text:     markdown,
	}}
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
	input.SessionType = sess.Type
	if agentID := metadataString(sess.Metadata, "acp_agent_id"); agentID != "" {
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
	p.clearToolSessionContext(sessionID)
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
	for id := range states {
		p.clearToolSessionContext(id)
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
	if removedState != nil {
		p.clearToolSessionContext(sessionID)
	}
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
		p.clearToolSessionContext(item.id)
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
	toolSession := toolSessionContext(input, sessionID)
	p.storeToolSessionContext(toolSession)

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
	var env []string
	if workspaceInfo.Backend != "local" {
		env, err = managedProcessEnv(profile, setup.Managed, mode)
		if err != nil {
			p.dropSession(sessionID, nil)
			return nil, err
		}
	}

	toolHTTPURL, err := p.resolveToolHTTPURL(startCtx, input, workspaceInfo)
	if err != nil {
		p.dropSession(sessionID, nil)
		return nil, err
	}

	sess, err := p.runner.StartSession(startCtx, acpclient.StartRequest{
		AgentID:         agentID,
		BotID:           input.BotID,
		ProjectPath:     projectPath,
		Command:         profile.Command,
		Args:            profile.Args,
		LocalCommand:    profile.LocalCommand,
		LocalArgs:       profile.LocalArgs,
		Env:             env,
		SetupMode:       mode,
		Timeout:         0,
		ToolHTTPURL:     toolHTTPURL,
		ToolHTTPHandler: p.toolHTTPHandler(toolSession),
		ToolSession:     toolSession,
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

func toolSessionContext(input PromptInput, sessionID string) acpclient.ToolSessionContext {
	return acpclient.ToolSessionContext{
		BotID:             input.BotID,
		ChatID:            firstNonEmpty(input.ChatID, input.BotID),
		SessionID:         strings.TrimSpace(sessionID),
		StreamID:          strings.TrimSpace(input.StreamID),
		SessionType:       firstNonEmpty(input.SessionType, session.TypeACPAgent),
		RouteID:           input.RouteID,
		ChannelIdentityID: input.ChannelIdentityID,
		SessionToken:      input.SessionToken,
		CurrentPlatform:   input.CurrentPlatform,
		ReplyTarget:       input.ReplyTarget,
		ConversationType:  input.ConversationType,
		IsSubagent:        false,
	}
}

func (p *SessionPool) storeToolSessionContext(session acpclient.ToolSessionContext) {
	if p == nil || p.contexts == nil {
		return
	}
	p.contexts.Put(session)
}

func (p *SessionPool) clearToolSessionContext(sessionID string) {
	if p == nil || p.contexts == nil {
		return
	}
	p.contexts.CloseSession(sessionID)
}

func (p *SessionPool) registerToolEventSink(input PromptInput, sink *promptToolEventSink) func() {
	if p == nil || p.contexts == nil || sink == nil {
		return func() {}
	}
	return p.contexts.RegisterToolEventSink(acpclient.ToolSessionContext{
		BotID:     input.BotID,
		SessionID: input.SessionID,
		StreamID:  input.StreamID,
	}, sink.EmitToolStreamEvent)
}

func (p *SessionPool) resolveToolHTTPURL(_ context.Context, input PromptInput, workspaceInfo bridge.WorkspaceInfo) (string, error) {
	if p == nil || p.tools == nil {
		return "", nil
	}
	backend := strings.TrimSpace(workspaceInfo.Backend)
	if backend == bridge.WorkspaceBackendLocal {
		if raw := strings.TrimSpace(input.ToolHTTPURL); raw != "" {
			return raw, nil
		}
		return "", nil
	}
	if backend == "" || backend == bridge.WorkspaceBackendContainer {
		if raw := strings.TrimSpace(workspaceInfo.ACPToolsHTTPURL); raw != "" {
			return raw, nil
		}
		return "", nil
	}
	if raw := strings.TrimSpace(input.ToolHTTPURL); raw != "" {
		return raw, nil
	}
	return "", nil
}

func (p *SessionPool) toolHTTPHandler(trusted acpclient.ToolSessionContext) http.Handler {
	if p == nil || p.tools == nil {
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		session := trustedToolSessionContext(req, trusted)
		mcp.ServeToolMCPHTTP(w, req, p.logger, p.tools, p.contexts, session)
	})
}

func trustedToolSessionContext(req *http.Request, trusted acpclient.ToolSessionContext) mcp.ToolSessionContext {
	session := mcp.ToolSessionContextFromHTTP(req, trusted.BotID)
	force := func(dst *string, value string) {
		if value := strings.TrimSpace(value); value != "" {
			*dst = value
		}
	}
	force(&session.BotID, trusted.BotID)
	force(&session.ChatID, trusted.ChatID)
	force(&session.SessionID, trusted.SessionID)
	force(&session.StreamID, trusted.StreamID)
	force(&session.SessionType, trusted.SessionType)
	force(&session.RouteID, trusted.RouteID)
	force(&session.ChannelIdentityID, trusted.ChannelIdentityID)
	force(&session.SessionToken, trusted.SessionToken)
	force(&session.CurrentPlatform, trusted.CurrentPlatform)
	force(&session.ReplyTarget, trusted.ReplyTarget)
	force(&session.ConversationType, trusted.ConversationType)
	session.IsSubagent = trusted.IsSubagent
	return session
}

type promptToolEventSink struct {
	mu     sync.Mutex
	next   acpclient.EventSink
	events []acpclient.StreamEvent
}

func newPromptToolEventSink(next acpclient.EventSink) *promptToolEventSink {
	return &promptToolEventSink{next: next}
}

func (s *promptToolEventSink) EmitACPEvent(event acpclient.StreamEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	if s.next != nil {
		s.next.EmitACPEvent(event)
	}
}

func (s *promptToolEventSink) EmitToolStreamEvent(event mcp.ToolStreamEvent) {
	if s == nil {
		return
	}
	typ := acpclient.StreamEventType(strings.TrimSpace(event.Type))
	switch typ {
	case acpclient.StreamEventToolCallStart, acpclient.StreamEventToolCallEnd:
		s.EmitACPEvent(acpclient.StreamEvent{
			Type:       typ,
			ToolCallID: event.ToolCallID,
			ToolName:   event.ToolName,
			Input:      event.Input,
			Result:     event.Result,
			Error:      event.Error,
		})
	}
}

func (s *promptToolEventSink) Events() []acpclient.StreamEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]acpclient.StreamEvent(nil), s.events...)
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
	if profile.ID == acpprofile.AgentClaudeCodeID {
		switch mode {
		case acpclient.SetupModeOAuth:
			if strings.TrimSpace(values["oauth_token"]) == "" {
				return fmt.Errorf("oauth_token required for %s oauth setup", profile.DisplayName)
			}
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

func managedProcessEnv(profile acpprofile.Profile, values map[string]string, mode acpclient.SetupMode) ([]string, error) {
	switch profile.ID {
	case acpprofile.AgentClaudeCodeID:
		env := []string{
			"ANTHROPIC_AUTH_TOKEN=",
			"CLAUDE_CODE_USE_BEDROCK=",
			"CLAUDE_CODE_USE_VERTEX=",
			"CLAUDE_CODE_USE_FOUNDRY=",
		}
		switch mode {
		case acpclient.SetupModeAPIKey:
			apiKey := strings.TrimSpace(values["api_key"])
			if apiKey == "" {
				return nil, fmt.Errorf("api_key required for %s api_key setup", profile.DisplayName)
			}
			env = append(env,
				"CLAUDE_CODE_OAUTH_TOKEN=",
				"ANTHROPIC_API_KEY="+apiKey,
			)
		case acpclient.SetupModeOAuth:
			token := strings.TrimSpace(values["oauth_token"])
			if token == "" {
				return nil, fmt.Errorf("oauth_token required for %s oauth setup", profile.DisplayName)
			}
			env = append(env,
				"ANTHROPIC_API_KEY=",
				"CLAUDE_CODE_OAUTH_TOKEN="+token,
			)
		default:
			return nil, nil
		}
		if baseURL := strings.TrimSpace(values["base_url"]); baseURL != "" {
			env = append(env, "ANTHROPIC_BASE_URL="+baseURL)
		}
		return env, nil
	default:
		return nil, nil
	}
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (p *SessionPool) setStatus(sessionID, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if state := p.sessions[strings.TrimSpace(sessionID)]; state != nil {
		state.status = status
		state.lastActive = time.Now()
	}
}
