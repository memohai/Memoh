package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// SessionCreator creates sessions for schedule runs.
type SessionCreator interface {
	CreateSession(ctx context.Context, botID, sessionType string) (string, error)
}

type Service struct {
	queries         *sqlc.Queries
	cron            *cron.Cron
	parser          cron.Parser
	triggerer       Triggerer
	sessionCreator  SessionCreator
	jwtSecret       string
	logger          *slog.Logger
	defaultLocation *time.Location
	mu              sync.Mutex
	jobs            map[string]cron.EntryID
}

func NewService(log *slog.Logger, queries *sqlc.Queries, triggerer Triggerer, sessionCreator SessionCreator, runtimeConfig *boot.RuntimeConfig) *Service {
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	location := time.UTC
	if runtimeConfig != nil && runtimeConfig.TimezoneLocation != nil {
		location = runtimeConfig.TimezoneLocation
	}
	c := cron.New(cron.WithParser(parser), cron.WithLocation(location))
	service := &Service{
		queries:         queries,
		cron:            c,
		parser:          parser,
		triggerer:       triggerer,
		sessionCreator:  sessionCreator,
		jwtSecret:       runtimeConfig.JwtSecret,
		logger:          log.With(slog.String("service", "schedule")),
		defaultLocation: location,
		jobs:            map[string]cron.EntryID{},
	}
	c.Start()
	return service
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if s.queries == nil {
		return errors.New("schedule queries not configured")
	}
	items, err := s.queries.ListEnabledSchedules(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.scheduleJob(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, botID string, req CreateRequest) (Schedule, error) {
	if s.queries == nil {
		return Schedule{}, errors.New("schedule queries not configured")
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" || strings.TrimSpace(req.Pattern) == "" || strings.TrimSpace(req.Command) == "" {
		return Schedule{}, errors.New("name, description, pattern, command are required")
	}
	if _, err := s.parser.Parse(req.Pattern); err != nil {
		return Schedule{}, fmt.Errorf("invalid cron pattern: %w", err)
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Schedule{}, err
	}
	maxCalls := pgtype.Int4{Valid: false}
	if req.MaxCalls.Set && req.MaxCalls.Value != nil {
		if *req.MaxCalls.Value < math.MinInt32 || *req.MaxCalls.Value > math.MaxInt32 {
			return Schedule{}, fmt.Errorf("max_calls out of range: %d", *req.MaxCalls.Value)
		}
		maxCalls = pgtype.Int4{Int32: int32(*req.MaxCalls.Value), Valid: true} //nolint:gosec // bounds checked above
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := s.queries.CreateSchedule(ctx, sqlc.CreateScheduleParams{
		Name:        req.Name,
		Description: req.Description,
		Pattern:     req.Pattern,
		MaxCalls:    maxCalls,
		Enabled:     enabled,
		Command:     req.Command,
		BotID:       pgBotID,
	})
	if err != nil {
		return Schedule{}, err
	}
	if row.Enabled {
		if err := s.scheduleJob(ctx, row); err != nil {
			return Schedule{}, err
		}
	}
	return toSchedule(row), nil
}

func (s *Service) Get(ctx context.Context, id string) (Schedule, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Schedule{}, err
	}
	row, err := s.queries.GetScheduleByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Schedule{}, errors.New("schedule not found")
		}
		return Schedule{}, err
	}
	return toSchedule(row), nil
}

func (s *Service) List(ctx context.Context, botID string) ([]Schedule, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSchedulesByBot(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Schedule, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSchedule(row))
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Schedule, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Schedule{}, err
	}
	existing, err := s.queries.GetScheduleByID(ctx, pgID)
	if err != nil {
		return Schedule{}, err
	}
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	description := existing.Description
	if req.Description != nil {
		description = *req.Description
	}
	pattern := existing.Pattern
	if req.Pattern != nil {
		if _, err := s.parser.Parse(*req.Pattern); err != nil {
			return Schedule{}, fmt.Errorf("invalid cron pattern: %w", err)
		}
		pattern = *req.Pattern
	}
	command := existing.Command
	if req.Command != nil {
		command = *req.Command
	}
	maxCalls := existing.MaxCalls
	if req.MaxCalls.Set {
		if req.MaxCalls.Value == nil {
			maxCalls = pgtype.Int4{Valid: false}
		} else {
			if *req.MaxCalls.Value < math.MinInt32 || *req.MaxCalls.Value > math.MaxInt32 {
				return Schedule{}, fmt.Errorf("max_calls out of range: %d", *req.MaxCalls.Value)
			}
			maxCalls = pgtype.Int4{Int32: int32(*req.MaxCalls.Value), Valid: true} //nolint:gosec // bounds checked above
		}
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := s.queries.UpdateSchedule(ctx, sqlc.UpdateScheduleParams{
		ID:          pgID,
		Name:        name,
		Description: description,
		Pattern:     pattern,
		MaxCalls:    maxCalls,
		Enabled:     enabled,
		Command:     command,
	})
	if err != nil {
		return Schedule{}, err
	}
	if err := s.rescheduleJob(ctx, updated); err != nil {
		return Schedule{}, fmt.Errorf("reschedule job: %w", err)
	}
	return toSchedule(updated), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	if err := s.queries.DeleteSchedule(ctx, pgID); err != nil {
		return err
	}
	s.removeJob(id)
	return nil
}

