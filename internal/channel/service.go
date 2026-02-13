package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service provides CRUD operations for channel configurations, user bindings, and sessions.
type Service struct {
	queries  *sqlc.Queries
	registry *Registry
}

// NewService creates a Service backed by the given database queries and adapter registry.
func NewService(queries *sqlc.Queries, registry *Registry) *Service {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Service{queries: queries, registry: registry}
}

// UpsertConfig creates or updates a bot's channel configuration.
func (s *Service) UpsertConfig(ctx context.Context, botID string, channelType ChannelType, req UpsertConfigRequest) (ChannelConfig, error) {
	if s.queries == nil {
		return ChannelConfig{}, fmt.Errorf("channel queries not configured")
	}
	if channelType == "" {
		return ChannelConfig{}, fmt.Errorf("channel type is required")
	}
	normalized, err := s.registry.NormalizeConfig(channelType, req.Credentials)
	if err != nil {
		return ChannelConfig{}, err
	}
	credentialsPayload, err := json.Marshal(normalized)
	if err != nil {
		return ChannelConfig{}, err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return ChannelConfig{}, err
	}
	selfIdentity := req.SelfIdentity
	if selfIdentity == nil {
		selfIdentity = map[string]any{}
	}
	externalIdentity := strings.TrimSpace(req.ExternalIdentity)
	if discovered, extID, err := s.registry.DiscoverSelf(ctx, channelType, normalized); err == nil && discovered != nil {
		for k, v := range discovered {
			if _, exists := selfIdentity[k]; !exists {
				selfIdentity[k] = v
			}
		}
		if externalIdentity == "" && strings.TrimSpace(extID) != "" {
			externalIdentity = strings.TrimSpace(extID)
		}
	}
	selfPayload, err := json.Marshal(selfIdentity)
	if err != nil {
		return ChannelConfig{}, err
	}
	routing := req.Routing
	if routing == nil {
		routing = map[string]any{}
	}
	routingPayload, err := json.Marshal(routing)
	if err != nil {
		return ChannelConfig{}, err
	}
	status, err := normalizeChannelConfigStatus(req.Status)
	if err != nil {
		return ChannelConfig{}, err
	}
	verifiedAt := pgtype.Timestamptz{Valid: false}
	if req.VerifiedAt != nil {
		verifiedAt = pgtype.Timestamptz{Time: req.VerifiedAt.UTC(), Valid: true}
	}
	row, err := s.queries.UpsertBotChannelConfig(ctx, sqlc.UpsertBotChannelConfigParams{
		BotID:       botUUID,
		ChannelType: channelType.String(),
		Credentials: credentialsPayload,
		ExternalIdentity: pgtype.Text{
			String: externalIdentity,
			Valid:  externalIdentity != "",
		},
		SelfIdentity: selfPayload,
		Routing:      routingPayload,
		Capabilities: []byte("{}"),
		Status:       status,
		VerifiedAt:   verifiedAt,
	})
	if err != nil {
		return ChannelConfig{}, err
	}
	return normalizeChannelConfig(row)
}

func normalizeChannelConfigStatus(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	if status == "" {
		return "pending", nil
	}
	switch status {
	case "pending", "verified", "disabled":
		return status, nil
	case "active":
		return "verified", nil
	case "inactive":
		return "disabled", nil
	default:
		return "", fmt.Errorf("invalid channel status: %s", raw)
	}
}

// UpsertChannelIdentityConfig creates or updates a channel identity's channel binding.
func (s *Service) UpsertChannelIdentityConfig(ctx context.Context, channelIdentityID string, channelType ChannelType, req UpsertChannelIdentityConfigRequest) (ChannelIdentityBinding, error) {
	if s.queries == nil {
		return ChannelIdentityBinding{}, fmt.Errorf("channel queries not configured")
	}
	if channelType == "" {
		return ChannelIdentityBinding{}, fmt.Errorf("channel type is required")
	}
	normalized, err := s.registry.NormalizeUserConfig(channelType, req.Config)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	row, err := s.queries.UpsertUserChannelBinding(ctx, sqlc.UpsertUserChannelBindingParams{
		UserID:      pgChannelIdentityID,
		ChannelType: channelType.String(),
		Config:      payload,
	})
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	return normalizeChannelIdentityBinding(row)
}

