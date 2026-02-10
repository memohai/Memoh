package schedule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries   *sqlc.Queries
	cron      *cron.Cron
	parser    cron.Parser
	triggerer Triggerer
	jwtSecret string
	logger    *slog.Logger
	mu        sync.Mutex
	jobs      map[string]cron.EntryID
}

func NewService(log *slog.Logger, queries *sqlc.Queries, triggerer Triggerer, runtimeConfig *boot.RuntimeConfig) *Service {
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	c := cron.New(cron.WithParser(parser))
	service := &Service{
		queries:   queries,
		cron:      c,
		parser:    parser,
		triggerer: triggerer,
		jwtSecret: runtimeConfig.JwtSecret,
		logger:    log.With(slog.String("service", "schedule")),
		jobs:      map[string]cron.EntryID{},
	}
	c.Start()
	return service
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if s.queries == nil {
		return fmt.Errorf("schedule queries not configured")
	}
	items, err := s.queries.ListEnabledSchedules(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.scheduleJob(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, botID string, req CreateRequest) (Schedule, error) {
	if s.queries == nil {
		return Schedule{}, fmt.Errorf("schedule queries not configured")
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" || strings.TrimSpace(req.Pattern) == "" || strings.TrimSpace(req.Command) == "" {
		return Schedule{}, fmt.Errorf("name, description, pattern, command are required")
	}
	if _, err := s.parser.Parse(req.Pattern); err != nil {
		return Schedule{}, fmt.Errorf("invalid cron pattern: %w", err)
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Schedule{}, err
	}
	maxCalls := pgtype.Int4{Valid: false}
	if req.MaxCalls.Set && req.MaxCalls.Value != nil {
		maxCalls = pgtype.Int4{Int32: int32(*req.MaxCalls.Value), Valid: true}
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
		if err := s.scheduleJob(row); err != nil {
			return Schedule{}, err
		}
	}
	return toSchedule(row), nil
}

func (s *Service) Get(ctx context.Context, id string) (Schedule, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return Schedule{}, err
	}
	row, err := s.queries.GetScheduleByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Schedule{}, fmt.Errorf("schedule not found")
		}
		return Schedule{}, err
	}
	return toSchedule(row), nil
}

func (s *Service) List(ctx context.Context, botID string) ([]Schedule, error) {
	pgBotID, err := parseUUID(botID)
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
	pgID, err := parseUUID(id)
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
			maxCalls = pgtype.Int4{Int32: int32(*req.MaxCalls.Value), Valid: true}
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
	s.rescheduleJob(updated)
	return toSchedule(updated), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := parseUUID(id)
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
		return fmt.Errorf("schedule triggerer not configured")
	}
	schedule, err := s.Get(ctx, scheduleID)
	if err != nil {
		return err
	}
	if !schedule.Enabled {
		return fmt.Errorf("schedule is disabled")
	}
	return s.runSchedule(ctx, schedule)
}

const scheduleTokenTTL = 10 * time.Minute

func (s *Service) runSchedule(ctx context.Context, schedule Schedule) error {
	if s.triggerer == nil {
		return fmt.Errorf("schedule triggerer not configured")
	}
	updated, err := s.queries.IncrementScheduleCalls(ctx, toUUID(schedule.ID))
	if err != nil {
		return err
	}
	if !updated.Enabled {
		s.removeJob(schedule.ID)
	}

	ownerUserID, err := s.resolveBotOwner(ctx, schedule.BotID)
	if err != nil {
		return fmt.Errorf("resolve bot owner: %w", err)
	}

	token, err := s.generateTriggerToken(ownerUserID)
	if err != nil {
		return fmt.Errorf("generate trigger token: %w", err)
	}

	return s.triggerer.TriggerSchedule(ctx, schedule.BotID, TriggerPayload{
		ID:          schedule.ID,
		Name:        schedule.Name,
		Description: schedule.Description,
		Pattern:     schedule.Pattern,
		MaxCalls:    schedule.MaxCalls,
		Command:     schedule.Command,
		OwnerUserID: ownerUserID,
	}, token)
}

// resolveBotOwner returns the owner user ID for the given bot.
func (s *Service) resolveBotOwner(ctx context.Context, botID string) (string, error) {
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return "", err
	}
	bot, err := s.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return "", fmt.Errorf("get bot: %w", err)
	}
	ownerID := toUUIDString(bot.OwnerUserID)
	if ownerID == "" {
		return "", fmt.Errorf("bot owner not found")
	}
	return ownerID, nil
}

// generateTriggerToken creates a short-lived JWT for schedule trigger callbacks.
func (s *Service) generateTriggerToken(userID string) (string, error) {
	if strings.TrimSpace(s.jwtSecret) == "" {
		return "", fmt.Errorf("jwt secret not configured")
	}
	signed, _, err := auth.GenerateToken(userID, s.jwtSecret, scheduleTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}

func (s *Service) scheduleJob(schedule sqlc.Schedule) error {
	id := toUUIDString(schedule.ID)
	if id == "" {
		return fmt.Errorf("schedule id missing")
	}
	job := func() {
		_ = s.runSchedule(context.Background(), toSchedule(schedule))
	}
	entryID, err := s.cron.AddFunc(schedule.Pattern, job)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.jobs[id] = entryID
	s.mu.Unlock()
	return nil
}

func (s *Service) rescheduleJob(schedule sqlc.Schedule) {
	id := toUUIDString(schedule.ID)
	if id == "" {
		return
	}
	s.removeJob(id)
	if schedule.Enabled {
		_ = s.scheduleJob(schedule)
	}
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
		ID:           toUUIDString(row.ID),
		Name:         row.Name,
		Description:  row.Description,
		Pattern:      row.Pattern,
		CurrentCalls: int(row.CurrentCalls),
		Enabled:      row.Enabled,
		Command:      row.Command,
		BotID:        toUUIDString(row.BotID),
	}
	if row.MaxCalls.Valid {
		max := int(row.MaxCalls.Int32)
		item.MaxCalls = &max
	}
	if row.CreatedAt.Valid {
		item.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		item.UpdatedAt = row.UpdatedAt.Time
	}
	return item
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

func toUUID(id string) pgtype.UUID {
	pgID, err := parseUUID(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgID
}

func toUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}
