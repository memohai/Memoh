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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
)

// SpawnAgent is the interface the spawn tool uses to run subagent tasks.
// It is satisfied by *agent.Agent and avoids an import cycle.
type SpawnAgent interface {
	Generate(ctx context.Context, cfg SpawnRunConfig) (*SpawnResult, error)
	GenerateWithWatchdog(ctx context.Context, cfg SpawnRunConfig, touchFn func()) (*SpawnResult, error)
}

// SpawnRunConfig mirrors agent.RunConfig fields needed by spawn.
type SpawnRunConfig struct {
	Model           *sdk.Model
	System          string
	Query           string
	SessionType     string
	Identity        SpawnIdentity
	LoopDetection   SpawnLoopConfig
	Messages        []sdk.Message
	ReasoningEffort string
}

// SpawnIdentity mirrors agent.SessionContext fields needed by spawn.
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

// subagentTimeout caps total execution time as a safety net per attempt.
// This prevents runaway subagent calls from blocking the parent agent forever,
// even if the watchdog keeps getting touched (e.g., tiny tokens but no convergence).
const subagentTimeout = 10 * time.Minute

// spawnHeartbeatInterval controls how often a progress event is emitted during
// spawn execution to keep the parent stream's idle timeout from firing.
const spawnHeartbeatInterval = 30 * time.Second

// subagentMaxRetries is the maximum number of retry attempts for a failed
// subagent task. Only transient errors (rate limits, network failures) are
// retried; fatal errors (bad config, invalid input) fail immediately.
const subagentMaxRetries = 3

// subagentRetryBaseDelay is the initial backoff delay between retry attempts.
const subagentRetryBaseDelay = 2 * time.Second

// ErrWatchdogTimedOut is returned when the subagent watchdog fires
// (no activity within the timeout period).
var ErrWatchdogTimedOut = errors.New("subagent watchdog: no activity within timeout")

// subagentWatchdogTimeout is the default inactivity timeout for the watchdog.
const subagentWatchdogTimeout = 3 * time.Minute

var (
	// err429Pattern matches HTTP 429 status codes in error strings.
	err429Pattern = regexp.MustCompile(`(^|[^0-9])429($|[^0-9])`)
	// errEOFPattern matches EOF or connection-level resets.
	errEOFPattern = regexp.MustCompile(`(?i)connection (reset|refused)|EOF$`)
	// serverErrPattern matches "api error 5XX" where XX is any two digits.
	serverErrPattern = regexp.MustCompile(`api error 5\\d{2}`)
)

// SubagentWatchdog implements an activity-based timeout for subagent execution.
// It is "touched" (fed/reset) on each activity signal from the LLM or tools.
// If no touch occurs within the configured timeout, it fires by cancelling
// its associated context.
//
// Lifecycle:
//  1. Call NewSubagentWatchdog to create a watchdog context.
//  2. Call Touch() on each activity signal.
//  3. Call Stop() when the watched operation completes normally.
//
// The watchdog respects parent context cancellation: if the parent context
// is cancelled, the watchdog's context is also cancelled immediately.
type SubagentWatchdog struct {
	timeout time.Duration
	touchCh chan struct{}
	cancel  context.CancelCauseFunc
	done    chan struct{}
	logger  *slog.Logger
}

// NewSubagentWatchdog creates a watchdog that cancels the returned context
// after timeout of inactivity. The returned context is derived from parentCtx.
// If parentCtx is cancelled, the watchdog context is also cancelled.
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

// Touch resets the watchdog timer. It is non-blocking and safe to call
// from any goroutine.
func (w *SubagentWatchdog) Touch() {
	select {
	case w.touchCh <- struct{}{}:
	default:
		// Already a pending touch, no need to queue another.
	}
}

// Stop terminates the watchdog goroutine and releases resources.
// Call this when the watched operation completes normally.
func (w *SubagentWatchdog) Stop() {
	w.cancel(context.Canceled)
	<-w.done
}

// run is the watchdog loop. It watches for touches and fires if none arrive
// within the configured timeout.
func (w *SubagentWatchdog) run(ctx context.Context) {
	defer close(w.done)

	timer := time.NewTimer(w.timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			// Parent cancelled or Stop() called.
			return
		case <-w.touchCh:
			// Activity detected; reset the timer.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.timeout)
		case <-timer.C:
			// No activity within timeout -- fire!
			w.logger.Warn("subagent watchdog fired",
				slog.Duration("timeout", w.timeout),
			)
			w.cancel(ErrWatchdogTimedOut)
			return
		}
	}
}

