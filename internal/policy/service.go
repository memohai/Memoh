// Package policy provides access policy evaluation for bots and channels.
package policy

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/settings"
)

// Decision is the resolved access policy for a bot (type and whether guest is allowed).
type Decision struct {
	BotID      string
	BotType    string
	AllowGuest bool
}

// Service evaluates bot access policy using bots and settings services.
type Service struct {
	bots     *bots.Service
	settings *settings.Service
	logger   *slog.Logger
}

// NewService creates a policy service.
func NewService(log *slog.Logger, botsService *bots.Service, settingsService *settings.Service) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		bots:     botsService,
		settings: settingsService,
		logger:   log.With(slog.String("service", "policy")),
	}
}

// Resolve evaluates the full access policy for a bot.
func (s *Service) Resolve(ctx context.Context, botID string) (Decision, error) {
	if s == nil || s.bots == nil || s.settings == nil {
		return Decision{}, errors.New("policy service not configured")
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return Decision{}, errors.New("bot id is required")
	}
	bot, err := s.bots.Get(ctx, botID)
	if err != nil {
		return Decision{}, err
	}
	botSettings, err := s.settings.GetBot(ctx, botID)
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{
		BotID:      botID,
		BotType:    strings.TrimSpace(bot.Type),
		AllowGuest: botSettings.AllowGuest,
	}
	if decision.BotType == bots.BotTypePersonal {
		decision.AllowGuest = false
	}
	return decision, nil
}

// AllowGuest checks if the bot allows guest access. Implements router.PolicyService.
func (s *Service) AllowGuest(ctx context.Context, botID string) (bool, error) {
	decision, err := s.Resolve(ctx, botID)
	if err != nil {
		return false, err
	}
	return decision.AllowGuest, nil
}

// BotType returns the normalized bot type. Implements router.PolicyService.
func (s *Service) BotType(ctx context.Context, botID string) (string, error) {
	decision, err := s.Resolve(ctx, botID)
	if err != nil {
		return "", err
	}
	return decision.BotType, nil
}

// BotOwnerUserID returns bot owner's user id. Implements router.PolicyService.
func (s *Service) BotOwnerUserID(ctx context.Context, botID string) (string, error) {
	if s == nil || s.bots == nil {
		return "", errors.New("policy service not configured")
	}
	bot, err := s.bots.Get(ctx, strings.TrimSpace(botID))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(bot.OwnerUserID), nil
}
