package compaction

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/models"
)

// Service manages context compaction for bot conversations.
type Service struct {
	queries     dbstore.Queries
	hookService *hooks.Service
	logger      *slog.Logger
}

// NewService creates a new compaction Service.
func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log,
	}
}

func (s *Service) SetHookService(h *hooks.Service) {
	s.hookService = h
}

// ShouldCompact returns true if inputTokens exceeds the threshold.
func ShouldCompact(inputTokens, threshold int) bool {
	return threshold > 0 && inputTokens >= threshold
}

// TriggerCompaction runs compaction in the background.
func (s *Service) TriggerCompaction(ctx context.Context, cfg TriggerConfig) {
	go func() {
		bgCtx := context.WithoutCancel(ctx)
		if err := s.runCompaction(bgCtx, cfg); err != nil {
			s.logger.Error("compaction failed", slog.String("bot_id", cfg.BotID), slog.String("session_id", cfg.SessionID), slog.String("error", err.Error()))
		}
	}()
}

// RunCompactionSync runs compaction synchronously and returns any error.
func (s *Service) RunCompactionSync(ctx context.Context, cfg TriggerConfig) error {
	return s.runCompaction(ctx, cfg)
}

func (s *Service) runCompaction(ctx context.Context, cfg TriggerConfig) error {
	if err := s.runCompactionHook(ctx, hooks.EventPreCompact, cfg, nil); err != nil {
		return err
	}
	var compactErr error
	defer func() {
		extra := map[string]any{}
		if compactErr != nil {
			extra["error"] = compactErr.Error()
		}
		if err := s.runCompactionHook(context.WithoutCancel(ctx), hooks.EventPostCompact, cfg, extra); err != nil && s.logger != nil {
			s.logger.Warn("post compaction hook failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		}
	}()
	botUUID, err := db.ParseUUID(cfg.BotID)
	if err != nil {
		compactErr = err
		return compactErr
	}
	sessionUUID, err := db.ParseUUID(cfg.SessionID)
	if err != nil {
		compactErr = err
		return compactErr
	}
	teamID, err := teamIDFromContext(ctx)
	if err != nil {
		compactErr = err
		return compactErr
	}

	logRow, err := s.queries.CreateCompactionLog(ctx, withTeamID(sqlc.CreateCompactionLogParams{
		BotID:     botUUID,
		SessionID: sessionUUID,
	}, teamID))
	if err != nil {
		compactErr = err
		return compactErr
	}

	compactErr = s.doCompaction(ctx, logRow.ID, sessionUUID, cfg)
	if compactErr != nil {
		s.completeLog(ctx, logRow.ID, "error", "", compactErr.Error(), 0, nil, pgtype.UUID{})
	}
	return compactErr
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

func (s *Service) doCompaction(ctx context.Context, logID pgtype.UUID, sessionUUID pgtype.UUID, cfg TriggerConfig) error {
	teamID, err := teamIDFromContext(ctx)
	if err != nil {
		return err
	}
	messages, err := s.queries.ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		s.completeLog(ctx, logID, "ok", "", "", 0, nil, pgtype.UUID{})
		return nil
	}

	var toCompact []sqlc.ListUncompactedMessagesBySessionRow
	if cfg.TargetTokens > 0 {
		// Sync compaction: compress enough messages to bring context
		// down to TargetTokens. Calculate how many tokens to keep
		// (newest messages) and compact everything older.
		toCompact = splitByTarget(messages, cfg.TargetTokens)
	} else {
		toCompact = splitByRatio(messages, cfg.TotalInputTokens, cfg.Ratio)
	}
	if len(toCompact) == 0 {
		s.completeLog(ctx, logID, "ok", "", "", 0, nil, pgtype.UUID{})
		return nil
	}

	// Cap the compaction input to avoid exceeding the compaction model's
	// context window. MaxCompactTokens is typically set to 90% of the model's
	// window. If not set, use a conservative default of 30K tokens.
	maxCompactTokens := cfg.MaxCompactTokens
	if maxCompactTokens <= 0 {
		maxCompactTokens = 30000
	}
	s.logger.Info("compaction: before trim",
		slog.Int("messages", len(toCompact)),
		slog.Int("total_uncompacted", len(messages)),
		slog.Int("max_compact_tokens", maxCompactTokens),
	)
	toCompact = trimCompactMessages(toCompact, maxCompactTokens)
	s.logger.Info("compaction: after trim",
		slog.Int("messages", len(toCompact)),
	)

	priorLogs, err := s.queries.ListCompactionLogsBySession(ctx, sessionUUID)
	if err != nil {
		return err
	}
	var priorSummaries []string
	for _, l := range priorLogs {
		if l.Summary != "" {
			priorSummaries = append(priorSummaries, l.Summary)
		}
	}

	entries := make([]messageEntry, 0, len(toCompact))
	messageIDs := make([]pgtype.UUID, 0, len(toCompact))
	for _, m := range toCompact {
		entries = append(entries, messageEntry{
			Role:    m.Role,
			Content: string(m.Content),
		})
		messageIDs = append(messageIDs, m.ID)
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

	result, err := sdk.GenerateTextResult(ctx,
		sdk.WithModel(model),
		sdk.WithSystem(systemPromptDecorated),
		sdk.WithMessages(sdkMessages),
	)
	if err != nil {
		return err
	}

	usageJSON, _ := json.Marshal(result.Usage)

	modelUUID := db.ParseUUIDOrEmpty(cfg.ModelID)

	if err := s.queries.MarkMessagesCompacted(ctx, withTeamID(sqlc.MarkMessagesCompactedParams{
		CompactID: logID,
		Column2:   messageIDs,
	}, teamID)); err != nil {
		return err
	}

	s.completeLog(ctx, logID, "ok", result.Text, "", len(messageIDs), usageJSON, modelUUID)
	return nil
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, summary, errMsg string, messageCount int, usage []byte, modelID pgtype.UUID) {
	teamID, teamErr := teamIDFromContext(ctx)
	if teamErr != nil {
		s.logger.Error("failed to resolve team scope for compaction log", slog.String("error", teamErr.Error()))
		return
	}
	if _, err := s.queries.CompleteCompactionLog(ctx, withTeamID(sqlc.CompleteCompactionLogParams{
		ID:           logID,
		Status:       status,
		Summary:      summary,
		MessageCount: int32(messageCount), //nolint:gosec // count always small
		ErrorMessage: errMsg,
		Usage:        usage,
		ModelID:      modelID,
	}, teamID)); err != nil {
		s.logger.Error("failed to complete compaction log", slog.String("error", err.Error()))
	}
}

// ListLogs returns paginated compaction logs for a bot.
func (s *Service) ListLogs(ctx context.Context, botID string, limit, offset int) ([]Log, int64, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, 0, err
	}
	teamID, err := teamIDFromContext(ctx)
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

	rows, err := s.queries.ListCompactionLogsByBot(ctx, withTeamID(sqlc.ListCompactionLogsByBotParams{
		BotID:       botUUID,
		LimitCount:  int32(limit),  //nolint:gosec // clamped above
		OffsetCount: int32(offset), //nolint:gosec // validated above
	}, teamID))
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

// splitByRatio splits messages so that roughly the first ratio% (by token weight)
// are returned for compaction, and the rest are kept as-is.
// When ratio >= 100, all messages are returned for compaction.
// When ratio <= 0 or totalInputTokens <= 0 or messages is empty, nil is returned (no compaction).
func splitByRatio(messages []sqlc.ListUncompactedMessagesBySessionRow, totalInputTokens, ratio int) []sqlc.ListUncompactedMessagesBySessionRow {
	if ratio <= 0 || totalInputTokens <= 0 || len(messages) == 0 {
		return nil
	}
	if ratio >= 100 {
		return messages
	}

	keepTokens := totalInputTokens * (100 - ratio) / 100
	if keepTokens <= 0 {
		return messages
	}

	accumulated := 0
	cutoff := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		accumulated += estimateRowTokens(messages[i])
		if accumulated >= keepTokens {
			cutoff = i + 1
			break
		}
	}

	if cutoff <= 0 {
		return nil
	}
	if cutoff >= len(messages) {
		return messages
	}
	return messages[:cutoff]
}

// splitByTarget returns the oldest messages to compact so that the remaining
// newest messages fit within targetTokens. This is used for synchronous
// compaction where the goal is to reduce context to a specific size.
func splitByTarget(messages []sqlc.ListUncompactedMessagesBySessionRow, targetTokens int) []sqlc.ListUncompactedMessagesBySessionRow {
	if targetTokens <= 0 || len(messages) == 0 {
		return nil
	}
	// Scan from newest to oldest, keeping messages that fit within target.
	accumulated := 0
	cutoff := 0
	for i := len(messages) - 1; i >= 0; i-- {
		accumulated += estimateRowTokens(messages[i])
		if accumulated > targetTokens {
			cutoff = i + 1
			break
		}
	}
	if cutoff <= 0 {
		return nil
	}
	return messages[:cutoff]
}

type usagePayload struct {
	OutputTokens *int `json:"output_tokens"`
}

func estimateRowTokens(m sqlc.ListUncompactedMessagesBySessionRow) int {
	if len(m.Usage) > 0 {
		var u usagePayload
		if json.Unmarshal(m.Usage, &u) == nil && u.OutputTokens != nil && *u.OutputTokens > 0 {
			return *u.OutputTokens
		}
	}
	return len(m.Content) / 4
}

// trimCompactMessages trims the compaction input from the tail (oldest)
// so the total estimated tokens stay within maxTokens.
func trimCompactMessages(messages []sqlc.ListUncompactedMessagesBySessionRow, maxTokens int) []sqlc.ListUncompactedMessagesBySessionRow {
	if len(messages) == 0 || maxTokens <= 0 {
		return messages
	}
	total := 0
	for _, m := range messages {
		total += estimateRowTokens(m)
	}
	if total <= maxTokens {
		return messages
	}
	// Drop oldest messages from the tail until within budget.
	accumulated := 0
	cutoff := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		accumulated += estimateRowTokens(messages[i])
		if accumulated > maxTokens {
			cutoff = i + 1
			break
		}
	}
	if cutoff >= len(messages) {
		return messages
	}
	return messages[cutoff:]
}
