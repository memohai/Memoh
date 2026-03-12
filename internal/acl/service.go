package acl

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

var ErrInvalidRuleSubject = errors.New("exactly one of user_id or channel_identity_id is required")

type Service struct {
	queries *sqlc.Queries
	bots    *bots.Service
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries, botService *bots.Service) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		bots:    botService,
		logger:  log.With(slog.String("service", "acl")),
	}
}

func (s *Service) AllowGuestEnabled(ctx context.Context, botID string) (bool, error) {
	if s == nil || s.queries == nil {
		return false, errors.New("acl queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return false, err
	}
	return s.queries.HasBotACLGuestAllAllowRule(ctx, pgBotID)
}

func (s *Service) SetAllowGuest(ctx context.Context, botID, createdByUserID string, enabled bool) error {
	if s == nil || s.queries == nil {
		return errors.New("acl queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	if enabled {
		_, err = s.queries.UpsertBotACLGuestAllAllowRule(ctx, sqlc.UpsertBotACLGuestAllAllowRuleParams{
			BotID:           pgBotID,
			CreatedByUserID: optionalUUID(createdByUserID),
		})
		return err
	}
	return s.queries.DeleteBotACLGuestAllAllowRule(ctx, pgBotID)
}

func (s *Service) ListWhitelist(ctx context.Context, botID string) ([]Rule, error) {
	return s.listByEffect(ctx, botID, EffectAllow)
}

func (s *Service) ListBlacklist(ctx context.Context, botID string) ([]Rule, error) {
	return s.listByEffect(ctx, botID, EffectDeny)
}

func (s *Service) AddWhitelistEntry(ctx context.Context, botID, createdByUserID string, req UpsertRuleRequest) (Rule, error) {
	return s.upsertEntry(ctx, botID, createdByUserID, EffectAllow, req)
}

func (s *Service) AddBlacklistEntry(ctx context.Context, botID, createdByUserID string, req UpsertRuleRequest) (Rule, error) {
	return s.upsertEntry(ctx, botID, createdByUserID, EffectDeny, req)
}

func (s *Service) DeleteRule(ctx context.Context, ruleID string) error {
	if s == nil || s.queries == nil {
		return errors.New("acl queries not configured")
	}
	pgRuleID, err := db.ParseUUID(ruleID)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotACLRuleByID(ctx, pgRuleID)
}

func (s *Service) CanPerformChatTrigger(ctx context.Context, botID, userID, channelIdentityID string) (bool, error) {
	if s == nil || s.queries == nil || s.bots == nil {
		return false, errors.New("acl service not configured")
	}
	botID = strings.TrimSpace(botID)
	userID = strings.TrimSpace(userID)
	channelIdentityID = strings.TrimSpace(channelIdentityID)

	bot, err := s.bots.Get(ctx, botID)
	if err != nil {
		return false, err
	}
	if userID != "" && strings.TrimSpace(bot.OwnerUserID) == userID {
		return true, nil
	}

	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return false, err
	}
	if userID != "" {
		matched, err := s.queries.HasBotACLUserRule(ctx, sqlc.HasBotACLUserRuleParams{
			BotID:  pgBotID,
			Effect: EffectDeny,
			UserID: optionalUUID(userID),
		})
		if err != nil {
			return false, err
		}
		if matched {
			return false, nil
		}
	}
	if channelIdentityID != "" {
		matched, err := s.queries.HasBotACLChannelIdentityRule(ctx, sqlc.HasBotACLChannelIdentityRuleParams{
			BotID:             pgBotID,
			Effect:            EffectDeny,
			ChannelIdentityID: optionalUUID(channelIdentityID),
		})
		if err != nil {
			return false, err
		}
		if matched {
			return false, nil
		}
	}
	if userID != "" {
		matched, err := s.queries.HasBotACLUserRule(ctx, sqlc.HasBotACLUserRuleParams{
			BotID:  pgBotID,
			Effect: EffectAllow,
			UserID: optionalUUID(userID),
		})
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	if channelIdentityID != "" {
		matched, err := s.queries.HasBotACLChannelIdentityRule(ctx, sqlc.HasBotACLChannelIdentityRuleParams{
			BotID:             pgBotID,
			Effect:            EffectAllow,
			ChannelIdentityID: optionalUUID(channelIdentityID),
		})
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return s.queries.HasBotACLGuestAllAllowRule(ctx, pgBotID)
}

func (s *Service) listByEffect(ctx context.Context, botID, effect string) ([]Rule, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("acl queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotACLSubjectRulesByEffect(ctx, sqlc.ListBotACLSubjectRulesByEffectParams{
		BotID:  pgBotID,
		Effect: effect,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Rule, 0, len(rows))
	for _, row := range rows {
		items = append(items, toRule(row))
	}
	return items, nil
}

func (s *Service) upsertEntry(ctx context.Context, botID, createdByUserID, effect string, req UpsertRuleRequest) (Rule, error) {
	if s == nil || s.queries == nil {
		return Rule{}, errors.New("acl queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Rule{}, err
	}
	userID := strings.TrimSpace(req.UserID)
	channelIdentityID := strings.TrimSpace(req.ChannelIdentityID)
	if (userID == "" && channelIdentityID == "") || (userID != "" && channelIdentityID != "") {
		return Rule{}, ErrInvalidRuleSubject
	}
	if userID != "" {
		row, err := s.queries.UpsertBotACLUserRule(ctx, sqlc.UpsertBotACLUserRuleParams{
			BotID:           pgBotID,
			Effect:          effect,
			UserID:          optionalUUID(userID),
			CreatedByUserID: optionalUUID(createdByUserID),
		})
		if err != nil {
			return Rule{}, err
		}
		return Rule{
			ID:          uuid.UUID(row.ID.Bytes).String(),
			BotID:       uuid.UUID(row.BotID.Bytes).String(),
			Action:      row.Action,
			Effect:      row.Effect,
			SubjectKind: row.SubjectKind,
			UserID:      uuid.UUID(row.UserID.Bytes).String(),
			CreatedAt:   timeFromPg(row.CreatedAt),
			UpdatedAt:   timeFromPg(row.UpdatedAt),
		}, nil
	}
	row, err := s.queries.UpsertBotACLChannelIdentityRule(ctx, sqlc.UpsertBotACLChannelIdentityRuleParams{
		BotID:             pgBotID,
		Effect:            effect,
		ChannelIdentityID: optionalUUID(channelIdentityID),
		CreatedByUserID:   optionalUUID(createdByUserID),
	})
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		ID:                uuid.UUID(row.ID.Bytes).String(),
		BotID:             uuid.UUID(row.BotID.Bytes).String(),
		Action:            row.Action,
		Effect:            row.Effect,
		SubjectKind:       row.SubjectKind,
		ChannelIdentityID: uuid.UUID(row.ChannelIdentityID.Bytes).String(),
		CreatedAt:         timeFromPg(row.CreatedAt),
		UpdatedAt:         timeFromPg(row.UpdatedAt),
	}, nil
}

func toRule(row sqlc.ListBotACLSubjectRulesByEffectRow) Rule {
	rule := Rule{
		ID:                         uuid.UUID(row.ID.Bytes).String(),
		BotID:                      uuid.UUID(row.BotID.Bytes).String(),
		Action:                     row.Action,
		Effect:                     row.Effect,
		SubjectKind:                row.SubjectKind,
		UserUsername:               strings.TrimSpace(row.UserUsername.String),
		UserDisplayName:            strings.TrimSpace(row.UserDisplayName.String),
		UserAvatarURL:              strings.TrimSpace(row.UserAvatarUrl.String),
		ChannelType:                strings.TrimSpace(row.ChannelType.String),
		ChannelSubjectID:           strings.TrimSpace(row.ChannelSubjectID.String),
		ChannelIdentityDisplayName: strings.TrimSpace(row.ChannelIdentityDisplayName.String),
		ChannelIdentityAvatarURL:   strings.TrimSpace(row.ChannelIdentityAvatarUrl.String),
		LinkedUserUsername:         strings.TrimSpace(row.LinkedUserUsername.String),
		LinkedUserDisplayName:      strings.TrimSpace(row.LinkedUserDisplayName.String),
		LinkedUserAvatarURL:        strings.TrimSpace(row.LinkedUserAvatarUrl.String),
		CreatedAt:                  timeFromPg(row.CreatedAt),
		UpdatedAt:                  timeFromPg(row.UpdatedAt),
	}
	if row.UserID.Valid {
		rule.UserID = uuid.UUID(row.UserID.Bytes).String()
	}
	if row.ChannelIdentityID.Valid {
		rule.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	if row.LinkedUserID.Valid {
		rule.LinkedUserID = uuid.UUID(row.LinkedUserID.Bytes).String()
	}
	return rule
}

func optionalUUID(value string) pgtype.UUID {
	parsed, err := db.ParseUUID(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}
	}
	return parsed
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
