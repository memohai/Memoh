package bots

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

var (
	ErrBotNotFound     = errors.New("bot not found")
	ErrBotAccessDenied = errors.New("bot access denied")
)

type AccessPolicy struct {
	AllowPublicMember bool
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "bots")),
	}
}

func (s *Service) AuthorizeAccess(ctx context.Context, actorID, botID string, isAdmin bool, policy AccessPolicy) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	bot, err := s.Get(ctx, botID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bot{}, ErrBotNotFound
		}
		return Bot{}, err
	}
	if isAdmin || bot.OwnerUserID == actorID {
		return bot, nil
	}
	if policy.AllowPublicMember && bot.Type == BotTypePublic {
		if _, err := s.GetMember(ctx, botID, actorID); err == nil {
			return bot, nil
		}
	}
	return Bot{}, ErrBotAccessDenied
}

func (s *Service) Create(ctx context.Context, ownerUserID string, req CreateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	ownerID := strings.TrimSpace(ownerUserID)
	if ownerID == "" {
		return Bot{}, fmt.Errorf("owner user id is required")
	}
	ownerUUID, err := parseUUID(ownerID)
	if err != nil {
		return Bot{}, err
	}
	normalizedType, err := normalizeBotType(req.Type)
	if err != nil {
		return Bot{}, err
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = "bot-" + uuid.NewString()
	}
	avatarURL := strings.TrimSpace(req.AvatarURL)
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.CreateBot(ctx, sqlc.CreateBotParams{
		OwnerUserID: ownerUUID,
		Type:        normalizedType,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    isActive,
		Metadata:    payload,
	})
	if err != nil {
		return Bot{}, err
	}
	return toBot(row)
}

func (s *Service) Get(ctx context.Context, botID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	return toBot(row)
}

func (s *Service) ListByOwner(ctx context.Context, ownerUserID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	ownerUUID, err := parseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotsByOwner(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListByMember(ctx context.Context, userID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotsByMember(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) ListAccessible(ctx context.Context, userID string) ([]Bot, error) {
	owned, err := s.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	members, err := s.ListByMember(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[string]Bot{}
	for _, item := range owned {
		seen[item.ID] = item
	}
	for _, item := range members {
		if _, ok := seen[item.ID]; !ok {
			seen[item.ID] = item
		}
	}
	items := make([]Bot, 0, len(seen))
	for _, item := range seen {
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, botID string, req UpdateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	existing, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	isActive := existing.IsActive
	metadata, err := decodeMetadata(existing.Metadata)
	if err != nil {
		return Bot{}, err
	}
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if req.Metadata != nil {
		metadata = req.Metadata
	}
	if displayName == "" {
		displayName = "bot-" + uuid.NewString()
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotProfile(ctx, sqlc.UpdateBotProfileParams{
		ID:          botUUID,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    isActive,
		Metadata:    payload,
	})
	if err != nil {
		return Bot{}, err
	}
	return toBot(row)
}

func (s *Service) TransferOwner(ctx context.Context, botID string, ownerUserID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	ownerUUID, err := parseUUID(ownerUserID)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotOwner(ctx, sqlc.UpdateBotOwnerParams{
		ID:          botUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		return Bot{}, err
	}
	return toBot(row)
}

func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return err
	}
	if _, err := s.queries.GetBotByID(ctx, botUUID); err != nil {
		return err
	}
	return s.queries.DeleteBotByID(ctx, botUUID)
}

func (s *Service) UpsertMember(ctx context.Context, botID string, req UpsertMemberRequest) (BotMember, error) {
	if s.queries == nil {
		return BotMember{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return BotMember{}, err
	}
	userUUID, err := parseUUID(req.UserID)
	if err != nil {
		return BotMember{}, err
	}
	role, err := normalizeMemberRole(req.Role)
	if err != nil {
		return BotMember{}, err
	}
	row, err := s.queries.UpsertBotMember(ctx, sqlc.UpsertBotMemberParams{
		BotID:  botUUID,
		UserID: userUUID,
		Role:   role,
	})
	if err != nil {
		return BotMember{}, err
	}
	return toBotMember(row), nil
}

func (s *Service) ListMembers(ctx context.Context, botID string) ([]BotMember, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotMembers(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	items := make([]BotMember, 0, len(rows))
	for _, row := range rows {
		items = append(items, toBotMember(row))
	}
	return items, nil
}

func (s *Service) GetMember(ctx context.Context, botID, userID string) (BotMember, error) {
	if s.queries == nil {
		return BotMember{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return BotMember{}, err
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return BotMember{}, err
	}
	row, err := s.queries.GetBotMember(ctx, sqlc.GetBotMemberParams{
		BotID:  botUUID,
		UserID: userUUID,
	})
	if err != nil {
		return BotMember{}, err
	}
	return toBotMember(row), nil
}

func (s *Service) DeleteMember(ctx context.Context, botID, userID string) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return err
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotMember(ctx, sqlc.DeleteBotMemberParams{
		BotID:  botUUID,
		UserID: userUUID,
	})
}

func normalizeBotType(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case BotTypePersonal, BotTypePublic:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid bot type: %s", raw)
	}
}

func normalizeMemberRole(raw string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(raw))
	if role == "" {
		return MemberRoleMember, nil
	}
	switch role {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember:
		return role, nil
	default:
		return "", fmt.Errorf("invalid member role: %s", raw)
	}
}

func toBot(row sqlc.Bot) (Bot, error) {
	displayName := ""
	if row.DisplayName.Valid {
		displayName = row.DisplayName.String
	}
	avatarURL := ""
	if row.AvatarUrl.Valid {
		avatarURL = row.AvatarUrl.String
	}
	metadata, err := decodeMetadata(row.Metadata)
	if err != nil {
		return Bot{}, err
	}
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time
	}
	updatedAt := time.Time{}
	if row.UpdatedAt.Valid {
		updatedAt = row.UpdatedAt.Time
	}
	return Bot{
		ID:          toUUIDString(row.ID),
		OwnerUserID: toUUIDString(row.OwnerUserID),
		Type:        row.Type,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		IsActive:    row.IsActive,
		Metadata:    metadata,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func toBotMember(row sqlc.BotMember) BotMember {
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time
	}
	return BotMember{
		BotID:     toUUIDString(row.BotID),
		UserID:    toUUIDString(row.UserID),
		Role:      row.Role,
		CreatedAt: createdAt,
	}
}

func decodeMetadata(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
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

func toUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}
