package acl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

var (
	ErrInvalidRuleSubject = errors.New("invalid rule subject: subject_kind does not match provided subject fields")
	ErrInvalidSourceScope = errors.New("invalid source scope")
	ErrInvalidEffect      = errors.New("effect must be 'allow' or 'deny'")
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "acl")),
	}
}

// Evaluate checks whether the given request is allowed to perform chat.trigger.
// It uses a single first-match-wins query over priority-ordered enabled rules,
// falling back to the bot's acl_default_effect if no rule matches.
func (s *Service) Evaluate(ctx context.Context, req EvaluateRequest) (bool, error) {
	// Validate scope before any service nil checks so callers get meaningful errors.
	sourceScope, err := normalizeSourceScope(req.SourceScope)
	if err != nil {
		return false, err
	}

	if s == nil || s.queries == nil {
		return false, errors.New("acl service not configured")
	}

	botID := strings.TrimSpace(req.BotID)
	channelIdentityID := strings.TrimSpace(req.ChannelIdentityID)
	channelType := strings.TrimSpace(req.ChannelType)

	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return false, err
	}

	effect, err := s.queries.EvaluateBotACLRule(ctx, sqlc.EvaluateBotACLRuleParams{
		BotID:                  pgBotID,
		Action:                 ActionChatTrigger,
		ChannelIdentityID:      optionalUUID(channelIdentityID),
		SubjectChannelType:     optionalText(channelType),
		SourceConversationType: optionalText(sourceScope.ConversationType),
		SourceConversationID:   optionalText(sourceScope.ConversationID),
		SourceThreadID:         optionalText(sourceScope.ThreadID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No rule matched — use the bot's default effect.
			defaultEffect, err := s.queries.GetBotACLDefaultEffect(ctx, pgBotID)
			if err != nil {
				return false, err
			}
			return defaultEffect == EffectAllow, nil
		}
		return false, err
	}
	return effect == EffectAllow, nil
}

