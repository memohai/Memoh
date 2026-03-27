package compaction

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
)

// Service manages context compaction for bot conversations.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a new compaction Service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log,
	}
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

func (s *Service) runCompaction(ctx context.Context, cfg TriggerConfig) error {
	botUUID, err := db.ParseUUID(cfg.BotID)
	if err != nil {
		return err
	}
	sessionUUID, err := db.ParseUUID(cfg.SessionID)
	if err != nil {
		return err
	}

	logRow, err := s.queries.CreateCompactionLog(ctx, sqlc.CreateCompactionLogParams{
		BotID:     botUUID,
		SessionID: sessionUUID,
	})
	if err != nil {
		return err
	}

	compactErr := s.doCompaction(ctx, logRow.ID, sessionUUID, cfg)
	if compactErr != nil {
		s.completeLog(ctx, logRow.ID, "error", "", compactErr.Error(), nil, pgtype.UUID{})
	}
	return compactErr
}

func (s *Service) doCompaction(ctx context.Context, logID pgtype.UUID, sessionUUID pgtype.UUID, cfg TriggerConfig) error {
	messages, err := s.queries.ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		s.completeLog(ctx, logID, "ok", "", "", nil, pgtype.UUID{})
		return nil
	}

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

	entries := make([]messageEntry, 0, len(messages))
	messageIDs := make([]pgtype.UUID, 0, len(messages))
	for _, m := range messages {
		entries = append(entries, messageEntry{
			Role:    m.Role,
			Content: extractTextContent(m.Content),
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

	result, err := sdk.GenerateTextResult(ctx,
		sdk.WithModel(model),
		sdk.WithSystem(systemPrompt),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage(userPrompt)}),
	)
	if err != nil {
		return err
	}

	usageJSON, _ := json.Marshal(result.Usage)

	modelUUID := db.ParseUUIDOrEmpty(cfg.ModelID)

	if err := s.queries.MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
		CompactID: logID,
		Column2:   messageIDs,
	}); err != nil {
		return err
	}

	s.completeLog(ctx, logID, "ok", result.Text, "", usageJSON, modelUUID)
	return nil
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, summary, errMsg string, usage []byte, modelID pgtype.UUID) {
	if _, err := s.queries.CompleteCompactionLog(ctx, sqlc.CompleteCompactionLogParams{
		ID:           logID,
		Status:       status,
		Summary:      summary,
		MessageCount: 0,
		ErrorMessage: errMsg,
		Usage:        usage,
		ModelID:      modelID,
	}); err != nil {
		s.logger.Error("failed to complete compaction log", slog.String("error", err.Error()))
	}
}

// ListLogs returns paginated compaction logs for a bot.
func (s *Service) ListLogs(ctx context.Context, botID string, before *time.Time, limit int) ([]Log, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}

	var beforeTS pgtype.Timestamptz
	if before != nil {
		beforeTS = pgtype.Timestamptz{Time: *before, Valid: true}
	}

	clampedLimit := limit
	if clampedLimit > 1000 {
		clampedLimit = 1000
	}
	rows, err := s.queries.ListCompactionLogsByBot(ctx, sqlc.ListCompactionLogsByBotParams{
		BotID:   botUUID,
		Column2: beforeTS,
		Limit:   int32(clampedLimit), //nolint:gosec // clamped above
	})
	if err != nil {
		return nil, err
	}

	logs := make([]Log, len(rows))
	for i, r := range rows {
		logs[i] = toLog(r)
	}
	return logs, nil
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

// extractTextContent extracts plain text from a message content JSONB field.
// The content may be a JSON string, an array of content parts, or raw bytes.
func extractTextContent(content []byte) string {
	if len(content) == 0 {
		return ""
	}

	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}

	var parts []map[string]any
	if json.Unmarshal(content, &parts) == nil {
		var texts []string
		for _, p := range parts {
			if t, ok := p["type"].(string); ok && t == "text" {
				if text, ok := p["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		if len(texts) > 0 {
			return joinTexts(texts)
		}
	}

	return string(content)
}

func joinTexts(parts []string) string {
	return strings.Join(parts, " ")
}