// maxTasksPerSpawn caps the number of tasks accepted in a single spawn call.
const maxTasksPerSpawn = 5

// maxSpawnCallsPerSession caps the total number of spawn tool calls within
// a single agent session to prevent subagent storms.
const maxSpawnCallsPerSession = 3

// SpawnProvider exposes a "spawn" tool that runs one or more subagent tasks
// concurrently and returns results to the parent agent.
type SpawnProvider struct {
	agent          SpawnAgent
	settings       *settings.Service
	models         *models.Service
	queries        *sqlc.Queries
	sessionService *sessionpkg.Service
	messageService messagepkg.Writer
	systemPromptFn func(sessionType string) string
	modelCreator   ModelCreator
	logger         *slog.Logger
}

// NewSpawnProvider creates a SpawnProvider. The agent must be injected later
// via SetAgent to avoid a dependency cycle.
func NewSpawnProvider(
	log *slog.Logger,
	settingsSvc *settings.Service,
	modelsSvc *models.Service,
	queries *sqlc.Queries,
	sessionService *sessionpkg.Service,
) *SpawnProvider {
	if log == nil {
		log = slog.Default()
	}
	return &SpawnProvider{
		settings:       settingsSvc,
		models:         modelsSvc,
		queries:        queries,
		sessionService: sessionService,
		logger:         log.With(slog.String("tool", "spawn")),
	}
}

// SetAgent injects the agent after construction (breaking the DI cycle).
func (p *SpawnProvider) SetAgent(a SpawnAgent) {
	p.agent = a
}

// SetMessageService injects an optional message writer for persisting
// subagent conversation history.
func (p *SpawnProvider) SetMessageService(w messagepkg.Writer) {
	p.messageService = w
}

// SetSystemPromptFunc injects the function used to generate the system prompt
// (typically agent.GenerateSystemPrompt).
func (p *SpawnProvider) SetSystemPromptFunc(fn func(sessionType string) string) {
	p.systemPromptFn = fn
}

func (p *SpawnProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.agent == nil {
		return nil, nil
	}
	sess := session
	spawnCount := new(int32)
	return []sdk.Tool{
		{
			Name:        "spawn",
			Description: fmt.Sprintf("Spawn one or more subagents to work on tasks in parallel. Each task runs in its own context with file, exec, and web tools. All results are returned together. Max %d tasks per call, max %d calls per session.", maxTasksPerSpawn, maxSpawnCallsPerSession),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tasks": map[string]any{
						"type":        "array",
						"description": fmt.Sprintf("List of task instructions. Each string is a self-contained prompt for one subagent. Max %d tasks.", maxTasksPerSpawn),
						"items":       map[string]any{"type": "string"},
					},
				},
				"required": []string{"tasks"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSpawn(ctx.Context, sess, inputAsMap(input), spawnCount)
			},
		},
	}, nil
}

