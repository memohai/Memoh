package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
)

// SpawnAgent is the interface the subagent control tools use to run tasks.
// It is satisfied by *agent.Agent and avoids an import cycle.
type SpawnAgent interface {
	Generate(ctx context.Context, cfg SpawnRunConfig) (*SpawnResult, error)
	GenerateWithWatchdog(ctx context.Context, cfg SpawnRunConfig, touchFn func()) (*SpawnResult, error)
}

// SpawnRunConfig mirrors agent.RunConfig fields needed by subagent controls.
type SpawnRunConfig struct {
	Model           *sdk.Model
	System          string
	Query           string
	SessionType     string
	Identity        SpawnIdentity
	LoopDetection   SpawnLoopConfig
	Messages        []sdk.Message
	ReasoningEffort string
	PromptCacheTTL  string
}

// SpawnIdentity mirrors agent.SessionContext fields needed by subagent controls.
type SpawnIdentity struct {
	BotID             string
	ChatID            string
	SessionID         string
	ChannelIdentityID string
	CurrentPlatform   string
	SessionToken      string //nolint:gosec // #nosec G117 -- session identifier, not a secret
	IsSubagent        bool
}

// SpawnLoopConfig mirrors agent.LoopDetectionConfig.
type SpawnLoopConfig struct {
	Enabled bool
}

// SpawnResult mirrors agent.GenerateResult.
type SpawnResult struct {
	Messages []sdk.Message
	Text     string
	Usage    *sdk.Usage
}

const (
	// subagentTimeout caps total execution time as a safety net per attempt.
	subagentTimeout = 10 * time.Minute
	// spawnHeartbeatInterval keeps the parent stream active during foreground waits.
	spawnHeartbeatInterval  = 30 * time.Second
	subagentMaxRetries      = 3
	subagentRetryBaseDelay  = 2 * time.Second
	subagentWatchdogTimeout = 3 * time.Minute

	agentControlVersion = "v1"
)

// ErrWatchdogTimedOut is returned when the subagent watchdog fires.
var ErrWatchdogTimedOut = errors.New("subagent watchdog: no activity within timeout")

var (
	err429Pattern    = regexp.MustCompile(`(^|[^0-9])429($|[^0-9])`)
	errEOFPattern    = regexp.MustCompile(`(?i)connection (reset|refused)|EOF$`)
	serverErrPattern = regexp.MustCompile(`api error 5\\d{2}`)
	agentIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	errAgentNotFound = errors.New("agent not found")
)

// SubagentWatchdog implements an activity-based timeout for subagent execution.
type SubagentWatchdog struct {
	timeout time.Duration
	touchCh chan struct{}
	cancel  context.CancelCauseFunc
	done    chan struct{}
	logger  *slog.Logger
}

func NewSubagentWatchdog(parentCtx context.Context, timeout time.Duration, logger *slog.Logger) (context.Context, *SubagentWatchdog) {
	if timeout <= 0 {
		timeout = subagentWatchdogTimeout
	}
	ctx, cancel := context.WithCancelCause(parentCtx)
	wd := &SubagentWatchdog{
		timeout: timeout,
		touchCh: make(chan struct{}, 1),
		cancel:  cancel,
		done:    make(chan struct{}),
		logger:  logger,
	}
	go wd.run(ctx)
	return ctx, wd
}

func (w *SubagentWatchdog) Touch() {
	select {
	case w.touchCh <- struct{}{}:
	default:
	}
}

func (w *SubagentWatchdog) Stop() {
	w.cancel(context.Canceled)
	<-w.done
}

func (w *SubagentWatchdog) run(ctx context.Context) {
	defer close(w.done)
	timer := time.NewTimer(w.timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.touchCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.timeout)
		case <-timer.C:
			w.logger.Warn("subagent watchdog fired", slog.Duration("timeout", w.timeout))
			w.cancel(ErrWatchdogTimedOut)
			return
		}
	}
}

type agentSessionService interface {
	Create(ctx context.Context, input sessionpkg.CreateInput) (sessionpkg.Session, error)
	ListSubagentsByParent(ctx context.Context, parentSessionID string) ([]sessionpkg.Session, error)
}

