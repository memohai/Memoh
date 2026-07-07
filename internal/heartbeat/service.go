package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/settings"
)

const heartbeatTokenTTL = 10 * time.Minute

// heartbeatRunTimeout caps how long a single heartbeat execution may take.
// This prevents unbounded Generate() calls from hanging forever.
const heartbeatRunTimeout = 5 * time.Minute

const defaultHeartbeatIntervalMinutes = 1440

// SessionCreator creates sessions for heartbeat runs.
type SessionCreator interface {
	CreateSession(ctx context.Context, botID, sessionType string) (string, error)
}

type Service struct {
	queries        dbstore.Queries
	cron           *cron.Cron
	triggerer      Triggerer
	sessionCreator SessionCreator
	memoryRegistry *memprovider.Registry
	settingsSvc    *settings.Service
	jwtSecret      string
	logger         *slog.Logger
	mu             sync.Mutex
	jobs           map[string]cron.EntryID
}

// SetMemoryRegistry wires the memory provider registry so heartbeats can
// converge agent-authored memory files into the DB. Set via setter (not the
// constructor) to mirror the MemoryHandler/Resolver convention and keep
// NewService's signature stable.
func (s *Service) SetMemoryRegistry(r *memprovider.Registry) { s.memoryRegistry = r }

// SetSettingsService wires the bot settings service used to resolve a bot's
// configured memory provider.
func (s *Service) SetSettingsService(svc *settings.Service) { s.settingsSvc = svc }

