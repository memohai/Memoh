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
func (s *Service) UpsertConfig(ctx context.Context, botID string, channelType Type, req UpsertConfigRequest) (Config, error) {
	if s.queries == nil {
		return Config{}, errors.New("channel queries not configured")
	}
	if channelType == "" {
		return Config{}, errors.New("channel type is required")
	}
	normalized, err := s.registry.NormalizeConfig(channelType, req.Credentials)
	if err != nil {
		return Config{}, err
	}
	credentialsPayload, err := json.Marshal(normalized)
	if err != nil {
		return Config{}, err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Config{}, err
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
		return Config{}, err
	}
	routing := req.Routing
	if routing == nil {
		routing = map[string]any{}
	}
	routingPayload, err := json.Marshal(routing)
	if err != nil {
		return Config{}, err
	}
	status, err := normalizeConfigStatus(req.Status)
	if err != nil {
		return Config{}, err
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
		return Config{}, err
	}
	return normalizeConfig(row)
}

func normalizeConfigStatus(raw string) (string, error) {
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
func (s *Service) UpsertChannelIdentityConfig(ctx context.Context, channelIdentityID string, channelType Type, req UpsertChannelIdentityConfigRequest) (IdentityBinding, error) {
	if s.queries == nil {
		return IdentityBinding{}, errors.New("channel queries not configured")
	}
	if channelType == "" {
		return IdentityBinding{}, errors.New("channel type is required")
	}
	normalized, err := s.registry.NormalizeUserConfig(channelType, req.Config)
	if err != nil {
		return IdentityBinding{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return IdentityBinding{}, err
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return IdentityBinding{}, err
	}
	row, err := s.queries.UpsertUserChannelBinding(ctx, sqlc.UpsertUserChannelBindingParams{
		UserID:      pgChannelIdentityID,
		ChannelType: channelType.String(),
		Config:      payload,
	})
	if err != nil {
		return IdentityBinding{}, err
	}
	return normalizeIdentityBinding(row)
}

// ResolveEffectiveConfig returns the active channel configuration for a bot.
// For configless channel types, a synthetic config is returned.
func (s *Service) ResolveEffectiveConfig(ctx context.Context, botID string, channelType Type) (Config, error) {
	if s.queries == nil {
		return Config{}, errors.New("channel queries not configured")
	}
	if channelType == "" {
		return Config{}, errors.New("channel type is required")
	}
	if s.registry.IsConfigless(channelType) {
		return Config{
			ID:    channelType.String() + ":" + strings.TrimSpace(botID),
			BotID: strings.TrimSpace(botID),
			Type:  channelType,
		}, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Config{}, err
	}
	row, err := s.queries.GetBotChannelConfig(ctx, sqlc.GetBotChannelConfigParams{
		BotID:       botUUID,
		ChannelType: channelType.String(),
	})
	if err == nil {
		return normalizeConfig(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Config{}, err
	}
	return Config{}, errors.New("channel config not found")
}

// ListConfigsByType returns all channel configurations of the given type.
func (s *Service) ListConfigsByType(ctx context.Context, channelType Type) ([]Config, error) {
	if s.queries == nil {
		return nil, errors.New("channel queries not configured")
	}
	if s.registry.IsConfigless(channelType) {
		return []Config{}, nil
	}
	rows, err := s.queries.ListBotChannelConfigsByType(ctx, channelType.String())
	if err != nil {
		return nil, err
	}
	items := make([]Config, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeConfig(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// GetChannelIdentityConfig returns the channel identity's channel binding for the given channel type.
func (s *Service) GetChannelIdentityConfig(ctx context.Context, channelIdentityID string, channelType Type) (IdentityBinding, error) {
	if s.queries == nil {
		return IdentityBinding{}, errors.New("channel queries not configured")
	}
	if channelType == "" {
		return IdentityBinding{}, errors.New("channel type is required")
	}
	pgChannelIdentityID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return IdentityBinding{}, err
	}
	row, err := s.queries.GetUserChannelBinding(ctx, sqlc.GetUserChannelBindingParams{
		UserID:      pgChannelIdentityID,
		ChannelType: channelType.String(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IdentityBinding{}, errors.New("channel user config not found")
		}
		return IdentityBinding{}, err
	}
	config, err := DecodeConfigMap(row.Config)
	if err != nil {
		return IdentityBinding{}, err
	}
	return IdentityBinding{
		ID:                row.ID.String(),
		Type:              Type(row.ChannelType),
		ChannelIdentityID: row.UserID.String(),
		Config:            config,
		CreatedAt:         db.TimeFromPg(row.CreatedAt),
		UpdatedAt:         db.TimeFromPg(row.UpdatedAt),
	}, nil
}

// ListChannelIdentityConfigsByType returns all channel identity bindings for the given channel type.
func (s *Service) ListChannelIdentityConfigsByType(ctx context.Context, channelType Type) ([]IdentityBinding, error) {
	if s.queries == nil {
		return nil, errors.New("channel queries not configured")
	}
	rows, err := s.queries.ListUserChannelBindingsByPlatform(ctx, channelType.String())
	if err != nil {
		return nil, err
	}
	items := make([]IdentityBinding, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeIdentityBinding(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ResolveIdentityBinding finds the channel identity ID whose channel binding matches the given criteria.
func (s *Service) ResolveIdentityBinding(ctx context.Context, channelType Type, criteria BindingCriteria) (string, error) {
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
	return "", errors.New("channel user binding not found")
}

func normalizeConfig(row sqlc.BotChannelConfig) (Config, error) {
	credentials, err := DecodeConfigMap(row.Credentials)
	if err != nil {
		return Config{}, err
	}
	selfIdentity, err := DecodeConfigMap(row.SelfIdentity)
	if err != nil {
		return Config{}, err
	}
	routing, err := DecodeConfigMap(row.Routing)
	if err != nil {
		return Config{}, err
	}
	verifiedAt := time.Time{}
	if row.VerifiedAt.Valid {
		verifiedAt = row.VerifiedAt.Time
	}
	externalIdentity := ""
	if row.ExternalIdentity.Valid {
		externalIdentity = strings.TrimSpace(row.ExternalIdentity.String)
	}
	return Config{
		ID:               row.ID.String(),
		BotID:            row.BotID.String(),
		Type:             Type(row.ChannelType),
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

func normalizeIdentityBinding(row sqlc.UserChannelBinding) (IdentityBinding, error) {
	config, err := DecodeConfigMap(row.Config)
	if err != nil {
		return IdentityBinding{}, err
	}
	return IdentityBinding{
		ID:                row.ID.String(),
		Type:              Type(row.ChannelType),
		ChannelIdentityID: row.UserID.String(),
		Config:            config,
		CreatedAt:         db.TimeFromPg(row.CreatedAt),
		UpdatedAt:         db.TimeFromPg(row.UpdatedAt),
	}, nil
}
