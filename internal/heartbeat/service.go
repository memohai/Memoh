package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
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
	"github.com/memohai/memoh/internal/teams"
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

// TeamSessionCreator creates sessions with an explicit tenant scope.
type TeamSessionCreator interface {
	CreateSessionForTeam(ctx context.Context, teamID, botID, sessionType string) (string, error)
}

type Service struct {
	queries        dbstore.Queries
	cron           *cron.Cron
	triggerer      Triggerer
	sessionCreator SessionCreator
	jwtSecret      string
	logger         *slog.Logger
	mu             sync.Mutex
	jobs           map[string]cron.EntryID
}

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
		teamID := teamIDFromValue(row, teams.ScopeOrDefault(ctx).TeamID)
		cfg := Config{
			TeamID:      teamID,
			BotID:       botID,
			OwnerUserID: ownerUserID,
			Interval:    int(row.HeartbeatInterval),
		}
		if err := s.scheduleJob(withTeamScope(ctx, teamID), cfg); err != nil {
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
	teamID := teams.ScopeOrDefault(ctx).TeamID
	cfg := Config{
		TeamID:      teamID,
		BotID:       botID,
		OwnerUserID: bot.OwnerUserID.String(),
		Interval:    int(bot.HeartbeatInterval),
	}
	return s.scheduleJob(withTeamScope(ctx, teamID), cfg)
}

func (s *Service) Stop(botID string) {
	s.removeJob(botID)
}

func (s *Service) runHeartbeat(ctx context.Context, cfg Config) {
	if s.triggerer == nil {
		s.logger.Error("heartbeat triggerer not configured")
		return
	}
	teamID := firstNonEmpty(cfg.TeamID, teams.ScopeOrDefault(ctx).TeamID)
	ctx = withTeamScope(ctx, teamID)

	pgBotID, err := db.ParseUUID(cfg.BotID)
	if err != nil {
		s.logger.Error("invalid bot id", slog.String("bot_id", cfg.BotID), slog.Any("error", err))
		return
	}

	sessionID, pgSessionID := s.createRunSession(ctx, teamID, cfg.BotID, "heartbeat")

	var lastHeartbeatAt string
	pgTeamID := db.ParseUUIDOrEmpty(teamID)
	if prevLogs, listErr := s.queries.ListHeartbeatLogsByBot(ctx, sqlc.ListHeartbeatLogsByBotParams{
		TeamID:     pgTeamID,
		BotID:      pgBotID,
		LimitCount: 1,
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
		TeamID:          teamID,
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
	pgTeamID, err := db.ParseUUID(teams.ScopeOrDefault(ctx).TeamID)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.queries.CountHeartbeatLogsByBot(ctx, pgBotID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.queries.ListHeartbeatLogsByBot(ctx, sqlc.ListHeartbeatLogsByBotParams{
		TeamID:      pgTeamID,
		BotID:       pgBotID,
		LimitCount:  int32(limit),  //nolint:gosec // capped to 100 above
		OffsetCount: int32(offset), //nolint:gosec // validated above
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
	cfg.TeamID = firstNonEmpty(cfg.TeamID, teams.ScopeOrDefault(ctx).TeamID)
	ctx = withTeamScope(ctx, cfg.TeamID)
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

func (s *Service) createRunSession(ctx context.Context, teamID, botID, sessionType string) (string, pgtype.UUID) {
	if s.sessionCreator == nil {
		return "", pgtype.UUID{}
	}
	var (
		sessionID string
		err       error
	)
	if strings.TrimSpace(teamID) != "" {
		if creator, ok := s.sessionCreator.(TeamSessionCreator); ok {
			sessionID, err = creator.CreateSessionForTeam(ctx, teamID, botID, sessionType)
		} else {
			sessionID, err = s.sessionCreator.CreateSession(ctx, botID, sessionType)
		}
	} else {
		sessionID, err = s.sessionCreator.CreateSession(ctx, botID, sessionType)
	}
	if err != nil {
		s.logger.Error("create heartbeat session failed",
			slog.String("team_id", teamID),
			slog.String("bot_id", botID),
			slog.String("session_type", sessionType),
			slog.Any("error", err),
		)
		return "", pgtype.UUID{}
	}
	return sessionID, db.ParseUUIDOrEmpty(sessionID)
}

func withTeamScope(ctx context.Context, teamID string) context.Context {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		teamID = teams.DefaultTeamID
	}
	return teams.WithScope(ctx, teams.Scope{TeamID: teamID})
}

func teamIDFromValue(value any, fallback string) string {
	if teamID, ok := value.(interface{ GetTeamID() string }); ok {
		if trimmed := strings.TrimSpace(teamID.GetTeamID()); trimmed != "" {
			return trimmed
		}
	}
	if stringer, ok := value.(interface{ TeamIDString() string }); ok {
		if trimmed := strings.TrimSpace(stringer.TeamIDString()); trimmed != "" {
			return trimmed
		}
	}
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			break
		}
		v = v.Elem()
	}
	if v.IsValid() && v.Kind() == reflect.Struct {
		field := v.FieldByName("TeamID")
		if field.IsValid() {
			if field.Kind() == reflect.String {
				if trimmed := strings.TrimSpace(field.String()); trimmed != "" {
					return trimmed
				}
			}
			if field.CanInterface() {
				if stringer, ok := field.Interface().(fmt.Stringer); ok {
					if trimmed := strings.TrimSpace(stringer.String()); trimmed != "" {
						return trimmed
					}
				}
			}
		}
	}
	teamID := strings.TrimSpace(fallback)
	if teamID == "" {
		teamID = teams.DefaultTeamID
	}
	return teamID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
		TeamID:       teamIDFromValue(row, ""),
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
