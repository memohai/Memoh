package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/models"
)

// errEmptySummary marks a completed LLM call that produced no usable summary
// text. The compacted rows must stay reclaimable, so this must never reach
// MarkMessagesCompacted.
var errEmptySummary = errors.New("compaction: model returned an empty summary")

// compactionFailureCooldown bounds how often a session may retry compaction
// after a failure, so a persistently failing model can't burn an LLM call on
// every blocking sync backstop or async trigger.
const compactionFailureCooldown = 5 * time.Minute

// inflightRun is the completion signal for one session's running compaction.
// done closes after res/err are set, so concurrent sync callers can wait for
// the owner and reuse its outcome instead of skipping or double-running.
type inflightRun struct {
	done    chan struct{}
	waiters atomic.Int32
	res     Result
	err     error
}

// Service manages context compaction for bot conversations.
type Service struct {
	queries     dbstore.Queries
	hookService *hooks.Service
	logger      *slog.Logger
	nowFn       func() time.Time

	inflightMu sync.Mutex
	inflight   map[string]*inflightRun
	failedAt   map[string]time.Time
}

// NewService creates a new compaction Service.
func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	return &Service{
		queries:  queries,
		logger:   log,
		nowFn:    time.Now,
		inflight: make(map[string]*inflightRun),
		failedAt: make(map[string]time.Time),
	}
}

// beginSessionCompaction marks a session as having a compaction in flight.
// Overlapping runs would select overlapping candidate sets and race
// MarkMessagesCompacted, so only one compaction may run per session at a time;
// a busy session returns the owner's run for callers that want to wait on it.
func (s *Service) beginSessionCompaction(sessionID string) (*inflightRun, bool) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if owner, busy := s.inflight[sessionID]; busy {
		return owner, false
	}
	run := &inflightRun{done: make(chan struct{})}
	s.inflight[sessionID] = run
	return run, true
}

func (s *Service) endSessionCompaction(sessionID string, run *inflightRun, res Result, err error) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	run.res, run.err = res, err
	close(run.done)
	delete(s.inflight, sessionID)
}

// inFailureCooldown reports whether sessionID failed compaction recently
// enough that a new attempt should be skipped. An entry found expired is
// deleted so failedAt does not accumulate sessions that never run again.
func (s *Service) inFailureCooldown(sessionID string) bool {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	failedAt, ok := s.failedAt[sessionID]
	if !ok {
		return false
	}
	if s.nowFn().Sub(failedAt) >= compactionFailureCooldown {
		delete(s.failedAt, sessionID)
		return false
	}
	return true
}

func (s *Service) recordCompactionFailure(sessionID string) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	s.failedAt[sessionID] = s.nowFn()
}

func (s *Service) clearCompactionFailure(sessionID string) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	delete(s.failedAt, sessionID)
}

func (s *Service) SetHookService(h *hooks.Service) {
	s.hookService = h
}

// ShouldCompact returns true if inputTokens exceeds the threshold.
func ShouldCompact(inputTokens, threshold int) bool {
	return threshold > 0 && inputTokens >= threshold
}

// TriggerCompaction runs compaction in the background. A session with a run
// already in flight is skipped: the async trigger fires per request, so the
// next turn re-evaluates the demand.
func (s *Service) TriggerCompaction(ctx context.Context, cfg TriggerConfig) {
	go func() {
		bgCtx := context.WithoutCancel(ctx)
		if _, _, err := s.runCompaction(bgCtx, cfg); err != nil {
			s.logger.Error("compaction failed", slog.String("bot_id", cfg.BotID), slog.String("session_id", cfg.SessionID), slog.String("error", err.Error()))
		}
	}()
}

// RunCompactionSync runs compaction synchronously and reports this session's
// scoped Result, so callers act on their own outcome (a noop keeps their
// current context) instead of reading an unscoped bot-wide log that may belong
// to another session. When another run for the session is already in flight,
// the sync path waits for the owner and reuses its outcome — the summary is
// seconds away, and waiting removes a duplicate LLM call over the same span; a
// canceled wait degrades to a noop.
func (s *Service) RunCompactionSync(ctx context.Context, cfg TriggerConfig) (Result, error) {
	for {
		res, owner, err := s.runCompaction(ctx, cfg)
		if owner == nil {
			return res, err
		}
		res, retry, err := waitForOwner(ctx, cfg, owner)
		if !retry {
			return res, err
		}
	}
}

