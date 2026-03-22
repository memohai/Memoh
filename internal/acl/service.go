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

var (
	ErrInvalidRuleSubject = errors.New("exactly one of user_id or channel_identity_id is required")
	ErrInvalidSourceScope = errors.New("invalid source scope")
)

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

func (s *Service) CanPerformChatTrigger(ctx context.Context, req ChatTriggerRequest) (bool, error) {
	if s == nil {
		return false, errors.New("acl service not configured")
	}
	botID := strings.TrimSpace(req.BotID)
	userID := strings.TrimSpace(req.UserID)
	channelIdentityID := strings.TrimSpace(req.ChannelIdentityID)
	sourceScope, err := normalizeSourceScope(req.SourceScope)
	if err != nil {
		return false, err
	}
	if s.queries == nil || s.bots == nil {
		return false, errors.New("acl service not configured")
	}

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
			BotID:                  pgBotID,
			Effect:                 EffectDeny,
			UserID:                 optionalUUID(userID),
			SourceChannel:          optionalText(sourceScope.Channel),
			SourceConversationType: optionalText(sourceScope.ConversationType),
			SourceConversationID:   optionalText(sourceScope.ConversationID),
			SourceThreadID:         optionalText(sourceScope.ThreadID),
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
			BotID:                  pgBotID,
			Effect:                 EffectDeny,
			ChannelIdentityID:      optionalUUID(channelIdentityID),
			SourceChannel:          optionalText(sourceScope.Channel),
			SourceConversationType: optionalText(sourceScope.ConversationType),
			SourceConversationID:   optionalText(sourceScope.ConversationID),
			SourceThreadID:         optionalText(sourceScope.ThreadID),
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
			BotID:                  pgBotID,
			Effect:                 EffectAllow,
			UserID:                 optionalUUID(userID),
			SourceChannel:          optionalText(sourceScope.Channel),
			SourceConversationType: optionalText(sourceScope.ConversationType),
			SourceConversationID:   optionalText(sourceScope.ConversationID),
			SourceThreadID:         optionalText(sourceScope.ThreadID),
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
			BotID:                  pgBotID,
			Effect:                 EffectAllow,
			ChannelIdentityID:      optionalUUID(channelIdentityID),
			SourceChannel:          optionalText(sourceScope.Channel),
			SourceConversationType: optionalText(sourceScope.ConversationType),
			SourceConversationID:   optionalText(sourceScope.ConversationID),
			SourceThreadID:         optionalText(sourceScope.ThreadID),
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

func (s *Service) ListObservedConversationsByChannelIdentity(ctx context.Context, botID, channelIdentityID string) ([]ObservedConversationCandidate, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("acl queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListObservedConversationsByChannelIdentity(ctx, sqlc.ListObservedConversationsByChannelIdentityParams{
		BotID:             pgBotID,
		ChannelIdentityID: pgIdentityID,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ObservedConversationCandidate, 0, len(rows))
	for _, row := range rows {
		items = append(items, ObservedConversationCandidate{
			RouteID:          row.RouteID.String(),
			Channel:          strings.TrimSpace(row.Channel),
			ConversationType: strings.TrimSpace(row.ConversationType),
			ConversationID:   strings.TrimSpace(row.ConversationID),
			ThreadID:         strings.TrimSpace(row.ThreadID),
			ConversationName: strings.TrimSpace(row.ConversationName),
			LastObservedAt:   timeFromPg(row.LastObservedAt),
		})
	}
	return items, nil
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
	sourceScope, err := normalizeOptionalSourceScope(req.SourceScope)
	if err != nil {
		return Rule{}, err
	}
	if (userID == "" && channelIdentityID == "") || (userID != "" && channelIdentityID != "") {
		return Rule{}, ErrInvalidRuleSubject
	}
	if userID != "" {
		row, err := s.queries.UpsertBotACLUserRule(ctx, sqlc.UpsertBotACLUserRuleParams{
			BotID:                  pgBotID,
			Effect:                 effect,
			UserID:                 optionalUUID(userID),
			SourceChannel:          optionalText(sourceScope.Channel),
			SourceConversationType: optionalText(sourceScope.ConversationType),
			SourceConversationID:   optionalText(sourceScope.ConversationID),
			SourceThreadID:         optionalText(sourceScope.ThreadID),
			CreatedByUserID:        optionalUUID(createdByUserID),
		})
		if err != nil {
			return Rule{}, err
		}
		return ruleFromWriteRow(
			row.ID,
			row.BotID,
			row.Action,
			row.Effect,
			row.SubjectKind,
			row.UserID,
			row.ChannelIdentityID,
			row.SourceChannel,
			row.SourceConversationType,
			row.SourceConversationID,
			row.SourceThreadID,
			row.CreatedAt,
			row.UpdatedAt,
		), nil
	}
	sourceScope, err = s.normalizeChannelIdentitySourceScope(ctx, channelIdentityID, sourceScope)
	if err != nil {
		return Rule{}, err
	}
	row, err := s.queries.UpsertBotACLChannelIdentityRule(ctx, sqlc.UpsertBotACLChannelIdentityRuleParams{
		BotID:                  pgBotID,
		Effect:                 effect,
		ChannelIdentityID:      optionalUUID(channelIdentityID),
		SourceChannel:          optionalText(sourceScope.Channel),
		SourceConversationType: optionalText(sourceScope.ConversationType),
		SourceConversationID:   optionalText(sourceScope.ConversationID),
		SourceThreadID:         optionalText(sourceScope.ThreadID),
		CreatedByUserID:        optionalUUID(createdByUserID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromWriteRow(
		row.ID,
		row.BotID,
		row.Action,
		row.Effect,
		row.SubjectKind,
		row.UserID,
		row.ChannelIdentityID,
		row.SourceChannel,
		row.SourceConversationType,
		row.SourceConversationID,
		row.SourceThreadID,
		row.CreatedAt,
		row.UpdatedAt,
	), nil
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
	rule.SourceScope = sourceScopeFromPg(row.SourceChannel, row.SourceConversationType, row.SourceConversationID, row.SourceThreadID)
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

func optionalText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func normalizeSourceScope(scope SourceScope) (SourceScope, error) {
	normalized := scope.Normalize()
	if normalized.ThreadID != "" && normalized.ConversationID == "" {
		return SourceScope{}, ErrInvalidSourceScope
	}
	if (normalized.ConversationID != "" || normalized.ThreadID != "") && normalized.Channel == "" {
		return SourceScope{}, ErrInvalidSourceScope
	}
	return normalized, nil
}

func normalizeOptionalSourceScope(scope *SourceScope) (SourceScope, error) {
	if scope == nil {
		return SourceScope{}, nil
	}
	normalized, err := normalizeSourceScope(*scope)
	if err != nil {
		return SourceScope{}, err
	}
	return normalized, nil
}

func (s *Service) normalizeChannelIdentitySourceScope(ctx context.Context, channelIdentityID string, sourceScope SourceScope) (SourceScope, error) {
	channelIdentityID = strings.TrimSpace(channelIdentityID)
	if channelIdentityID == "" {
		return sourceScope, nil
	}
	if s == nil || s.queries == nil {
		return SourceScope{}, errors.New("acl queries not configured")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return SourceScope{}, err
	}
	identityRow, err := s.queries.GetChannelIdentityByID(ctx, pgChannelIdentityID)
	if err != nil {
		return SourceScope{}, err
	}
	sourceScope.Channel = strings.TrimSpace(identityRow.ChannelType)
	return normalizeSourceScope(sourceScope)
}

func sourceScopeFromPg(channelValue, conversationTypeValue, conversationIDValue, threadIDValue pgtype.Text) *SourceScope {
	scope := SourceScope{
		Channel:          strings.TrimSpace(channelValue.String),
		ConversationType: strings.TrimSpace(conversationTypeValue.String),
		ConversationID:   strings.TrimSpace(conversationIDValue.String),
		ThreadID:         strings.TrimSpace(threadIDValue.String),
	}
	if scope.IsZero() {
		return nil
	}
	return &scope
}

func ruleFromWriteRow(
	id pgtype.UUID,
	botID pgtype.UUID,
	action string,
	effect string,
	subjectKind string,
	userID pgtype.UUID,
	channelIdentityID pgtype.UUID,
	sourceChannel pgtype.Text,
	sourceConversationType pgtype.Text,
	sourceConversationID pgtype.Text,
	sourceThreadID pgtype.Text,
	createdAt pgtype.Timestamptz,
	updatedAt pgtype.Timestamptz,
) Rule {
	rule := Rule{
		ID:          uuid.UUID(id.Bytes).String(),
		BotID:       uuid.UUID(botID.Bytes).String(),
		Action:      action,
		Effect:      effect,
		SubjectKind: subjectKind,
		SourceScope: sourceScopeFromPg(sourceChannel, sourceConversationType, sourceConversationID, sourceThreadID),
		CreatedAt:   timeFromPg(createdAt),
		UpdatedAt:   timeFromPg(updatedAt),
	}
	if userID.Valid {
		rule.UserID = uuid.UUID(userID.Bytes).String()
	}
	if channelIdentityID.Valid {
		rule.ChannelIdentityID = uuid.UUID(channelIdentityID.Bytes).String()
	}
	return rule
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