// GetDefaultEffect returns the bot's fallback ACL effect.
func (s *Service) GetDefaultEffect(ctx context.Context, botID string) (string, error) {
	if s == nil || s.queries == nil {
		return "", errors.New("acl service not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return "", err
	}
	return s.queries.GetBotACLDefaultEffect(ctx, pgBotID)
}

// SetDefaultEffect sets the bot's fallback ACL effect.
func (s *Service) SetDefaultEffect(ctx context.Context, botID, effect string) error {
	if s == nil || s.queries == nil {
		return errors.New("acl service not configured")
	}
	effect = strings.TrimSpace(effect)
	if effect != EffectAllow && effect != EffectDeny {
		return ErrInvalidEffect
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.SetBotACLDefaultEffect(ctx, sqlc.SetBotACLDefaultEffectParams{
		ID:               pgBotID,
		AclDefaultEffect: effect,
	})
}

// ListRules returns all ACL rules for a bot ordered by priority.
func (s *Service) ListRules(ctx context.Context, botID string) ([]Rule, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("acl service not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotACLRules(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Rule, 0, len(rows))
	for _, row := range rows {
		items = append(items, ruleFromListRow(row))
	}
	return items, nil
}

// CreateRule creates a new ACL rule.
func (s *Service) CreateRule(ctx context.Context, botID, createdByUserID string, req CreateRuleRequest) (Rule, error) {
	if s == nil || s.queries == nil {
		return Rule{}, errors.New("acl service not configured")
	}
	if err := validateEffect(req.Effect); err != nil {
		return Rule{}, err
	}
	if err := validateSubject(req.SubjectKind, req.ChannelIdentityID, req.SubjectChannelType); err != nil {
		return Rule{}, err
	}
	sourceScope, err := normalizeOptionalSourceScope(req.SourceScope)
	if err != nil {
		return Rule{}, err
	}
	sourceChannel, err := s.resolveSourceChannel(ctx, sourceScope, req.SubjectKind, req.SubjectChannelType, req.ChannelIdentityID)
	if err != nil {
		return Rule{}, err
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Rule{}, err
	}
	row, err := s.queries.CreateBotACLRule(ctx, sqlc.CreateBotACLRuleParams{
		BotID:                  pgBotID,
		Priority:               req.Priority,
		Enabled:                req.Enabled,
		Description:            optionalText(req.Description),
		Effect:                 req.Effect,
		SubjectKind:            req.SubjectKind,
		ChannelIdentityID:      optionalUUID(req.ChannelIdentityID),
		SubjectChannelType:     optionalText(req.SubjectChannelType),
		SourceChannel:          optionalText(sourceChannel),
		SourceConversationType: optionalText(sourceScope.ConversationType),
		SourceConversationID:   optionalText(sourceScope.ConversationID),
		SourceThreadID:         optionalText(sourceScope.ThreadID),
		CreatedByUserID:        optionalUUID(createdByUserID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromWrite(row), nil
}

// UpdateRule updates an existing ACL rule.
func (s *Service) UpdateRule(ctx context.Context, ruleID string, req UpdateRuleRequest) (Rule, error) {
	if s == nil || s.queries == nil {
		return Rule{}, errors.New("acl service not configured")
	}
	if err := validateEffect(req.Effect); err != nil {
		return Rule{}, err
	}
	if err := validateSubject(req.SubjectKind, req.ChannelIdentityID, req.SubjectChannelType); err != nil {
		return Rule{}, err
	}
	sourceScope, err := normalizeOptionalSourceScope(req.SourceScope)
	if err != nil {
		return Rule{}, err
	}
	sourceChannel, err := s.resolveSourceChannel(ctx, sourceScope, req.SubjectKind, req.SubjectChannelType, req.ChannelIdentityID)
	if err != nil {
		return Rule{}, err
	}
	pgRuleID, err := db.ParseUUID(ruleID)
	if err != nil {
		return Rule{}, err
	}
	row, err := s.queries.UpdateBotACLRule(ctx, sqlc.UpdateBotACLRuleParams{
		ID:                     pgRuleID,
		Priority:               req.Priority,
		Enabled:                req.Enabled,
		Description:            optionalText(req.Description),
		Effect:                 req.Effect,
		SubjectKind:            req.SubjectKind,
		ChannelIdentityID:      optionalUUID(req.ChannelIdentityID),
		SubjectChannelType:     optionalText(req.SubjectChannelType),
		SourceChannel:          optionalText(sourceChannel),
		SourceConversationType: optionalText(sourceScope.ConversationType),
		SourceConversationID:   optionalText(sourceScope.ConversationID),
		SourceThreadID:         optionalText(sourceScope.ThreadID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromUpdateRow(row), nil
}

// resolveSourceChannel derives the source_channel value from the rule's subject context.
// source_channel is required by DB constraint whenever source_conversation_id or source_thread_id is set.
func (s *Service) resolveSourceChannel(ctx context.Context, scope SourceScope, subjectKind, subjectChannelType, channelIdentityID string) (string, error) {
	if scope.IsZero() {
		return "", nil
	}
	switch subjectKind {
	case SubjectKindChannelType:
		return strings.TrimSpace(subjectChannelType), nil
	case SubjectKindChannelIdentity:
		pgID, err := db.ParseUUID(strings.TrimSpace(channelIdentityID))
		if err != nil {
			return "", fmt.Errorf("resolve source channel: %w", err)
		}
		identity, err := s.queries.GetChannelIdentityByID(ctx, pgID)
		if err != nil {
			return "", fmt.Errorf("resolve source channel: get identity: %w", err)
		}
		return strings.TrimSpace(identity.ChannelType), nil
	default:
		return "", nil
	}
}

// DeleteRule removes an ACL rule by ID.
func (s *Service) DeleteRule(ctx context.Context, ruleID string) error {
	if s == nil || s.queries == nil {
		return errors.New("acl service not configured")
	}
	pgRuleID, err := db.ParseUUID(ruleID)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotACLRuleByID(ctx, pgRuleID)
}

// ReorderRules batch-updates the priority of multiple rules.
func (s *Service) ReorderRules(ctx context.Context, items []ReorderItem) error {
	if s == nil || s.queries == nil {
		return errors.New("acl service not configured")
	}
	for _, item := range items {
		pgID, err := db.ParseUUID(item.ID)
		if err != nil {
			return err
		}
		if err := s.queries.UpdateBotACLRulePriority(ctx, sqlc.UpdateBotACLRulePriorityParams{
			ID:       pgID,
			Priority: item.Priority,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListObservedConversationsByChannelIdentity returns conversations observed for a specific
// channel identity under a bot, useful for building scoped rule source selectors.
func (s *Service) ListObservedConversationsByChannelIdentity(ctx context.Context, botID, channelIdentityID string) ([]ObservedConversationCandidate, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("acl service not configured")
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

// ListObservedConversationsByChannelType returns conversations observed on a platform type
// for this bot (any sender), for scoped rule building when subject is channel_type.
func (s *Service) ListObservedConversationsByChannelType(ctx context.Context, botID, channelType string) ([]ObservedConversationCandidate, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("acl service not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	channelType = strings.TrimSpace(channelType)
	if channelType == "" {
		return nil, errors.New("channel_type is required")
	}
	rows, err := s.queries.ListObservedConversationsByChannelType(ctx, sqlc.ListObservedConversationsByChannelTypeParams{
		BotID:       pgBotID,
		ChannelType: channelType,
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

// ---- helpers ----

func validateEffect(effect string) error {
	switch strings.TrimSpace(effect) {
	case EffectAllow, EffectDeny:
		return nil
	}
	return ErrInvalidEffect
}

func validateSubject(kind, channelIdentityID, channelType string) error {
	kind = strings.TrimSpace(kind)
	channelIdentityID = strings.TrimSpace(channelIdentityID)
	channelType = strings.TrimSpace(channelType)
	switch kind {
	case SubjectKindAll:
		if channelIdentityID != "" || channelType != "" {
			return ErrInvalidRuleSubject
		}
	case SubjectKindChannelIdentity:
		if channelIdentityID == "" || channelType != "" {
			return ErrInvalidRuleSubject
		}
	case SubjectKindChannelType:
		if channelType == "" || channelIdentityID != "" {
			return ErrInvalidRuleSubject
		}
	default:
		return ErrInvalidRuleSubject
	}
	return nil
}

func normalizeSourceScope(scope SourceScope) (SourceScope, error) {
	normalized := scope.Normalize()
	if normalized.ThreadID != "" && normalized.ConversationID == "" {
		return SourceScope{}, ErrInvalidSourceScope
	}
	return normalized, nil
}

func normalizeOptionalSourceScope(scope *SourceScope) (SourceScope, error) {
	if scope == nil {
		return SourceScope{}, nil
	}
	return normalizeSourceScope(*scope)
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

func sourceScopeFromPg(conversationTypeValue, conversationIDValue, threadIDValue pgtype.Text) *SourceScope {
	scope := SourceScope{
		ConversationType: strings.TrimSpace(conversationTypeValue.String),
		ConversationID:   strings.TrimSpace(conversationIDValue.String),
		ThreadID:         strings.TrimSpace(threadIDValue.String),
	}
	if scope.IsZero() {
		return nil
	}
	return &scope
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}

func ruleFromListRow(row sqlc.ListBotACLRulesRow) Rule {
	rule := Rule{
		ID:                         uuid.UUID(row.ID.Bytes).String(),
		BotID:                      uuid.UUID(row.BotID.Bytes).String(),
		Priority:                   row.Priority,
		Enabled:                    row.Enabled,
		Description:                strings.TrimSpace(row.Description.String),
		Action:                     row.Action,
		Effect:                     row.Effect,
		SubjectKind:                row.SubjectKind,
		SubjectChannelType:         strings.TrimSpace(row.SubjectChannelType.String),
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
	rule.SourceScope = sourceScopeFromPg(row.SourceConversationType, row.SourceConversationID, row.SourceThreadID)
	if row.ChannelIdentityID.Valid {
		rule.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	if row.LinkedUserID.Valid {
		rule.LinkedUserID = uuid.UUID(row.LinkedUserID.Bytes).String()
	}
	return rule
}

func ruleFromWrite(row sqlc.CreateBotACLRuleRow) Rule {
	rule := Rule{
		ID:                 uuid.UUID(row.ID.Bytes).String(),
		BotID:              uuid.UUID(row.BotID.Bytes).String(),
		Priority:           row.Priority,
		Enabled:            row.Enabled,
		Description:        strings.TrimSpace(row.Description.String),
		Action:             row.Action,
		Effect:             row.Effect,
		SubjectKind:        row.SubjectKind,
		SubjectChannelType: strings.TrimSpace(row.SubjectChannelType.String),
		SourceScope:        sourceScopeFromPg(row.SourceConversationType, row.SourceConversationID, row.SourceThreadID),
		CreatedAt:          timeFromPg(row.CreatedAt),
		UpdatedAt:          timeFromPg(row.UpdatedAt),
	}
	if row.ChannelIdentityID.Valid {
		rule.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	return rule
}

func ruleFromUpdateRow(row sqlc.UpdateBotACLRuleRow) Rule {
	rule := Rule{
		ID:                 uuid.UUID(row.ID.Bytes).String(),
		BotID:              uuid.UUID(row.BotID.Bytes).String(),
		Priority:           row.Priority,
		Enabled:            row.Enabled,
		Description:        strings.TrimSpace(row.Description.String),
		Action:             row.Action,
		Effect:             row.Effect,
		SubjectKind:        row.SubjectKind,
		SubjectChannelType: strings.TrimSpace(row.SubjectChannelType.String),
		SourceScope:        sourceScopeFromPg(row.SourceConversationType, row.SourceConversationID, row.SourceThreadID),
		CreatedAt:          timeFromPg(row.CreatedAt),
		UpdatedAt:          timeFromPg(row.UpdatedAt),
	}
	if row.ChannelIdentityID.Valid {
		rule.ChannelIdentityID = uuid.UUID(row.ChannelIdentityID.Bytes).String()
	}
	return rule
}