func (s *Service) Trigger(ctx context.Context, scheduleID string) error {
	if s.triggerer == nil {
		return errors.New("schedule triggerer not configured")
	}
	sched, err := s.Get(ctx, scheduleID)
	if err != nil {
		return err
	}
	if !sched.Enabled {
		return errors.New("schedule is disabled")
	}
	return s.runSchedule(ctx, sched)
}

const scheduleTokenTTL = 10 * time.Minute

// scheduleRunTimeout caps how long a single schedule execution may take.
// This prevents unbounded Generate() calls from hanging forever.
const scheduleRunTimeout = 5 * time.Minute

func (s *Service) runSchedule(ctx context.Context, sched Schedule) error {
	if s.triggerer == nil {
		return errors.New("schedule triggerer not configured")
	}
	updated, err := s.queries.IncrementScheduleCalls(ctx, toUUID(sched.ID))
	if err != nil {
		return err
	}
	if !updated.Enabled {
		s.removeJob(sched.ID)
	}

	ownerUserID, err := s.resolveBotOwner(ctx, sched.BotID)
	if err != nil {
		return fmt.Errorf("resolve bot owner: %w", err)
	}

	var sessionID string
	var pgSessionID pgtype.UUID
	if s.sessionCreator != nil {
		sid, err := s.sessionCreator.CreateSession(ctx, sched.BotID, "schedule")
		if err != nil {
			s.logger.Error("create schedule session failed", slog.String("bot_id", sched.BotID), slog.Any("error", err))
		} else {
			sessionID = sid
			pgSessionID = db.ParseUUIDOrEmpty(sid)
		}
	}

	pgScheduleID := toUUID(sched.ID)
	pgBotID := toUUID(sched.BotID)

	logRow, err := s.queries.CreateScheduleLog(ctx, sqlc.CreateScheduleLogParams{
		ScheduleID: pgScheduleID,
		BotID:      pgBotID,
		SessionID:  pgSessionID,
	})
	if err != nil {
		s.logger.Error("create schedule log failed", slog.String("schedule_id", sched.ID), slog.Any("error", err))
	}

	token, err := s.generateTriggerToken(ownerUserID)
	if err != nil {
		s.completeLog(ctx, logRow.ID, "error", "", err.Error(), nil, pgtype.UUID{})
		return fmt.Errorf("generate trigger token: %w", err)
	}

	result, triggerErr := s.triggerer.TriggerSchedule(ctx, sched.BotID, TriggerPayload{
		ID:          sched.ID,
		Name:        sched.Name,
		Description: sched.Description,
		Pattern:     sched.Pattern,
		MaxCalls:    sched.MaxCalls,
		Command:     sched.Command,
		OwnerUserID: ownerUserID,
		SessionID:   sessionID,
	}, token)
	if triggerErr != nil {
		s.completeLog(ctx, logRow.ID, "error", "", triggerErr.Error(), nil, pgtype.UUID{})
		return triggerErr
	}

	modelID := db.ParseUUIDOrEmpty(result.ModelID)
	s.completeLog(ctx, logRow.ID, result.Status, result.Text, "", result.UsageBytes, modelID)
	s.logger.Info("schedule completed", slog.String("schedule_id", sched.ID), slog.String("status", result.Status))
	return nil
}