// ResolveEffectiveConfig returns the active channel configuration for a bot.
// For configless channel types, a synthetic config is returned.
func (s *Service) ResolveEffectiveConfig(ctx context.Context, botID string, channelType ChannelType) (ChannelConfig, error) {
	if s.queries == nil {
		return ChannelConfig{}, fmt.Errorf("channel queries not configured")
	}
	if channelType == "" {
		return ChannelConfig{}, fmt.Errorf("channel type is required")
	}
	if s.registry.IsConfigless(channelType) {
		return ChannelConfig{
			ID:          channelType.String() + ":" + strings.TrimSpace(botID),
			BotID:       strings.TrimSpace(botID),
			ChannelType: channelType,
		}, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return ChannelConfig{}, err
	}
	row, err := s.queries.GetBotChannelConfig(ctx, sqlc.GetBotChannelConfigParams{
		BotID:       botUUID,
		ChannelType: channelType.String(),
	})
	if err == nil {
		return normalizeChannelConfig(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ChannelConfig{}, err
	}
	return ChannelConfig{}, fmt.Errorf("channel config not found")
}

// ListConfigsByType returns all channel configurations of the given type.
func (s *Service) ListConfigsByType(ctx context.Context, channelType ChannelType) ([]ChannelConfig, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("channel queries not configured")
	}
	if s.registry.IsConfigless(channelType) {
		return []ChannelConfig{}, nil
	}
	rows, err := s.queries.ListBotChannelConfigsByType(ctx, channelType.String())
	if err != nil {
		return nil, err
	}
	items := make([]ChannelConfig, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeChannelConfig(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// GetChannelIdentityConfig returns the channel identity's channel binding for the given channel type.
func (s *Service) GetChannelIdentityConfig(ctx context.Context, channelIdentityID string, channelType ChannelType) (ChannelIdentityBinding, error) {
	if s.queries == nil {
		return ChannelIdentityBinding{}, fmt.Errorf("channel queries not configured")
	}
	if channelType == "" {
		return ChannelIdentityBinding{}, fmt.Errorf("channel type is required")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	row, err := s.queries.GetUserChannelBinding(ctx, sqlc.GetUserChannelBindingParams{
		UserID:      pgChannelIdentityID,
		ChannelType: channelType.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChannelIdentityBinding{}, fmt.Errorf("channel user config not found")
		}
		return ChannelIdentityBinding{}, err
	}
	config, err := DecodeConfigMap(row.Config)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	return ChannelIdentityBinding{
		ID:                row.ID.String(),
		ChannelType:       ChannelType(row.ChannelType),
		ChannelIdentityID: row.UserID.String(),
		Config:            config,
		CreatedAt:         db.TimeFromPg(row.CreatedAt),
		UpdatedAt:         db.TimeFromPg(row.UpdatedAt),
	}, nil
}

// ListChannelIdentityConfigsByType returns all channel identity bindings for the given channel type.
func (s *Service) ListChannelIdentityConfigsByType(ctx context.Context, channelType ChannelType) ([]ChannelIdentityBinding, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("channel queries not configured")
	}
	rows, err := s.queries.ListUserChannelBindingsByPlatform(ctx, channelType.String())
	if err != nil {
		return nil, err
	}
	items := make([]ChannelIdentityBinding, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeChannelIdentityBinding(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ResolveChannelIdentityBinding finds the channel identity ID whose channel binding matches the given criteria.
func (s *Service) ResolveChannelIdentityBinding(ctx context.Context, channelType ChannelType, criteria BindingCriteria) (string, error) {
	rows, err := s.ListChannelIdentityConfigsByType(ctx, channelType)
	if err != nil {
		return "", err
	}
	if _, ok := s.registry.Get(channelType); !ok {
		return "", fmt.Errorf("unsupported channel type: %s", channelType)
	}
	for _, row := range rows {
		if s.registry.MatchUserBinding(channelType, row.Config, criteria) {
			return row.ChannelIdentityID, nil
		}
	}
	return "", fmt.Errorf("channel user binding not found")
}

func normalizeChannelConfig(row sqlc.BotChannelConfig) (ChannelConfig, error) {
	credentials, err := DecodeConfigMap(row.Credentials)
	if err != nil {
		return ChannelConfig{}, err
	}
	selfIdentity, err := DecodeConfigMap(row.SelfIdentity)
	if err != nil {
		return ChannelConfig{}, err
	}
	routing, err := DecodeConfigMap(row.Routing)
	if err != nil {
		return ChannelConfig{}, err
	}
	verifiedAt := time.Time{}
	if row.VerifiedAt.Valid {
		verifiedAt = row.VerifiedAt.Time
	}
	externalIdentity := ""
	if row.ExternalIdentity.Valid {
		externalIdentity = strings.TrimSpace(row.ExternalIdentity.String)
	}
	return ChannelConfig{
		ID:               row.ID.String(),
		BotID:            row.BotID.String(),
		ChannelType:      ChannelType(row.ChannelType),
		Credentials:      credentials,
		ExternalIdentity: externalIdentity,
		SelfIdentity:     selfIdentity,
		Routing:          routing,
		Status:           strings.TrimSpace(row.Status),
		VerifiedAt:       verifiedAt,
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		UpdatedAt:        db.TimeFromPg(row.UpdatedAt),
	}, nil
}

func normalizeChannelIdentityBinding(row sqlc.UserChannelBinding) (ChannelIdentityBinding, error) {
	config, err := DecodeConfigMap(row.Config)
	if err != nil {
		return ChannelIdentityBinding{}, err
	}
	return ChannelIdentityBinding{
		ID:                row.ID.String(),
		ChannelType:       ChannelType(row.ChannelType),
		ChannelIdentityID: row.UserID.String(),
		Config:            config,
		CreatedAt:         db.TimeFromPg(row.CreatedAt),
		UpdatedAt:         db.TimeFromPg(row.UpdatedAt),
	}, nil
}