// waitForOwner blocks until the in-flight owner publishes its outcome or the
// caller's context ends, reporting whether the caller should try again. Only
// a manual request retries, and only through an owner that nooped without an
// error — typically an automatic run skipped by the failure cooldown that the
// manual request must still bypass. A canceled caller never retries: that
// would start new side effects after cancellation.
func waitForOwner(ctx context.Context, cfg TriggerConfig, owner *inflightRun) (Result, bool, error) {
	owner.waiters.Add(1)
	select {
	case <-owner.done:
		if cfg.Manual && owner.err == nil && owner.res.Status == StatusNoop {
			if ctx.Err() != nil {
				return Result{Status: StatusNoop}, false, nil
			}
			return Result{}, true, nil
		}
		return owner.res, false, owner.err
	case <-ctx.Done():
		return Result{Status: StatusNoop}, false, nil
	}
}

func (s *Service) runCompaction(ctx context.Context, cfg TriggerConfig) (Result, *inflightRun, error) {
	// Attaching to an existing owner wins over the cooldown check: the
	// cooldown exists to stop new failing runs, not to hide a run already in
	// flight (a manual retry may be the owner precisely because it bypassed
	// the cooldown, and sync callers should reuse its result).
	run, ok := s.beginSessionCompaction(cfg.SessionID)
	if !ok {
		s.logger.Info("compaction: already in flight for session",
			slog.String("bot_id", cfg.BotID),
			slog.String("session_id", cfg.SessionID),
		)
		return Result{Status: StatusNoop}, run, nil
	}

	var compactRes Result
	var compactErr error
	defer func() {
		s.endSessionCompaction(cfg.SessionID, run, compactRes, compactErr)
	}()

	// Manual (user-initiated) compaction bypasses the cooldown: the user may
	// have just fixed the failing model, and a silent skip would report success
	// while nothing runs. Automatic per-request paths still honor the cooldown.
	if !cfg.Manual && s.inFailureCooldown(cfg.SessionID) {
		s.logger.Info("compaction: session in failure cooldown, skipping",
			slog.String("bot_id", cfg.BotID),
			slog.String("session_id", cfg.SessionID),
		)
		compactRes = Result{Status: StatusNoop}
		return compactRes, nil, nil
	}

	preHookRan := false
	defer func() {
		if r := recover(); r != nil {
			// A panicking run — the pre-hook included — must not publish a
			// zero-value success to waiters or clear the cooldown as if it
			// succeeded.
			compactErr = fmt.Errorf("compaction panicked: %v", r)
			compactRes = Result{}
			s.recordCompactionFailure(cfg.SessionID)
			panic(r)
		}
		switch {
		case compactErr == nil:
			s.clearCompactionFailure(cfg.SessionID)
		case !preHookRan:
			// A pre-hook error or deny is bot policy, not a model failure;
			// it must not arm the cooldown (panics above still do).
		case ctx.Err() != nil:
			// The caller's request was canceled or hit its deadline — not a
			// model failure. Arming the five-minute cooldown here would
			// silence auto-compaction for a healthy session just because the
			// user aborted one request. A timeout with the caller's context
			// still live (e.g. the HTTP client's own timeout on a model too
			// slow to summarize) is a real failure and still arms it.
		default:
			s.recordCompactionFailure(cfg.SessionID)
		}
		if !preHookRan {
			return
		}
		s.runPostCompactHook(context.WithoutCancel(ctx), cfg, compactErr)
	}()

	if err := s.runCompactionHook(ctx, hooks.EventPreCompact, cfg, nil); err != nil {
		compactErr = err
		return Result{}, nil, err
	}
	preHookRan = true
	botUUID, err := db.ParseUUID(cfg.BotID)
	if err != nil {
		compactErr = err
		return Result{}, nil, compactErr
	}
	sessionUUID, err := db.ParseUUID(cfg.SessionID)
	if err != nil {
		compactErr = err
		return Result{}, nil, compactErr
	}

	compactRes, compactErr = s.doCompaction(ctx, botUUID, sessionUUID, cfg)
	return compactRes, nil, compactErr
}