func (s *Service) completeLog(ctx context.Context, logID pgtype.UUID, status, resultText, errorMessage string, usageBytes []byte, modelID pgtype.UUID) {
	if !logID.Valid {
		return
	}
	_, err := s.queries.CompleteScheduleLog(ctx, sqlc.CompleteScheduleLogParams{
		ID:           logID,
		Status:       status,
		ResultText:   resultText,
		ErrorMessage: errorMessage,
		Usage:        usageBytes,
		ModelID:      modelID,
	})
	if err != nil {
		s.logger.Error("complete schedule log failed", slog.Any("error", err))
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

	total, err := s.queries.CountScheduleLogsByBot(ctx, pgBotID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.queries.ListScheduleLogsByBot(ctx, sqlc.ListScheduleLogsByBotParams{
		BotID:  pgBotID,
		Limit:  int32(limit),  //nolint:gosec // capped to 100 above
		Offset: int32(offset), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]Log, 0, len(rows))
	for _, row := range rows {
		items = append(items, toScheduleLog(row))
	}
	return items, total, nil
}

func (s *Service) ListLogsBySchedule(ctx context.Context, scheduleID string, limit, offset int) ([]Log, int64, error) {
	pgID, err := db.ParseUUID(scheduleID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	total, err := s.queries.CountScheduleLogsBySchedule(ctx, pgID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.queries.ListScheduleLogsBySchedule(ctx, sqlc.ListScheduleLogsByScheduleParams{
		ScheduleID: pgID,
		Limit:      int32(limit),  //nolint:gosec // capped to 100 above
		Offset:     int32(offset), //nolint:gosec // validated above
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]Log, 0, len(rows))
	for _, row := range rows {
		items = append(items, toScheduleLogFromSchedule(row))
	}
	return items, total, nil
}

func (s *Service) DeleteLogs(ctx context.Context, botID string) error {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteScheduleLogsByBot(ctx, pgBotID)
}

func toScheduleLog(row sqlc.ListScheduleLogsByBotRow) Log {
	l := Log{
		ID:           row.ID.String(),
		ScheduleID:   row.ScheduleID.String(),
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

func toScheduleLogFromSchedule(row sqlc.ListScheduleLogsByScheduleRow) Log {
	l := Log{
		ID:           row.ID.String(),
		ScheduleID:   row.ScheduleID.String(),
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

// resolveBotOwner returns the owner user ID for the given bot.
func (s *Service) resolveBotOwner(ctx context.Context, botID string) (string, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return "", err
	}
	bot, err := s.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return "", fmt.Errorf("get bot: %w", err)
	}
	ownerID := bot.OwnerUserID.String()
	if ownerID == "" {
		return "", errors.New("bot owner not found")
	}
	return ownerID, nil
}

// generateTriggerToken creates a short-lived JWT for schedule trigger callbacks.
func (s *Service) generateTriggerToken(userID string) (string, error) {
	if strings.TrimSpace(s.jwtSecret) == "" {
		return "", errors.New("jwt secret not configured")
	}
	signed, _, err := auth.GenerateServiceToken(userID, s.jwtSecret, scheduleTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}

func (s *Service) scheduleJob(ctx context.Context, schedule sqlc.Schedule) error {
	id := schedule.ID.String()
	if id == "" {
		return errors.New("schedule id missing")
	}
	job := func() {
		runCtx, runCancel := context.WithTimeout(context.WithoutCancel(ctx), scheduleRunTimeout)
		defer runCancel()
		if err := s.runSchedule(runCtx, toSchedule(schedule)); err != nil {
			s.logger.Error("scheduled job failed", slog.String("schedule_id", schedule.ID.String()), slog.Any("error", err))
		}
	}

	// Resolve bot timezone so cron expressions are interpreted in the bot's
	// configured timezone rather than the system default.
	loc := s.resolveBotLocation(ctx, schedule.BotID)
	sched, err := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor).Parse(schedule.Pattern)
	if err != nil {
		return err
	}
	entryID := s.cron.Schedule(newLocationSchedule(sched, loc), cron.FuncJob(job))
	s.mu.Lock()
	s.jobs[id] = entryID
	s.mu.Unlock()
	return nil
}

func (s *Service) rescheduleJob(ctx context.Context, schedule sqlc.Schedule) error {
	id := schedule.ID.String()
	if id == "" {
		return nil
	}
	s.removeJob(id)
	if schedule.Enabled {
		return s.scheduleJob(ctx, schedule)
	}
	return nil
}

func (s *Service) removeJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entryID, ok := s.jobs[id]
	if ok {
		s.cron.Remove(entryID)
		delete(s.jobs, id)
	}
}

func toSchedule(row sqlc.Schedule) Schedule {
	item := Schedule{
		ID:           row.ID.String(),
		Name:         row.Name,
		Description:  row.Description,
		Pattern:      row.Pattern,
		CurrentCalls: int(row.CurrentCalls),
		Enabled:      row.Enabled,
		Command:      row.Command,
		BotID:        row.BotID.String(),
	}
	if row.MaxCalls.Valid {
		maxCalls := int(row.MaxCalls.Int32)
		item.MaxCalls = &maxCalls
	}
	if row.CreatedAt.Valid {
		item.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		item.UpdatedAt = row.UpdatedAt.Time
	}
	return item
}

func toUUID(id string) pgtype.UUID {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgID
}

// resolveBotLocation returns the bot's configured timezone location, falling
// back to the system default when the bot has no timezone set or the value is
// invalid.
func (s *Service) resolveBotLocation(ctx context.Context, botID pgtype.UUID) *time.Location {
	if s.queries == nil || !botID.Valid {
		return s.defaultLocation
	}
	row, err := s.queries.GetBotByID(ctx, botID)
	if err != nil {
		return s.defaultLocation
	}
	if !row.Timezone.Valid {
		return s.defaultLocation
	}
	tz := strings.TrimSpace(row.Timezone.String)
	if tz == "" {
		return s.defaultLocation
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		s.logger.Warn("invalid bot timezone for schedule, using default",
			slog.String("bot_id", botID.String()),
			slog.String("timezone", tz),
			slog.Any("error", err),
		)
		return s.defaultLocation
	}
	return loc
}

// locationSchedule wraps a cron.Schedule to evaluate Next() in a specific
// timezone, regardless of the global cron location.
type locationSchedule struct {
	inner cron.Schedule
	loc   *time.Location
}

func newLocationSchedule(inner cron.Schedule, loc *time.Location) cron.Schedule {
	if loc == nil {
		return inner
	}
	return &locationSchedule{inner: inner, loc: loc}
}

func (s *locationSchedule) Next(t time.Time) time.Time {
	return s.inner.Next(t.In(s.loc))
}
