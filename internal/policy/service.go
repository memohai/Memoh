package policy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/settings"
)

type Decision struct {
	BotID      string
	BotType    string
	AllowGuest bool
}

type Service struct {
	bots     *bots.Service
	settings *settings.Service
	logger   *slog.Logger
}

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

func (s *Service) Resolve(ctx context.Context, botID string) (Decision, error) {
	if s == nil || s.bots == nil || s.settings == nil {
		return Decision{}, fmt.Errorf("policy service not configured")
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return Decision{}, fmt.Errorf("bot id is required")
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