// SpawnProvider exposes managed subagent control tools.
type SpawnProvider struct {
	agent          SpawnAgent
	settings       *settings.Service
	models         *models.Service
	queries        dbstore.Queries
	sessionService agentSessionService
	messageService messagepkg.Service
	systemPromptFn func(sessionType string) string
	modelCreator   ModelCreator
	bgManager      *background.Manager
	hookService    *hooks.Service
	modelResolver  func(ctx context.Context, botID string) (*sdk.Model, string, string, error)
	coord          *agentCoordinator
	logger         *slog.Logger
}

func NewSpawnProvider(
	log *slog.Logger,
	settingsSvc *settings.Service,
	modelsSvc *models.Service,
	queries dbstore.Queries,
	sessionService *sessionpkg.Service,
	bgManager *background.Manager,
) *SpawnProvider {
	if log == nil {
		log = slog.Default()
	}
	p := &SpawnProvider{
		settings:       settingsSvc,
		models:         modelsSvc,
		queries:        queries,
		sessionService: sessionService,
		bgManager:      bgManager,
		coord:          newAgentCoordinator(),
		logger:         log.With(slog.String("tool", "agent_control")),
	}
	p.modelResolver = p.resolveModel
	return p
}

func (p *SpawnProvider) SetAgent(a SpawnAgent) {
	p.agent = a
}

func (p *SpawnProvider) SetMessageService(w messagepkg.Service) {
	p.messageService = w
}

func (p *SpawnProvider) SetSystemPromptFunc(fn func(sessionType string) string) {
	p.systemPromptFn = fn
}

func (p *SpawnProvider) SetHookService(h *hooks.Service) {
	p.hookService = h
}

// Usage frames how the available agent-control tools are meant to be used.
func (*SpawnProvider) Usage(_ context.Context, _ SessionContext, available AvailableTools) string {
	var parts []string
	canStartBackground := false
	if spawnRef, ok := available.Ref(ToolSpawnAgent()); ok {
		canStartBackground = true
		parts = append(parts,
			"Use "+spawnRef+" to create a managed subagent for an independent task.",
			"Each subagent has a restricted worker tool set: file tools, exec/background tools, web search, and web fetch.",
			"Use subagents when work benefits from isolated context or can proceed while you continue. Don't use one for simple single-step work — just do it directly.",
		)
	}
	if ref, ok := available.Ref(ToolSendMessage()); ok {
		canStartBackground = true
		parts = append(parts, "Use "+ref+" to continue an existing agent with a follow-up.")
	}
	if backgroundTools := available.Refs(ToolWaitUntil(), ToolGetBackgroundStatus(), ToolListBackground(), ToolKillBackground()); len(backgroundTools) > 0 {
		if canStartBackground {
			parts = append(parts, "For long work, set `run_in_background: true`. The call returns a task ID immediately. Use `wait_until(task_id)`, then `get_background_status(task_id)` to read `result`.")
		}
		parts = append(parts, "Manage running agent tasks with "+joinRefs(backgroundTools, "and")+".")
	}
	if ref, ok := available.Ref(ToolListAgents()); ok {
		parts = append(parts, "Use "+ref+" to see agents created in the current session.")
	}
	if ref, ok := available.Ref(ToolSearchMessages()); ok {
		parts = append(parts, "Read a finished task's full transcript with "+ref+" using the session ID returned by get_background_status when the brief result is not enough.")
	}
	if len(parts) == 0 {
		return ""
	}
	return usageSection("Subagents", parts)
}