func NewService(log *slog.Logger, queries dbstore.Queries, triggerer Triggerer, sessionCreator SessionCreator, runtimeConfig *boot.RuntimeConfig) *Service {
	c := cron.New()
	service := &Service{
		queries:        queries,
		cron:           c,
		triggerer:      triggerer,
		sessionCreator: sessionCreator,
		jwtSecret:      runtimeConfig.JwtSecret,
		logger:         log.With(slog.String("service", "heartbeat")),
		jobs:           map[string]cron.EntryID{},
	}
	c.Start()
	return service
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if s.queries == nil {
		return errors.New("heartbeat queries not configured")
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
		if err := s.scheduleJob(ctx, cfg); err != nil {
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
	return s.scheduleJob(ctx, cfg)
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

	var sessionID string
	var pgSessionID pgtype.UUID
	if s.sessionCreator != nil {
		sid, err := s.sessionCreator.CreateSession(ctx, cfg.BotID, "heartbeat")
		if err != nil {
			s.logger.Error("create heartbeat session failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		} else {
			sessionID = sid
			pgSessionID = db.ParseUUIDOrEmpty(sid)
		}
	}

	var lastHeartbeatAt string
	if prevLogs, listErr := s.queries.ListHeartbeatLogsByBot(ctx, sqlc.ListHeartbeatLogsByBotParams{
		BotID: pgBotID,
		Limit: 1,
	}); listErr == nil && len(prevLogs) > 0 {
		lastHeartbeatAt = prevLogs[0].StartedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}

	logRow, err := s.queries.CreateHeartbeatLog(ctx, sqlc.CreateHeartbeatLogParams{
		BotID:     pgBotID,
		SessionID: pgSessionID,
	})
	if err != nil {
		s.logger.Error("create heartbeat log failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	token, err := s.generateTriggerToken(cfg.OwnerUserID)
	if err != nil {
		s.completeLog(ctx, logRow.ID, "error", "", err.Error(), nil, pgtype.UUID{})
		s.logger.Error("generate trigger token failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	result, err := s.triggerer.TriggerHeartbeat(ctx, cfg.BotID, TriggerPayload{
		BotID:           cfg.BotID,
		Interval:        cfg.Interval,
		OwnerUserID:     cfg.OwnerUserID,
		SessionID:       sessionID,
		LastHeartbeatAt: lastHeartbeatAt,
	}, token)
	if err != nil {
		s.completeLog(ctx, logRow.ID, "error", "", err.Error(), nil, pgtype.UUID{})
		s.logger.Error("heartbeat trigger failed", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	modelID := db.ParseUUIDOrEmpty(result.ModelID)
	s.completeLog(ctx, logRow.ID, result.Status, result.Text, "", result.UsageBytes, modelID)
	s.logger.Info("heartbeat completed", slog.String("bot_id", cfg.BotID), slog.String("status", result.Status))

	// Best-effort: converge agent-authored memory files into the DB so they
	// become visible to search_memory without a manual /ingest. Runs after the
	// trigger so files written during this tick are caught. Idempotent (ON
	// CONFLICT upsert); failures are Warn-logged and never affect the tick.
	s.ingestMemoryFiles(ctx, cfg.BotID)
}

// defaultBuiltinProviderID is the registry key under which the built-in graph
// memory provider is registered (see app.go provideMemoryProviderRegistry). It
// mirrors handlers.MemoryHandler.defaultBuiltinProviderID.
const defaultBuiltinProviderID = "__builtin_default__"

// ingestMemoryFiles resolves the bot's memory provider and, if it supports
// markdown ingest, imports agent-authored /data/memory/*.md into the DB. This
// mirrors MemoryHandler.resolveProvider + ChatIngest: it honours a bot's
// configured provider and falls back to the default builtin, then type-asserts
// to MarkdownIngestProvider. Non-supporting providers are a no-op (Debug log),
// not an error — only the builtin graph runtime implements ingest today.
func (s *Service) ingestMemoryFiles(ctx context.Context, botID string) {
	botID = strings.TrimSpace(botID)
	if botID == "" || s.memoryRegistry == nil {
		return
	}
	provider, err := s.resolveMemoryProvider(ctx, botID)
	if err != nil || provider == nil {
		s.logger.Debug("memory ingest skipped: provider unavailable", slog.String("bot_id", botID), slog.Any("error", err))
		return
	}
	ingest, ok := provider.(memprovider.MarkdownIngestProvider)
	if !ok {
		s.logger.Debug("memory ingest skipped: provider does not support markdown ingest", slog.String("bot_id", botID))
		return
	}
	result, err := ingest.IngestFromMarkdown(ctx, botID)
	if err != nil {
		s.logger.Warn("memory ingest failed", slog.String("bot_id", botID), slog.Any("error", err))
		return
	}
	if result.Ingested > 0 || result.Skipped > 0 {
		s.logger.Info("memory ingest completed",
			slog.String("bot_id", botID),
			slog.Int("ingested", result.Ingested),
			slog.Int("skipped", result.Skipped),
		)
	}
}

// resolveMemoryProvider resolves a bot's configured memory provider, falling
// back to the default builtin when none is configured. Mirrors
// handlers.MemoryHandler.resolveProvider so the heartbeat uses the same
// resolution semantics as the HTTP /ingest endpoint.
func (s *Service) resolveMemoryProvider(ctx context.Context, botID string) (memprovider.Provider, error) {
	if s.settingsSvc != nil {
		botSettings, err := s.settingsSvc.GetBot(ctx, botID)
		if err == nil {
			providerID := strings.TrimSpace(botSettings.MemoryProviderID)
			if providerID != "" {
				p, getErr := s.memoryRegistry.Get(providerID)
				if getErr == nil {
					return p, nil
				}
				s.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", getErr))
				return nil, fmt.Errorf("configured memory provider is unavailable: %w", getErr)
			}
		}
	}
	p, err := s.memoryRegistry.Get(defaultBuiltinProviderID)
	if err != nil {
		return nil, nil
	}
	return p, nil
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, resultText, errorMessage string, usageBytes []byte, modelID pgtype.UUID) {
	_, err := s.queries.CompleteHeartbeatLog(ctx, sqlc.CompleteHeartbeatLogParams{
		ID:           logID,
		Status:       status,
		ResultText:   resultText,
		ErrorMessage: errorMessage,
		Usage:        usageBytes,
		ModelID:      modelID,
	})
	if err != nil {
		s.logger.Error("complete heartbeat log failed", slog.Any("error", err))
	}
}

func (s *Service) ListLogs(ctx context.Context, botID string, limit, offset int) ([]Log, int64, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := s.queries.CountHeartbeatLogsByBot(ctx, pgBotID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.queries.ListHeartbeatLogsByBot(ctx, sqlc.ListHeartbeatLogsByBotParams{
		BotID:  pgBotID,
		Limit:  int32(limit),  //nolint:gosec // capped to 100 above
		Offset: int32(offset), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]Log, 0, len(rows))
	for _, row := range rows {
		items = append(items, toLog(row))
	}
	return items, total, nil
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
		return "", errors.New("jwt secret not configured")
	}
	signed, _, err := auth.GenerateToken(userID, s.jwtSecret, heartbeatTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}

func (s *Service) scheduleJob(ctx context.Context, cfg Config) error {
	cfg.Interval = normalizeHeartbeatInterval(cfg.Interval)
	spec := fmt.Sprintf("@every %dm", cfg.Interval)
	job := func() {
		runCtx, runCancel := context.WithTimeout(context.WithoutCancel(ctx), heartbeatRunTimeout)
		defer runCancel()
		s.runHeartbeat(runCtx, cfg)
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

func normalizeHeartbeatInterval(interval int) int {
	if interval <= 0 {
		return defaultHeartbeatIntervalMinutes
	}
	return interval
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

func toLog(row sqlc.ListHeartbeatLogsByBotRow) Log {
	l := Log{
		ID:           row.ID.String(),
		BotID:        row.BotID.String(),
		SessionID:    row.SessionID.String(),
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
