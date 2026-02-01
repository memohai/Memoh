package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "settings")),
	}
}

func (s *Service) Get(ctx context.Context, userID string) (Settings, error) {
	pgID, err := parseUUID(userID)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.queries.GetSettingsByUserID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Settings{
				MaxContextLoadTime: DefaultMaxContextLoadTime,
				Language:           DefaultLanguage,
			}, nil
		}
		return Settings{}, err
	}
	return normalizeUserSetting(row), nil
}

func (s *Service) Upsert(ctx context.Context, userID string, req UpsertRequest) (Settings, error) {
	if s.queries == nil {
		return Settings{}, fmt.Errorf("settings queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return Settings{}, err
	}

	current := Settings{
		MaxContextLoadTime: DefaultMaxContextLoadTime,
		Language:           DefaultLanguage,
	}
	existing, err := s.queries.GetSettingsByUserID(ctx, pgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, err
	}
	if err == nil {
		current = normalizeUserSetting(existing)
	}

	if req.MaxContextLoadTime != nil && *req.MaxContextLoadTime > 0 {
		current.MaxContextLoadTime = *req.MaxContextLoadTime
	}
	if strings.TrimSpace(req.Language) != "" {
		current.Language = strings.TrimSpace(req.Language)
	}

	_, err = s.queries.UpsertSettings(ctx, sqlc.UpsertSettingsParams{
		UserID:             pgID,
		MaxContextLoadTime: int32(current.MaxContextLoadTime),
		Language:           current.Language,
	})
	if err != nil {
		return Settings{}, err
	}
	return current, nil
}

func (s *Service) Delete(ctx context.Context, userID string) error {
	if s.queries == nil {
		return fmt.Errorf("settings queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	return s.queries.DeleteSettingsByUserID(ctx, pgID)
}

func normalizeUserSetting(row sqlc.UserSetting) Settings {
	settings := Settings{
		MaxContextLoadTime: int(row.MaxContextLoadTime),
		Language:           strings.TrimSpace(row.Language),
	}
	if settings.MaxContextLoadTime <= 0 {
		settings.MaxContextLoadTime = DefaultMaxContextLoadTime
	}
	if settings.Language == "" {
		settings.Language = DefaultLanguage
	}
	return settings
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}