func (p *SpawnProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.agent == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        ToolSpawnAgent().String(),
			Description: "Create one managed subagent for an independent task. Returns a memorable agent_id.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Optional memorable agent id. If omitted, an id like agent_1 is assigned. Must be lowercase letters, digits, underscore, or hyphen.",
					},
					"task": map[string]any{
						"type":        "string",
						"description": "Task instruction for the new agent.",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "If true, return immediately with a task_id. Use wait_until(task_id), then get_background_status(task_id) to inspect result.",
					},
				},
				"required": []string{"task"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSpawnAgent(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        ToolSendMessage().String(),
			Description: "Send a follow-up message to an existing managed subagent. Messages to a busy agent are queued and run serially.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "Existing agent id returned when the agent was created or listed.",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Follow-up instruction for the agent.",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "If true, return immediately with a task_id. If the agent is busy, the message is queued regardless of this value. Use wait_until(task_id), then get_background_status(task_id) to inspect result.",
					},
				},
				"required": []string{"id", "message"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSendMessage(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        ToolListAgents().String(),
			Description: "List managed subagents created in the current session only.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execListAgents(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

type agentRecord struct {
	AgentID   string
	SessionID string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type agentRunResult struct {
	AgentID        string `json:"agent_id"`
	SessionID      string `json:"session_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	Text           string `json:"text,omitempty"`
	Error          string `json:"error,omitempty"`
	QueuePosition  int    `json:"queue_position,omitempty"`
	QueueRemaining int    `json:"queue_remaining,omitempty"`
	TimedOut       bool   `json:"timed_out,omitempty"`
}

type agentRequest struct {
	taskID           string
	agentID          string
	agentSessionID   string
	message          string
	messagePersisted bool
	parentSession    SessionContext
	model            *sdk.Model
	modelID          string
	promptCacheTTL   string
	systemPrompt     string
}

type agentCoordinator struct {
	mu     sync.Mutex
	states map[string]*agentState
}

type agentState struct {
	botID           string
	parentSessionID string
	agentID         string
	agentSessionID  string
	runningTaskID   string
	queue           []*agentRequest
	last            agentRunResult
}

type agentStateSnapshot struct {
	RunningTaskID string
	QueuedTaskIDs []string
	Last          agentRunResult
}

func newAgentCoordinator() *agentCoordinator {
	return &agentCoordinator{states: make(map[string]*agentState)}
}

func agentStateKey(botID, parentSessionID, agentID string) string {
	return botID + "\x00" + parentSessionID + "\x00" + agentID
}

func (c *agentCoordinator) ensure(botID, parentSessionID, agentID, agentSessionID string) *agentState {
	key := agentStateKey(botID, parentSessionID, agentID)
	st := c.states[key]
	if st == nil {
		st = &agentState{
			botID:           botID,
			parentSessionID: parentSessionID,
			agentID:         agentID,
			agentSessionID:  agentSessionID,
		}
		c.states[key] = st
	}
	if st.agentSessionID == "" {
		st.agentSessionID = agentSessionID
	}
	return st
}

func (c *agentCoordinator) snapshot(botID, parentSessionID, agentID string) agentStateSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[agentStateKey(botID, parentSessionID, agentID)]
	if st == nil {
		return agentStateSnapshot{}
	}
	queued := make([]string, 0, len(st.queue))
	for _, req := range st.queue {
		queued = append(queued, req.taskID)
	}
	return agentStateSnapshot{
		RunningTaskID: st.runningTaskID,
		QueuedTaskIDs: queued,
		Last:          st.last,
	}
}

func (p *SpawnProvider) execSpawnAgent(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	if err := validateParentSession(session); err != nil {
		return nil, err
	}
	task := strings.TrimSpace(StringArg(args, "task"))
	if task == "" {
		return nil, errors.New("task is required")
	}
	agentID, err := p.resolveNewAgentID(ctx, session, StringArg(args, "id"))
	if err != nil {
		return nil, err
	}
	if existing, err := p.findAgent(ctx, session, agentID); err == nil && existing.AgentID != "" {
		return nil, fmt.Errorf("agent %q already exists; choose a different id or continue the existing agent if follow-up messaging is available", agentID)
	} else if err != nil && !errors.Is(err, errAgentNotFound) {
		return nil, err
	}
	rec, err := p.createAgentSession(context.WithoutCancel(ctx), session, agentID, task)
	if err != nil {
		return nil, err
	}
	runInBackground, _, _ := BoolArg(args, "run_in_background")
	return p.submitAgentTask(ctx, session, rec, task, runInBackground)
}

func (p *SpawnProvider) execSendMessage(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	if err := validateParentSession(session); err != nil {
		return nil, err
	}
	agentID, err := normalizeAgentID(StringArg(args, "id"))
	if err != nil {
		return nil, err
	}
	message := strings.TrimSpace(StringArg(args, "message"))
	if message == "" {
		return nil, errors.New("message is required")
	}
	rec, err := p.findAgent(ctx, session, agentID)
	if err != nil {
		return nil, err
	}
	runInBackground, _, _ := BoolArg(args, "run_in_background")
	return p.submitAgentTask(ctx, session, rec, message, runInBackground)
}

func (p *SpawnProvider) execListAgents(ctx context.Context, session SessionContext, _ map[string]any) (any, error) {
	if err := validateParentSession(session); err != nil {
		return nil, err
	}
	agents, err := p.listAgentRecords(ctx, session)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(agents))
	for _, rec := range agents {
		snap := p.coord.snapshot(session.BotID, session.SessionID, rec.AgentID)
		status := "idle"
		switch {
		case snap.RunningTaskID != "":
			status = string(background.TaskRunning)
		case len(snap.QueuedTaskIDs) > 0:
			status = string(background.TaskQueued)
		case snap.Last.Status != "":
			status = snap.Last.Status
		}
		item := map[string]any{
			"agent_id":     rec.AgentID,
			"session_id":   rec.SessionID,
			"title":        rec.Title,
			"status":       status,
			"queued_count": len(snap.QueuedTaskIDs),
			"created_at":   session.FormatTime(rec.CreatedAt),
			"updated_at":   session.FormatTime(rec.UpdatedAt),
		}
		if snap.RunningTaskID != "" {
			item["current_task_id"] = snap.RunningTaskID
		}
		if snap.Last.TaskID != "" {
			item["last_task_id"] = snap.Last.TaskID
		}
		items = append(items, item)
	}
	return map[string]any{"agents": items, "count": len(items)}, nil
}

func (p *SpawnProvider) submitAgentTask(ctx context.Context, session SessionContext, rec agentRecord, message string, runInBackground bool) (any, error) {
	if p.bgManager == nil {
		return nil, errors.New("background task manager not available")
	}
	sdkModel, modelID, promptCacheTTL, err := p.modelResolver(context.WithoutCancel(ctx), session.BotID)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}
	systemPrompt := ""
	if p.systemPromptFn != nil {
		systemPrompt = p.systemPromptFn(sessionpkg.TypeSubagent)
	}

	req := &agentRequest{
		agentID:        rec.AgentID,
		agentSessionID: rec.SessionID,
		message:        message,
		parentSession:  session,
		model:          sdkModel,
		modelID:        modelID,
		promptCacheTTL: promptCacheTTL,
		systemPrompt:   systemPrompt,
	}
	description := truncateTitle(fmt.Sprintf("%s: %s", rec.AgentID, message), 120)

	key := agentStateKey(session.BotID, session.SessionID, rec.AgentID)
	p.coord.mu.Lock()
	st := p.coord.ensure(session.BotID, session.SessionID, rec.AgentID, rec.SessionID)
	if st.runningTaskID != "" {
		taskID, _, err := p.bgManager.StartAgentTask(ctx, session.BotID, session.SessionID, rec.AgentID, rec.SessionID, message, description, true)
		if err != nil {
			p.coord.mu.Unlock()
			return nil, err
		}
		req.taskID = taskID
		st.queue = append(st.queue, req)
		queuePosition := len(st.queue)
		p.coord.mu.Unlock()
		return map[string]any{
			"agent_id":       rec.AgentID,
			"session_id":     rec.SessionID,
			"task_id":        taskID,
			"status":         string(background.TaskQueued),
			"description":    description,
			"queue_position": queuePosition,
			"message":        "Agent is currently running. Message queued. Use wait_until(task_id), then get_background_status(task_id) to inspect result.",
		}, nil
	}

	taskID, taskCtx, err := p.bgManager.StartAgentTask(context.WithoutCancel(ctx), session.BotID, session.SessionID, rec.AgentID, rec.SessionID, message, description, false)
	if err != nil {
		p.coord.mu.Unlock()
		return nil, err
	}
	req.taskID = taskID
	st.runningTaskID = taskID
	p.coord.mu.Unlock()

	if runInBackground {
		go p.runAgentRequest(taskCtx, key, req)
		return map[string]any{
			"agent_id":    rec.AgentID,
			"session_id":  rec.SessionID,
			"task_id":     taskID,
			"status":      "background_started",
			"description": description,
			"message":     fmt.Sprintf("Agent %s started in background with task ID: %s. Use wait_until(task_id), then get_background_status(task_id) to inspect result.", rec.AgentID, taskID),
		}, nil
	}

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer heartbeatCancel()
	p.startSpawnHeartbeat(heartbeatCtx, session, 1)
	result := p.runAgentRequest(taskCtx, key, req)
	return agentResultMap(result), nil
}

func (p *SpawnProvider) runAgentRequest(ctx context.Context, key string, req *agentRequest) agentRunResult {
	req.messagePersisted = p.persistUserMessage(context.WithoutCancel(ctx), req.parentSession.BotID, req.agentSessionID, req.message)
	result := p.runSubagentTask(ctx, req)
	if task := p.bgManager.Get(req.taskID); task != nil {
		if snap := task.Snapshot(); snap.Status == background.TaskKilled {
			result.Status = string(background.TaskKilled)
		}
	}
	status := background.TaskCompleted
	switch {
	case result.Status == string(background.TaskKilled):
		status = background.TaskKilled
	case result.Error != "":
		status = background.TaskFailed
		result.Status = string(background.TaskFailed)
	default:
		result.Status = string(background.TaskCompleted)
	}
	p.bgManager.CompleteAgentTask(req.taskID, background.AgentTaskResult{
		AgentID:        req.agentID,
		AgentSessionID: req.agentSessionID,
		Message:        req.message,
		Status:         status,
		Report:         result.Text,
		Error:          result.Error,
	})
	p.finishAgentRequest(ctx, key, result)
	return result
}

func (p *SpawnProvider) finishAgentRequest(ctx context.Context, key string, result agentRunResult) {
	var next *agentRequest
	p.coord.mu.Lock()
	st := p.coord.states[key]
	if st != nil {
		st.runningTaskID = ""
		st.last = result
		for len(st.queue) > 0 {
			candidate := st.queue[0]
			st.queue = st.queue[1:]
			task := p.bgManager.Get(candidate.taskID)
			if task != nil && task.Snapshot().Status == background.TaskKilled {
				continue
			}
			next = candidate
			st.runningTaskID = candidate.taskID
			break
		}
	}
	p.coord.mu.Unlock()
	if next == nil {
		return
	}
	runCtx, ok, err := p.bgManager.MarkAgentTaskRunning(ctx, next.taskID)
	if err != nil {
		p.logger.Warn("start queued agent task failed", slog.String("task_id", next.taskID), slog.Any("error", err))
		p.finishAgentRequest(ctx, key, agentRunResult{
			AgentID:   next.agentID,
			SessionID: next.agentSessionID,
			TaskID:    next.taskID,
			Status:    string(background.TaskFailed),
			Message:   next.message,
			Error:     err.Error(),
		})
		return
	}
	if !ok {
		p.finishAgentRequest(ctx, key, agentRunResult{
			AgentID:   next.agentID,
			SessionID: next.agentSessionID,
			TaskID:    next.taskID,
			Status:    string(background.TaskKilled),
			Message:   next.message,
		})
		return
	}
	go p.runAgentRequest(runCtx, key, next)
}

func (p *SpawnProvider) runSubagentTask(ctx context.Context, req *agentRequest) agentRunResult {
	res := agentRunResult{
		AgentID:   req.agentID,
		SessionID: req.agentSessionID,
		TaskID:    req.taskID,
		Message:   req.message,
	}
	if err := p.runSubagentHook(ctx, hooks.EventSubagentStart, req, res); err != nil {
		res.Error = err.Error()
		res.Status = string(background.TaskFailed)
		return res
	}
	defer func() {
		if err := p.runSubagentHook(context.WithoutCancel(ctx), hooks.EventSubagentStop, req, res); err != nil && p.logger != nil {
			p.logger.Warn("subagent stop hook failed",
				slog.String("bot_id", req.parentSession.BotID),
				slog.String("agent_id", req.agentID),
				slog.Any("error", err),
			)
		}
	}()
	history := p.loadAgentMessages(context.WithoutCancel(ctx), req.agentSessionID)
	if req.messagePersisted {
		history = dropLatestMatchingUserMessage(history, req.message)
	}
	cfg := SpawnRunConfig{
		Model:          req.model,
		System:         req.systemPrompt,
		Query:          req.message,
		SessionType:    sessionpkg.TypeSubagent,
		PromptCacheTTL: req.promptCacheTTL,
		Messages:       history,
		Identity: SpawnIdentity{
			BotID:             req.parentSession.BotID,
			ChatID:            req.parentSession.ChatID,
			SessionID:         req.agentSessionID,
			ChannelIdentityID: req.parentSession.ChannelIdentityID,
			CurrentPlatform:   req.parentSession.CurrentPlatform,
			SessionToken:      req.parentSession.SessionToken,
			IsSubagent:        true,
		},
		LoopDetection: SpawnLoopConfig{Enabled: true},
	}

	var lastErr error
	for attempt := 0; attempt <= subagentMaxRetries; attempt++ {
		if attempt > 0 {
			delay := subagentRetryBaseDelay * time.Duration(attempt)
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				res.Error = fmt.Sprintf("parent cancelled: %v", ctx.Err())
				return res
			}
		}

		safetyCtx, safetyCancel := context.WithTimeout(ctx, subagentTimeout)
		wdCtx, wd := NewSubagentWatchdog(safetyCtx, subagentWatchdogTimeout, p.logger)
		genResult, err := p.agent.GenerateWithWatchdog(wdCtx, cfg, wd.Touch)
		wd.Stop()
		safetyCancel()

		if err == nil {
			res.Text = genResult.Text
			if p.messageService != nil && req.agentSessionID != "" {
				p.persistMessages(context.WithoutCancel(ctx), req.parentSession.BotID, req.agentSessionID, req.modelID, req.message, genResult, !req.messagePersisted)
			}
			return res
		}
		lastErr = err
		if ctx.Err() != nil && !errors.Is(err, ErrWatchdogTimedOut) {
			res.Error = fmt.Sprintf("parent cancelled: %v", ctx.Err())
			return res
		}
		if errors.Is(err, ErrWatchdogTimedOut) || isRetryableSubagentError(err) {
			continue
		}
		res.Error = err.Error()
		return res
	}
	res.Error = fmt.Sprintf("all %d attempts failed (last: %v)", subagentMaxRetries+1, lastErr)
	return res
}

func (p *SpawnProvider) runSubagentHook(ctx context.Context, eventName string, req *agentRequest, result agentRunResult) error {
	if p == nil || p.hookService == nil || req == nil {
		return nil
	}
	extra := map[string]any{
		"agent_id":         req.agentID,
		"agent_session_id": req.agentSessionID,
		"task_id":          req.taskID,
		"message":          req.message,
	}
	if strings.TrimSpace(result.Status) != "" {
		extra["status"] = result.Status
	}
	if strings.TrimSpace(result.Error) != "" {
		extra["error"] = result.Error
	}
	if strings.TrimSpace(result.Text) != "" {
		extra["text_bytes"] = len(result.Text)
	}
	hreq := hooks.Request{
		Version:   1,
		Event:     eventName,
		BotID:     req.parentSession.BotID,
		SessionID: req.parentSession.SessionID,
		ChatID:    req.parentSession.ChatID,
		Workspace: hooks.WorkspaceInfo{CWD: hooks.DefaultWorkDir},
		Extra:     extra,
	}
	res, err := p.hookService.Run(ctx, hreq, nil)
	if err != nil {
		return err
	}
	if res.Decision == hooks.DecisionDeny {
		return hooks.ErrDenied
	}
	return nil
}

func (p *SpawnProvider) createAgentSession(ctx context.Context, parent SessionContext, agentID, task string) (agentRecord, error) {
	if p.sessionService == nil {
		return agentRecord{}, errors.New("session service not available")
	}
	sess, err := p.sessionService.Create(ctx, sessionpkg.CreateInput{
		BotID:           parent.BotID,
		Type:            sessionpkg.TypeSubagent,
		Title:           truncateTitle(task, 100),
		ParentSessionID: parent.SessionID,
		CreatedByUserID: strings.TrimSpace(parent.ChannelIdentityID),
		Metadata: map[string]any{
			"agent_id":              agentID,
			"agent_control_version": agentControlVersion,
		},
	})
	if err != nil {
		return agentRecord{}, err
	}
	return agentRecord{
		AgentID:   agentID,
		SessionID: sess.ID,
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}, nil
}

func (p *SpawnProvider) resolveNewAgentID(ctx context.Context, session SessionContext, raw string) (string, error) {
	if strings.TrimSpace(raw) != "" {
		return normalizeAgentID(raw)
	}
	agents, err := p.listAgentRecords(ctx, session)
	if err != nil {
		return "", err
	}
	used := make(map[string]struct{}, len(agents))
	for _, rec := range agents {
		used[rec.AgentID] = struct{}{}
	}
	for i := 1; ; i++ {
		id := "agent_" + strconv.Itoa(i)
		if _, ok := used[id]; !ok {
			return id, nil
		}
	}
}

func (p *SpawnProvider) findAgent(ctx context.Context, session SessionContext, agentID string) (agentRecord, error) {
	agents, err := p.listAgentRecords(ctx, session)
	if err != nil {
		return agentRecord{}, err
	}
	for _, rec := range agents {
		if rec.AgentID == agentID {
			return rec, nil
		}
	}
	return agentRecord{}, fmt.Errorf("%w: %q in current session", errAgentNotFound, agentID)
}

func (p *SpawnProvider) listAgentRecords(ctx context.Context, session SessionContext) ([]agentRecord, error) {
	if p.sessionService == nil {
		return nil, errors.New("session service not available")
	}
	sessions, err := p.sessionService.ListSubagentsByParent(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}
	records := make([]agentRecord, 0, len(sessions))
	for _, sess := range sessions {
		if sess.Type != sessionpkg.TypeSubagent {
			continue
		}
		agentID, _ := sess.Metadata["agent_id"].(string)
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		records = append(records, agentRecord{
			AgentID:   agentID,
			SessionID: sess.ID,
			Title:     sess.Title,
			CreatedAt: sess.CreatedAt,
			UpdatedAt: sess.UpdatedAt,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (p *SpawnProvider) loadAgentMessages(ctx context.Context, sessionID string) []sdk.Message {
	if p.messageService == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	msgs, err := p.messageService.ListBySession(ctx, sessionID)
	if err != nil {
		p.logger.Warn("load subagent messages failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil
	}
	out := make([]sdk.Message, 0, len(msgs))
	for _, msg := range msgs {
		if converted, ok := sdkMessageFromPersisted(msg); ok {
			out = append(out, converted)
		}
	}
	return out
}

func sdkMessageFromPersisted(msg messagepkg.Message) (sdk.Message, bool) {
	var full sdk.Message
	if err := json.Unmarshal(msg.Content, &full); err == nil && (full.Role != "" || len(full.Content) > 0) {
		if full.Role == "" {
			full.Role = sdk.MessageRole(msg.Role)
		}
		return full, true
	}

	var envelope struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(msg.Content, &envelope); err == nil {
		role := envelope.Role
		if role == "" {
			role = msg.Role
		}
		var text string
		if err := json.Unmarshal(envelope.Content, &text); err == nil {
			return sdk.Message{
				Role:    sdk.MessageRole(role),
				Content: []sdk.MessagePart{sdk.TextPart{Text: text}},
			}, true
		}
		wrapped, _ := json.Marshal(struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}{Role: role, Content: envelope.Content})
		if err := json.Unmarshal(wrapped, &full); err == nil {
			return full, true
		}
	}

	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil {
		return sdk.Message{
			Role:    sdk.MessageRole(msg.Role),
			Content: []sdk.MessagePart{sdk.TextPart{Text: text}},
		}, true
	}
	return sdk.Message{}, false
}

func dropLatestMatchingUserMessage(messages []sdk.Message, query string) []sdk.Message {
	needle := strings.TrimSpace(query)
	if needle == "" || len(messages) == 0 {
		return messages
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != sdk.MessageRoleUser {
			continue
		}
		if strings.TrimSpace(messageTextContent(msg)) != needle {
			continue
		}
		out := make([]sdk.Message, 0, len(messages)-1)
		out = append(out, messages[:i]...)
		out = append(out, messages[i+1:]...)
		return out
	}
	return messages
}

func messageTextContent(msg sdk.Message) string {
	var b strings.Builder
	for _, part := range msg.Content {
		if text, ok := part.(sdk.TextPart); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func validateParentSession(session SessionContext) error {
	if strings.TrimSpace(session.BotID) == "" {
		return errors.New("bot_id is required")
	}
	if strings.TrimSpace(session.SessionID) == "" {
		return errors.New("session_id is required")
	}
	return nil
}

func normalizeAgentID(raw string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	if id == "" {
		return "", errors.New("id is required")
	}
	if !agentIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid agent id %q: expected lowercase slug matching %s", raw, agentIDPattern.String())
	}
	return id, nil
}

func agentResultMap(res agentRunResult) map[string]any {
	out := map[string]any{
		"agent_id":   res.AgentID,
		"session_id": res.SessionID,
		"task_id":    res.TaskID,
		"status":     res.Status,
	}
	if res.Message != "" {
		out["message"] = res.Message
	}
	if res.Text != "" {
		out["text"] = res.Text
	}
	if res.Error != "" {
		out["error"] = res.Error
	}
	if res.QueuePosition > 0 {
		out["queue_position"] = res.QueuePosition
	}
	if res.QueueRemaining > 0 {
		out["queue_remaining"] = res.QueueRemaining
	}
	if res.TimedOut {
		out["timed_out"] = true
	}
	return out
}

func (*SpawnProvider) startSpawnHeartbeat(ctx context.Context, session SessionContext, _ int) {
	emitter := session.Emitter
	if emitter == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(spawnHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				emitter(ToolStreamEvent{Type: StreamEventSpawnHeartbeat})
			}
		}
	}()
}

func isRetryableSubagentError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "rate_limit") {
		return true
	}
	if err429Pattern.MatchString(errStr) || serverErrPattern.MatchString(errStr) {
		return true
	}
	if errEOFPattern.MatchString(errStr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (p *SpawnProvider) persistMessages(
	ctx context.Context,
	botID, sessionID, modelID, query string,
	result *SpawnResult,
	includeUser bool,
) {
	if includeUser {
		p.persistUserMessage(ctx, botID, sessionID, query)
	}

	for _, msg := range result.Messages {
		if msg.Role == sdk.MessageRoleUser {
			continue
		}
		content, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var usage json.RawMessage
		if msg.Usage != nil {
			usage, _ = json.Marshal(msg.Usage)
		}
		if _, err := p.messageService.Persist(ctx, messagepkg.PersistInput{
			BotID:     botID,
			SessionID: sessionID,
			Role:      string(msg.Role),
			Content:   content,
			Usage:     usage,
			ModelID:   modelID,
		}); err != nil {
			p.logger.Warn("persist subagent message failed", slog.Any("error", err))
		}
	}
}

func (p *SpawnProvider) persistUserMessage(ctx context.Context, botID, sessionID, query string) bool {
	if p.messageService == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	userContent, _ := json.Marshal(map[string]any{
		"role":    "user",
		"content": query,
	})
	if _, err := p.messageService.Persist(ctx, messagepkg.PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   userContent,
	}); err != nil {
		p.logger.Warn("persist subagent user message failed", slog.Any("error", err))
		return false
	}
	return true
}

// ModelCreator creates an sdk.Model from provider config. Set via SetModelCreator.
type ModelCreator func(modelID, clientType, apiKey, codexAccountID, baseURL string, httpClient *http.Client) *sdk.Model

func (p *SpawnProvider) SetModelCreator(fn ModelCreator) {
	p.modelCreator = fn
}

func (p *SpawnProvider) resolveModel(ctx context.Context, botID string) (*sdk.Model, string, string, error) {
	if p.settings == nil || p.models == nil || p.queries == nil {
		return nil, "", "", errors.New("model resolution services not configured")
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, "", "", err
	}
	chatModelID := strings.TrimSpace(botSettings.ChatModelID)
	if chatModelID == "" {
		return nil, "", "", errors.New("no chat model configured for bot")
	}
	modelInfo, err := p.models.GetByID(ctx, chatModelID)
	if err != nil {
		return nil, "", "", err
	}
	if !modelInfo.Enable {
		return nil, "", "", fmt.Errorf("subagent chat model %s is disabled", modelInfo.ModelID)
	}
	provider, err := models.FetchProviderByID(ctx, p.queries, modelInfo.ProviderID)
	if err != nil {
		return nil, "", "", err
	}
	if p.modelCreator == nil {
		return nil, "", "", errors.New("model creator not configured")
	}
	authResolver := providers.NewService(nil, p.queries, "")
	creds, err := authResolver.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return nil, "", "", err
	}
	sdkModel := p.modelCreator(
		modelInfo.ModelID,
		provider.ClientType,
		creds.APIKey,
		creds.CodexAccountID,
		providers.ProviderConfigString(provider, "base_url"),
		nil,
	)
	cacheTTL := providers.ProviderConfigString(provider, "prompt_cache_ttl")
	return sdkModel, modelInfo.ID, cacheTTL, nil
}

func truncateTitle(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}
