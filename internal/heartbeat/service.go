package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

const heartbeatTokenTTL = 10 * time.Minute

type Service struct {
	queries   *sqlc.Queries
	cron      *cron.Cron
	triggerer Triggerer
	jwtSecret string
	logger    *slog.Logger
	mu        sync.Mutex
	jobs      map[string]cron.EntryID
}

func NewService(log *slog.Logger, queries *sqlc.Queries, triggerer Triggerer, runtimeConfig *boot.RuntimeConfig) *Service {
	c := cron.New()
	service := &Service{
		queries:   queries,
		cron:      c,
		triggerer: triggerer,
		jwtSecret: runtimeConfig.JwtSecret,
		logger:    log.With(slog.String("service", "heartbeat")),
		jobs:      map[string]cron.EntryID{},
	}
	c.Start()
	return service
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if s.queries == nil {
		return fmt.Errorf("heartbeat queries not configured")
	}
	rows, err := s.queries.ListHeartbeatEnabledBots(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		botID := row.ID.String()
		ownerUserID := row.OwnerUserID.String()
		cfg := Config{
			BotID:       botID,
			OwnerUserID: ownerUserID,
			Interval:    int(row.HeartbeatInterval),
		}
		if err := s.scheduleJob(cfg); err != nil {
			s.logger.Error("failed to schedule heartbeat", slog.String("bot_id", botID), slog.Any("error", err))
		}
	}
	s.logger.Info("heartbeat bootstrap complete", slog.Int("count", len(rows)))
	return nil
}

func (s *Service) Reschedule(ctx context.Context, botID string) error {
	s.removeJob(botID)

	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	bot, err := s.queries.GetBotByID(ctx, pgID)
	if err != nil {
		return fmt.Errorf("get bot: %w", err)
	}
	if !bot.HeartbeatEnabled || bot.Status != "ready" {
		return nil
	}
	cfg := Config{
		BotID:       botID,
		OwnerUserID: bot.OwnerUserID.String(),
		Interval:    int(bot.HeartbeatInterval),
	}
	return s.scheduleJob(cfg)
}

func (s *Service) Stop(botID string) {
	s.removeJob(botID)
}

func (s *Service) runHeartbeat(ctx context.Context, cfg Config) {
	if s.triggerer == nil {
		s.logger.Error("heartbeat triggerer not configured")
		return
	}

	pgBotID, err := db.ParseUUID(cfg.BotID)
	if err != nil {
		s.logger.Error("invalid bot id", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	logRow, err := s.queries.CreateHeartbeatLog(ctx, pgBotID)
	if err != nil {
		s.logger.Error("create heartbeat log failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	token, err := s.generateTriggerToken(cfg.OwnerUserID)
	if err != nil {
		s.completeLog(ctx, logRow.ID, "error", "", err.Error(), nil)
		s.logger.Error("generate trigger token failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	result, err := s.triggerer.TriggerHeartbeat(ctx, cfg.BotID, TriggerPayload{
		BotID:       cfg.BotID,
		Interval:    cfg.Interval,
		OwnerUserID: cfg.OwnerUserID,
	}, token)
	if err != nil {
		s.completeLog(ctx, logRow.ID, "error", "", err.Error(), nil)
		s.logger.Error("heartbeat trigger failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	s.completeLog(ctx, logRow.ID, result.Status, result.Text, "", result.UsageBytes)
	s.logger.Info("heartbeat completed", slog.String("bot_id", cfg.BotID), slog.String("status", result.Status))
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, resultText, errorMessage string, usageBytes []byte) {
	_, err := s.queries.CompleteHeartbeatLog(ctx, sqlc.CompleteHeartbeatLogParams{
		ID:           logID,
		Status:       status,
		ResultText:   resultText,
		ErrorMessage: errorMessage,
		Usage:        usageBytes,
	})
	if err != nil {
		s.logger.Error("complete heartbeat log failed", slog.Any("error", err))
	}
}

func (s *Service) ListLogs(ctx context.Context, botID string, before *time.Time, limit int) ([]Log, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	beforeTS := pgtype.Timestamptz{}
	if before != nil {
		beforeTS = pgtype.Timestamptz{Time: *before, Valid: true}
	}
	rows, err := s.queries.ListHeartbeatLogsByBot(ctx, sqlc.ListHeartbeatLogsByBotParams{
		BotID:   pgBotID,
		Column2: beforeTS,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Log, 0, len(rows))
	for _, row := range rows {
		items = append(items, toLog(row))
	}
	return items, nil
}

func (s *Service) DeleteLogs(ctx context.Context, botID string) error {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteHeartbeatLogsByBot(ctx, pgBotID)
}

func (s *Service) generateTriggerToken(userID string) (string, error) {
	if strings.TrimSpace(s.jwtSecret) == "" {
		return "", fmt.Errorf("jwt secret not configured")
	}
	signed, _, err := auth.GenerateToken(userID, s.jwtSecret, heartbeatTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}

func (s *Service) scheduleJob(cfg Config) error {
	if cfg.Interval <= 0 {
		cfg.Interval = 30
	}
	spec := fmt.Sprintf("@every %dm", cfg.Interval)
	job := func() {
		s.runHeartbeat(context.Background(), cfg)
	}
	entryID, err := s.cron.AddFunc(spec, job)
	if err != nil {
		return fmt.Errorf("add heartbeat cron job: %w", err)
	}
	s.mu.Lock()
	s.jobs[cfg.BotID] = entryID
	s.mu.Unlock()
	s.logger.Info("heartbeat scheduled", slog.String("bot_id", cfg.BotID), slog.Int("interval_minutes", cfg.Interval))
	return nil
}

func (s *Service) removeJob(botID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entryID, ok := s.jobs[botID]
	if ok {
		s.cron.Remove(entryID)
		delete(s.jobs, botID)
	}
}

func toLog(row sqlc.BotHeartbeatLog) Log {
	l := Log{
		ID:           row.ID.String(),
		BotID:        row.BotID.String(),
		Status:       row.Status,
		ResultText:   row.ResultText,
		ErrorMessage: row.ErrorMessage,
	}
	if row.StartedAt.Valid {
		l.StartedAt = row.StartedAt.Time
	}
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		l.CompletedAt = &t
	}
	if row.Usage != nil {
		var usage any
		if err := json.Unmarshal(row.Usage, &usage); err == nil {
			l.Usage = usage
		}
	}
	return l
}