// runPostCompactHook dispatches PostCompact after the compaction outcome is
// already decided and published state is settled. Post-hook failures are
// advisory, so an error only logs a warning — and a panic is contained here:
// letting it escape would blow past the recovery defer's already-spent
// recover(), crash async triggers, and publish the pre-panic result to
// waiters as if nothing happened.
func (s *Service) runPostCompactHook(ctx context.Context, cfg TriggerConfig, compactErr error) {
	defer func() {
		if r := recover(); r != nil && s.logger != nil {
			s.logger.Warn("post compaction hook panicked", slog.String("bot_id", cfg.BotID), slog.Any("panic", r))
		}
	}()
	extra := map[string]any{}
	if compactErr != nil {
		extra["error"] = compactErr.Error()
	}
	if err := s.runCompactionHook(ctx, hooks.EventPostCompact, cfg, extra); err != nil && s.logger != nil {
		s.logger.Warn("post compaction hook failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
	}
}

func (s *Service) runCompactionHook(ctx context.Context, eventName string, cfg TriggerConfig, extra map[string]any) error {
	if s == nil || s.hookService == nil {
		return nil
	}
	payload := map[string]any{
		"input_tokens":  cfg.TotalInputTokens,
		"target_tokens": cfg.TargetTokens,
		"ratio":         cfg.Ratio,
		"model_id":      cfg.ModelID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	req := hooks.Request{
		Version:   1,
		Event:     eventName,
		BotID:     cfg.BotID,
		SessionID: cfg.SessionID,
		Workspace: hooks.WorkspaceInfo{
			CWD: hooks.DefaultWorkDir,
		},
		Turn: payload,
	}
	res, err := s.hookService.Run(ctx, req, nil)
	if err != nil {
		return err
	}
	if res.Decision == hooks.DecisionDeny {
		return hooks.ErrDenied
	}
	return nil
}

func (s *Service) doCompaction(ctx context.Context, botUUID pgtype.UUID, sessionUUID pgtype.UUID, cfg TriggerConfig) (Result, error) {
	rows, err := s.queries.ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		return Result{}, err
	}
	if len(rows) == 0 {
		return Result{Status: StatusNoop}, nil
	}

	messages, barrierCount := itemsFromRows(rows)
	if barrierCount > 0 {
		s.logger.Warn("compaction: kept unparseable history rows as span barriers",
			slog.Int("barrier_count", barrierCount),
			slog.String("session_id", cfg.SessionID),
		)
	}
	if len(messages) == 0 {
		return Result{Status: StatusNoop}, nil
	}

	var toCompact []CompactionCandidate
	if cfg.TargetTokens > 0 {
		// Sync compaction: compress enough messages to bring context
		// down to TargetTokens. Calculate how many tokens to keep
		// (newest messages) and compact everything older.
		toCompact = splitByTarget(messages, cfg.TargetTokens)
	} else {
		toCompact = splitByRatio(messages, cfg.TotalInputTokens, cfg.Ratio)
	}
	if len(toCompact) == 0 {
		return Result{Status: StatusNoop}, nil
	}

	// Cap the compaction input to avoid exceeding the compaction model's
	// context window. MaxCompactTokens is typically set to 90% of the model's
	// window. If not set, use a conservative default of 30K tokens. Prior
	// summaries and message entries share this one budget — an additive prior
	// allowance would let the combined prompt exceed the window headroom.
	maxCompactTokens := cfg.MaxCompactTokens
	if maxCompactTokens <= 0 {
		maxCompactTokens = 30000
	}

	priorLogs, err := s.queries.ListActiveCompactionArtifactsBySession(ctx, sessionUUID)
	if err != nil {
		return Result{}, err
	}
	var priorSummaries []string
	for _, l := range priorLogs {
		if strings.TrimSpace(l.Summary) != "" {
			priorSummaries = append(priorSummaries, l.Summary)
		}
	}
	priorSummaries = capPriorSummaries(priorSummaries, maxCompactTokens/4)
	priorTokens := priorContextTokens(priorSummaries)
	// capPriorSummaries always keeps the newest summary, so a single oversized
	// one can exceed its allowance; floor the entries budget at half the total
	// so compaction keeps making progress.
	entriesBudget := maxCompactTokens - priorTokens
	if entriesBudget < maxCompactTokens/2 {
		entriesBudget = maxCompactTokens / 2
	}

	s.logger.Info("compaction: before trim",
		slog.Int("messages", len(toCompact)),
		slog.Int("total_uncompacted", len(messages)),
		slog.Int("max_compact_tokens", maxCompactTokens),
		slog.Int("prior_context_tokens", priorTokens),
	)
	toCompact = trimCompactMessages(toCompact, entriesBudget)
	// The progress guarantee may keep one oversized markable group past the
	// entries budget; the prior context is reference-only, so shrink it (down
	// to nothing) before letting the combined prompt exceed MaxCompactTokens.
	if entriesCost := markableCompactCost(toCompact); entriesCost+priorTokens > maxCompactTokens {
		priorSummaries = capPriorSummaries(priorSummaries, maxCompactTokens-entriesCost)
		priorTokens = priorContextTokens(priorSummaries)
	}
	s.logger.Info("compaction: after trim",
		slog.Int("messages", len(toCompact)),
		slog.Int("prior_summaries", len(priorSummaries)),
	)

	entries, compactedMessageIDs := buildEntriesAndIDs(toCompact)
	if len(entries) == 0 || len(compactedMessageIDs) == 0 {
		// No complete group survived: every selected group had a row that rendered
		// empty (a reasoning-only message, or a tool exchange whose result renders
		// empty). buildEntriesAndIDs withholds such a group from both entries and
		// ids, so summarizing here would either destroy rows for a junk summary or
		// mark rows we cannot faithfully summarize. Leave them in raw history.
		return Result{Status: StatusNoop}, nil
	}
	assetRows, err := s.queries.ListMessageAssetsBatch(ctx, compactedMessageIDs)
	if err != nil {
		return Result{}, fmt.Errorf("load compaction message assets: %w", err)
	}
	toCompact, err = candidatesWithAssets(toCompact, rows, assetRows)
	if err != nil {
		return Result{}, err
	}
	artifact, err := artifactMetadataFor(toCompact, compactedMessageIDs)
	if err != nil {
		return Result{}, err
	}

	// A single markable group larger than the whole budget survives trim by
	// design (progress guarantee); truncate its rendered entries rather than
	// send a prompt the model rejects on every pass. Entry floors can still
	// exceed the budget when that group holds enough rows, so recheck and
	// surface the overshoot instead of claiming an unconditional cap.
	entries = capEntriesToBudget(entries, maxCompactTokens-priorTokens)
	if cost := entriesPromptCost(entries); cost+priorTokens > maxCompactTokens {
		s.logger.Warn("compaction: entry floors exceed the budget, prompt may overflow the compaction window",
			slog.Int("entries", len(entries)),
			slog.Int("entry_tokens", cost),
			slog.Int("max_compact_tokens", maxCompactTokens),
			slog.String("session_id", cfg.SessionID),
		)
	}

	userPrompt := buildUserPrompt(priorSummaries, entries)

	model := models.NewSDKChatModel(models.SDKModelConfig{
		ClientType:     cfg.ClientType,
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		CodexAccountID: cfg.CodexAccountID,
		ModelID:        cfg.ModelID,
		HTTPClient:     cfg.HTTPClient,
	})

	systemPromptDecorated, sdkMessages, _ := models.ApplyPromptCache(
		model, cfg.PromptCacheTTL,
		systemPrompt, []sdk.Message{sdk.UserMessage(userPrompt)}, nil,
	)

	// The log row is created only once a real attempt starts, so no-op runs do
	// not accumulate rows. Persistence uses a non-cancellable context: a client
	// disconnect mid-run must not strand rows marked compacted without a
	// completed summary.
	persistCtx := context.WithoutCancel(ctx)
	logRow, err := s.queries.CreateCompactionLog(persistCtx, sqlc.CreateCompactionLogParams{
		BotID:     botUUID,
		SessionID: sessionUUID,
	})
	if err != nil {
		return Result{}, err
	}
	logID := logRow.ID

	result, err := sdk.GenerateTextResult(ctx,
		sdk.WithModel(model),
		sdk.WithSystem(systemPromptDecorated),
		sdk.WithMessages(sdkMessages),
	)
	if err != nil {
		_ = s.completeLog(persistCtx, logID, "error", "", err.Error(), 0, nil, pgtype.UUID{}, nil)
		return Result{}, err
	}

	if strings.TrimSpace(result.Text) == "" {
		_ = s.completeLog(persistCtx, logID, "error", "", errEmptySummary.Error(), 0, nil, pgtype.UUID{}, nil)
		return Result{}, errEmptySummary
	}

	usageJSON, _ := json.Marshal(result.Usage)

	modelUUID := db.ParseUUIDOrEmpty(cfg.ModelID)
	if err := s.queries.MarkMessagesCompacted(persistCtx, sqlc.MarkMessagesCompactedParams{
		CompactID: logID,
		Column2:   compactedMessageIDs,
	}); err != nil {
		_ = s.completeLog(persistCtx, logID, "error", "", err.Error(), 0, nil, pgtype.UUID{}, nil)
		return Result{}, err
	}

	if err := s.completeLog(persistCtx, logID, "ok", result.Text, "", len(compactedMessageIDs), usageJSON, modelUUID, &artifact); err != nil {
		// The rows are already marked, but the log never reached status=ok, so
		// the reclaim SQL keeps them eligible for a later pass. Reporting ok
		// here would claim a summary that was never persisted.
		return Result{}, err
	}
	return Result{Status: StatusOK, Summary: result.Text, MessageCount: len(compactedMessageIDs)}, nil
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, summary, errMsg string, messageCount int, usage []byte, modelID pgtype.UUID, artifact *artifactMetadata) error {
	coverage := []byte("[]")
	var anchorStartMs, anchorEndMs int64
	if artifact != nil {
		coverage = artifact.Coverage
		anchorStartMs = artifact.AnchorStartMs
		anchorEndMs = artifact.AnchorEndMs
	}
	_, err := s.queries.CompleteCompactionLog(ctx, sqlc.CompleteCompactionLogParams{
		ID:            logID,
		Status:        status,
		Summary:       summary,
		MessageCount:  int32(messageCount), //nolint:gosec // count always small
		ErrorMessage:  errMsg,
		Usage:         usage,
		ModelID:       modelID,
		Coverage:      coverage,
		AnchorStartMs: anchorStartMs,
		AnchorEndMs:   anchorEndMs,
	})
	if err != nil {
		s.logger.Error("failed to complete compaction log", slog.String("error", err.Error()))
		return err
	}
	return nil
}

// ListLogs returns paginated compaction logs for a bot.
func (s *Service) ListLogs(ctx context.Context, botID string, limit, offset int) ([]Log, int64, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, 0, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := s.queries.CountCompactionLogsByBot(ctx, botUUID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.queries.ListCompactionLogsByBot(ctx, sqlc.ListCompactionLogsByBotParams{
		BotID:  botUUID,
		Limit:  int32(limit),  //nolint:gosec // clamped above
		Offset: int32(offset), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, 0, err
	}

	logs := make([]Log, len(rows))
	for i, r := range rows {
		logs[i] = toLog(r)
	}
	return logs, total, nil
}

// DeleteLogs deletes all compaction logs for a bot.
func (s *Service) DeleteLogs(ctx context.Context, botID string) error {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteCompactionLogsByBot(ctx, botUUID)
}

func toLog(r sqlc.BotHistoryMessageCompact) Log {
	l := Log{
		ID:           formatUUID(r.ID),
		BotID:        formatUUID(r.BotID),
		SessionID:    formatUUID(r.SessionID),
		Status:       r.Status,
		Summary:      r.Summary,
		MessageCount: int(r.MessageCount),
		ErrorMessage: r.ErrorMessage,
		ModelID:      formatUUID(r.ModelID),
		StartedAt:    r.StartedAt.Time,
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		l.CompletedAt = &t
	}
	if len(r.Usage) > 0 {
		var u any
		if json.Unmarshal(r.Usage, &u) == nil {
			l.Usage = u
		}
	}
	return l
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