type spawnResult struct {
	Task      string `json:"task"`
	SessionID string `json:"session_id,omitempty"`
	Text      string `json:"text"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func (p *SpawnProvider) execSpawn(ctx context.Context, session SessionContext, args map[string]any, spawnCount *int32) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}

	// Enforce per-session spawn call limit.
	current := atomic.AddInt32(spawnCount, 1)
	if current > maxSpawnCallsPerSession {
		return map[string]any{
			"isError": true,
			"content": []map[string]any{{
				"type": "text",
				"text": fmt.Sprintf("Spawn limit reached: max %d spawn calls per session (already made %d). Consolidate your remaining work into the current agent context instead of spawning more subagents.", maxSpawnCallsPerSession, current-1),
			}},
		}, nil
	}

	tasksRaw, ok := args["tasks"]
	if !ok {
		return nil, errors.New("tasks is required")
	}
	tasks, err := toStringSlice(tasksRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, errors.New("at least one task is required")
	}
	// Cap tasks per call.
	if len(tasks) > maxTasksPerSpawn {
		p.logger.Warn("spawn tasks capped",
			slog.Int("requested", len(tasks)),
			slog.Int("max", maxTasksPerSpawn),
		)
		tasks = tasks[:maxTasksPerSpawn]
	}

	// Use a decoupled context for model resolution and subagent execution
	// so that a parent stream cancellation (e.g. idle timeout) does not
	// prevent the spawn from completing and returning its results.
	sessionCtx := context.WithoutCancel(ctx)

	sdkModel, modelID, err := p.resolveModel(sessionCtx, botID)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}

	systemPrompt := ""
	if p.systemPromptFn != nil {
		systemPrompt = p.systemPromptFn(sessionpkg.TypeSubagent)
	}

	results := make([]spawnResult, len(tasks))
	var wg sync.WaitGroup
	wg.Add(len(tasks))

	// Start a heartbeat goroutine that emits progress events into the
	// parent stream at regular intervals. This keeps the stream's idle
	// timeout from firing while subagents are running.
	heartbeatCtx, heartbeatCancel := context.WithCancel(sessionCtx)
	defer heartbeatCancel()
	p.startSpawnHeartbeat(heartbeatCtx, session, len(tasks))

	for i, task := range tasks {
		go func(idx int, query string) {
			defer wg.Done()
			results[idx] = p.runSubagentTask(sessionCtx, session, sdkModel, modelID, systemPrompt, query)
		}(i, task)
	}
	wg.Wait()

	return map[string]any{"results": results}, nil
}

// startSpawnHeartbeat emits periodic progress events into the parent agent
// stream to prevent the idle timeout from firing while spawn tasks run.
// Each heartbeat carries a progress status so the frontend can display it.
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
				// Emit a progress event through the agent's stream emitter.
				// The agent framework converts ToolStreamEvent into the
				// appropriate wire-level progress event, which resets the
				// idle timeout timer in the resolver.
				emitter(ToolStreamEvent{
					Type: StreamEventSpawnHeartbeat,
				})
			}
		}
	}()
}

func (p *SpawnProvider) runSubagentTask(
	ctx context.Context,
	parentSession SessionContext,
	model *sdk.Model,
	modelID string,
	systemPrompt string,
	query string,
) spawnResult {
	res := spawnResult{Task: query}

	var sessionID string
	if p.sessionService != nil {
		sess, err := p.sessionService.Create(context.WithoutCancel(ctx), sessionpkg.CreateInput{
			BotID:           parentSession.BotID,
			Type:            sessionpkg.TypeSubagent,
			Title:           truncateTitle(query, 100),
			ParentSessionID: parentSession.SessionID,
		})
		if err != nil {
			p.logger.Warn("failed to create subagent session", slog.Any("error", err))
		} else {
			sessionID = sess.ID
			res.SessionID = sessionID
		}
	}

	cfg := SpawnRunConfig{
		Model:       model,
		System:      systemPrompt,
		Query:       query,
		SessionType: sessionpkg.TypeSubagent,
		Identity: SpawnIdentity{
			BotID:             parentSession.BotID,
			ChatID:            parentSession.ChatID,
			SessionID:         sessionID,
			ChannelIdentityID: parentSession.ChannelIdentityID,
			CurrentPlatform:   parentSession.CurrentPlatform,
			SessionToken:      parentSession.SessionToken,
			IsSubagent:        true,
		},
		LoopDetection: SpawnLoopConfig{Enabled: true},
	}

	var lastErr error
	for attempt := 0; attempt <= subagentMaxRetries; attempt++ {
		if attempt > 0 {
			delay := subagentRetryBaseDelay * time.Duration(attempt)
			p.logger.Info("subagent retry",
				slog.String("session_id", sessionID),
				slog.Int("attempt", attempt),
				slog.Duration("delay", delay),
				slog.String("error", lastErr.Error()),
			)
			delayTimer := time.NewTimer(delay)
			deadlineTimer := time.NewTimer(subagentTimeout)
			select {
			case <-delayTimer.C:
				deadlineTimer.Stop()
			case <-deadlineTimer.C:
				delayTimer.Stop()
				// Hard deadline: don't retry indefinitely.
				res.Error = fmt.Sprintf("retry deadline exceeded (last error: %v)", lastErr)
				return res
			}
		}

		// Create a two-layer timeout per attempt:
		// 1. Safety net: wall-clock timeout (subagentTimeout) via context.WithTimeout.
		// 2. Watchdog: activity-based timeout (subagentWatchdogTimeout) that fires
		//    when no stream events (tokens, tool output) are received.
		// Use context.WithoutCancel so retries get a fresh timeout even if
		// the parent stream was cancelled (e.g. by idle timeout).
		safetyCtx, safetyCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentTimeout)
		wdCtx, wd := NewSubagentWatchdog(safetyCtx, subagentWatchdogTimeout, p.logger)

		genResult, err := p.agent.GenerateWithWatchdog(wdCtx, cfg, wd.Touch)
		wd.Stop()
		safetyCancel()

		if err == nil {
			res.Text = genResult.Text
			res.Success = true
			if p.messageService != nil && sessionID != "" {
				p.persistMessages(context.WithoutCancel(ctx), parentSession.BotID, sessionID, modelID, query, genResult)
			}
			return res
		}

		lastErr = err

		// Check if the true parent context was cancelled (not watchdog, not safety timeout).
		// If the parent is done, don't retry.
		if ctx.Err() != nil && !errors.Is(err, ErrWatchdogTimedOut) {
			res.Error = fmt.Sprintf("parent cancelled: %v", ctx.Err())
			return res
		}

		// Watchdog timeouts are always retryable.
		if errors.Is(err, ErrWatchdogTimedOut) {
			p.logger.Warn("subagent watchdog fired, will retry",
				slog.String("session_id", sessionID),
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", subagentMaxRetries+1),
			)
			continue
		}

		if !isRetryableSubagentError(err) {
			res.Error = err.Error()
			return res
		}
	}

	p.logger.Warn("subagent failed after all retries",
		slog.String("session_id", sessionID),
		slog.Int("attempts", subagentMaxRetries+1),
		slog.String("error", lastErr.Error()),
	)
	res.Error = fmt.Sprintf("all %d attempts failed (last: %v)", subagentMaxRetries+1, lastErr)
	return res
}

// isRetryableSubagentError returns true for transient errors that warrant a retry.
// Fatal errors (invalid config, context cancelled by user) return false.
func isRetryableSubagentError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Rate limits
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "rate_limit") {
		return true
	}
	// HTTP 429 and 5xx
	if err429Pattern.MatchString(errStr) || serverErrPattern.MatchString(errStr) {
		return true
	}
	// Connection-level errors
	if errEOFPattern.MatchString(errStr) {
		return true
	}
	// Network timeouts
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Context cancellation from parent (idle timeout, etc.) IS retryable
	// for subagents — they should complete their work even if the parent
	// stream was interrupted.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

func (p *SpawnProvider) persistMessages(
	ctx context.Context,
	botID, sessionID, modelID, query string,
	result *SpawnResult,
) {
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

// ModelCreator creates an sdk.Model from provider config. Set via SetModelCreator.
type ModelCreator func(modelID, clientType, apiKey, codexAccountID, baseURL string, httpClient *http.Client) *sdk.Model

// SetModelCreator injects the function used to create SDK models
// (typically agent.CreateModel wrapped to match the signature).
func (p *SpawnProvider) SetModelCreator(fn ModelCreator) {
	p.modelCreator = fn
}

func (p *SpawnProvider) resolveModel(ctx context.Context, botID string) (*sdk.Model, string, error) {
	if p.settings == nil || p.models == nil || p.queries == nil {
		return nil, "", errors.New("model resolution services not configured")
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	chatModelID := strings.TrimSpace(botSettings.ChatModelID)
	if chatModelID == "" {
		return nil, "", errors.New("no chat model configured for bot")
	}
	modelInfo, err := p.models.GetByID(ctx, chatModelID)
	if err != nil {
		return nil, "", err
	}
	provider, err := models.FetchProviderByID(ctx, p.queries, modelInfo.ProviderID)
	if err != nil {
		return nil, "", err
	}
	if p.modelCreator == nil {
		return nil, "", errors.New("model creator not configured")
	}
	authResolver := providers.NewService(nil, p.queries, "")
	creds, err := authResolver.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return nil, "", err
	}
	sdkModel := p.modelCreator(
		modelInfo.ModelID,
		provider.ClientType,
		creds.APIKey,
		creds.CodexAccountID,
		providers.ProviderConfigString(provider, "base_url"),
		nil,
	)
	return sdkModel, modelInfo.ID, nil
}

func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result = append(result, s)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", v)
	}
}

func truncateTitle(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}
