package bots

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service provides bot CRUD and membership management.
type Service struct {
	queries            *sqlc.Queries
	logger             *slog.Logger
	containerLifecycle ContainerLifecycle
	checkers           []RuntimeChecker
}

const (
	botLifecycleOperationTimeout = 5 * time.Minute
)

var (
	ErrBotNotFound       = errors.New("bot not found")
	ErrBotAccessDenied   = errors.New("bot access denied")
	ErrOwnerUserNotFound = errors.New("owner user not found")
)

// AccessPolicy controls bot access behavior.
type AccessPolicy struct {
	AllowPublicMember bool
	AllowGuest        bool
}

// NewService creates a new bot service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "bots")),
	}
}

// SetContainerLifecycle registers a container lifecycle handler for bot operations.
func (s *Service) SetContainerLifecycle(lc ContainerLifecycle) {
	s.containerLifecycle = lc
}

// AddRuntimeChecker registers an additional runtime checker.
func (s *Service) AddRuntimeChecker(c RuntimeChecker) {
	if c != nil {
		s.checkers = append(s.checkers, c)
	}
}

// AuthorizeAccess checks whether userID may access the given bot.
func (s *Service) AuthorizeAccess(ctx context.Context, userID, botID string, isAdmin bool, policy AccessPolicy) (Bot, error) {
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
	if isAdmin || bot.OwnerUserID == userID {
		return bot, nil
	}
	if bot.Type == BotTypePublic {
		if policy.AllowPublicMember {
			if _, err := s.GetMember(ctx, botID, userID); err == nil {
				return bot, nil
			}
		}
		if policy.AllowGuest && bot.AllowGuest {
			return bot, nil
		}
	}
	return Bot{}, ErrBotAccessDenied
}

// Create creates a new bot owned by owner user.
func (s *Service) Create(ctx context.Context, ownerUserID string, req CreateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	ownerID := strings.TrimSpace(ownerUserID)
	if ownerID == "" {
		return Bot{}, fmt.Errorf("owner user id is required")
	}
	ownerUUID, err := db.ParseUUID(ownerID)
	if err != nil {
		return Bot{}, err
	}
	if err := s.ensureUserExists(ctx, ownerUUID); err != nil {
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
		Status:      BotStatusCreating,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	s.enqueueCreateLifecycle(bot.ID)
	return bot, nil
}

// Get returns a bot by its ID.
func (s *Service) Get(ctx context.Context, botID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

// ListByOwner returns bots owned by the given user.
func (s *Service) ListByOwner(ctx context.Context, ownerUserID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	ownerUUID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotsByOwner(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(asSQLCBot(row))
		if err != nil {
			return nil, err
		}
		if err := s.attachCheckSummary(ctx, &item, asSQLCBot(row)); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListByMember returns bots where the user is a member.
func (s *Service) ListByMember(ctx context.Context, channelIdentityID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	memberUUID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotsByMember(ctx, memberUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(asSQLCBot(row))
		if err != nil {
			return nil, err
		}
		if err := s.attachCheckSummary(ctx, &item, asSQLCBot(row)); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListAccessible returns all bots the user can access (owned or member).
func (s *Service) ListAccessible(ctx context.Context, channelIdentityID string) ([]Bot, error) {
	owned, err := s.ListByOwner(ctx, channelIdentityID)
	if err != nil {
		return nil, err
	}
	members, err := s.ListByMember(ctx, channelIdentityID)
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

// Update updates bot profile fields.
func (s *Service) Update(ctx context.Context, botID string, req UpdateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
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
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

// TransferOwner transfers bot ownership to another user.
func (s *Service) TransferOwner(ctx context.Context, botID string, ownerUserID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	ownerUUID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return Bot{}, err
	}
	if err := s.ensureUserExists(ctx, ownerUUID); err != nil {
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotOwner(ctx, sqlc.UpdateBotOwnerParams{
		ID:          botUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

// Delete removes a bot and its associated resources.
func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(row.Status) == BotStatusDeleting {
		return nil
	}
	if err := s.queries.UpdateBotStatus(ctx, sqlc.UpdateBotStatusParams{
		ID:     botUUID,
		Status: BotStatusDeleting,
	}); err != nil {
		return err
	}
	s.enqueueDeleteLifecycle(botID)
	return nil
}

// ListChecks evaluates runtime resource checks for a bot.
func (s *Service) ListChecks(ctx context.Context, botID string) ([]BotCheck, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	return s.buildRuntimeChecks(ctx, asSQLCBot(row), true)
}

func (s *Service) enqueueCreateLifecycle(botID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), botLifecycleOperationTimeout)
		defer cancel()

		if s.containerLifecycle != nil {
			if err := s.containerLifecycle.SetupBotContainer(ctx, botID); err != nil {
				s.logger.Error("bot container setup failed",
					slog.String("bot_id", botID),
					slog.Any("error", err),
				)
			}
		}

		if err := s.updateStatus(ctx, botID, BotStatusReady); err != nil {
			s.logger.Error("failed to update bot status to ready after create",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) enqueueDeleteLifecycle(botID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), botLifecycleOperationTimeout)
		defer cancel()

		if s.containerLifecycle != nil {
			if err := s.containerLifecycle.CleanupBotContainer(ctx, botID); err != nil {
				s.logger.Error("bot container cleanup failed",
					slog.String("bot_id", botID),
					slog.Any("error", err),
				)
			}
		}

		botUUID, err := db.ParseUUID(botID)
		if err != nil {
			s.logger.Error("invalid bot id while finalizing delete",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
			if err := s.updateStatus(ctx, botID, BotStatusReady); err != nil {
				s.logger.Error("revert bot status failed", slog.String("bot_id", botID), slog.Any("error", err))
			}
			return
		}
		if err := s.queries.DeleteBotByID(ctx, botUUID); err != nil {
			s.logger.Error("failed to delete bot after cleanup",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
			if err := s.updateStatus(ctx, botID, BotStatusReady); err != nil {
				s.logger.Error("revert bot status failed", slog.String("bot_id", botID), slog.Any("error", err))
			}
			return
		}
	}()
}

func (s *Service) updateStatus(ctx context.Context, botID, status string) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.UpdateBotStatus(ctx, sqlc.UpdateBotStatusParams{
		ID:     botUUID,
		Status: strings.TrimSpace(status),
	})
}

func (s *Service) ensureUserExists(ctx context.Context, userID pgtype.UUID) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	_, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOwnerUserNotFound
		}
		return err
	}
	return nil
}

// UpsertMember creates or updates a bot membership.
func (s *Service) UpsertMember(ctx context.Context, botID string, req UpsertMemberRequest) (BotMember, error) {
	if s.queries == nil {
		return BotMember{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return BotMember{}, err
	}
	memberUUID, err := db.ParseUUID(req.UserID)
	if err != nil {
		return BotMember{}, err
	}
	role, err := normalizeMemberRole(req.Role)
	if err != nil {
		return BotMember{}, err
	}
	row, err := s.queries.UpsertBotMember(ctx, sqlc.UpsertBotMemberParams{
		BotID:  botUUID,
		UserID: memberUUID,
		Role:   role,
	})
	if err != nil {
		return BotMember{}, err
	}
	return toBotMember(row), nil
}

// ListMembers returns all members of a bot.
func (s *Service) ListMembers(ctx context.Context, botID string) ([]BotMember, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
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

// GetMember returns a specific bot member.
func (s *Service) GetMember(ctx context.Context, botID, channelIdentityID string) (BotMember, error) {
	if s.queries == nil {
		return BotMember{}, fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return BotMember{}, err
	}
	memberUUID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return BotMember{}, err
	}
	row, err := s.queries.GetBotMember(ctx, sqlc.GetBotMemberParams{
		BotID:  botUUID,
		UserID: memberUUID,
	})
	if err != nil {
		return BotMember{}, err
	}
	return toBotMember(row), nil
}

// DeleteMember removes a member from a bot.
func (s *Service) DeleteMember(ctx context.Context, botID, channelIdentityID string) error {
	if s.queries == nil {
		return fmt.Errorf("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	memberUUID, err := db.ParseUUID(channelIdentityID)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotMember(ctx, sqlc.DeleteBotMemberParams{
		BotID:  botUUID,
		UserID: memberUUID,
	})
}

// UpsertMemberSimple creates or updates a bot membership with a direct channel identity ID and role.
// This satisfies the router.BotMemberService interface.
func (s *Service) UpsertMemberSimple(ctx context.Context, botID, channelIdentityID, role string) error {
	_, err := s.UpsertMember(ctx, botID, UpsertMemberRequest{
		UserID: channelIdentityID,
		Role:   role,
	})
	return err
}

// IsMember checks if a user is a member of a bot.
func (s *Service) IsMember(ctx context.Context, botID, channelIdentityID string) (bool, error) {
	_, err := s.GetMember(ctx, botID, channelIdentityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func normalizeBotType(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return BotTypePersonal, nil
	}
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

func asSQLCBot(v any) sqlc.Bot {
	switch r := v.(type) {
	case sqlc.Bot:
		return r
	case sqlc.CreateBotRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.GetBotByIDRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.ListBotsByOwnerRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.ListBotsByMemberRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.UpdateBotProfileRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.UpdateBotOwnerRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, Type: r.Type, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, IsActive: r.IsActive, Status: r.Status, MaxContextLoadTime: r.MaxContextLoadTime, MaxContextTokens: r.MaxContextTokens, MaxInboxItems: r.MaxInboxItems, Language: r.Language, AllowGuest: r.AllowGuest, ChatModelID: r.ChatModelID, MemoryModelID: r.MemoryModelID, EmbeddingModelID: r.EmbeddingModelID, SearchProviderID: r.SearchProviderID, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	default:
		return sqlc.Bot{}
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
		ID:              row.ID.String(),
		OwnerUserID:     row.OwnerUserID.String(),
		Type:            row.Type,
		DisplayName:     displayName,
		AvatarURL:       avatarURL,
		IsActive:        row.IsActive,
		AllowGuest:      row.AllowGuest,
		Status:          strings.TrimSpace(row.Status),
		CheckState:      BotCheckStateUnknown,
		CheckIssueCount: 0,
		Metadata:        metadata,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func toBotMember(row sqlc.BotMember) BotMember {
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time
	}
	return BotMember{
		BotID:     row.BotID.String(),
		UserID:    row.UserID.String(),
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

func (s *Service) attachCheckSummary(ctx context.Context, bot *Bot, row sqlc.Bot) error {
	checks, err := s.buildRuntimeChecks(ctx, row, false)
	if err != nil {
		return err
	}
	checkState, issueCount := summarizeChecks(checks)
	bot.CheckState = checkState
	bot.CheckIssueCount = issueCount
	return nil
}

// buildRuntimeChecks composes builtin checks and optional dynamic checker results.
// includeDynamic is disabled when computing list summary to avoid expensive runtime probes.
func (s *Service) buildRuntimeChecks(ctx context.Context, row sqlc.Bot, includeDynamic bool) ([]BotCheck, error) {
	status := strings.TrimSpace(row.Status)
	checks := make([]BotCheck, 0, 4)

	if status == BotStatusCreating {
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerInit,
			Type:     BotCheckTypeContainerInit,
			TitleKey: "bots.checks.titles.containerInit",
			Status:   BotCheckStatusUnknown,
			Summary:  "Initialization is in progress.",
			Detail:   "Bot resources are still being provisioned.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerRecord,
			Type:     BotCheckTypeContainerRecord,
			TitleKey: "bots.checks.titles.containerRecord",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container record is pending.",
			Detail:   "Container record will be checked after initialization.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerTask,
			Type:     BotCheckTypeContainerTask,
			TitleKey: "bots.checks.titles.containerTask",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container task state is pending.",
			Detail:   "Task state will be checked after initialization.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerData,
			Type:     BotCheckTypeContainerData,
			TitleKey: "bots.checks.titles.containerDataPath",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container host path check is pending.",
			Detail:   "Data path will be checked after initialization.",
		})
		if includeDynamic {
			checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
		}
		return checks, nil
	}
	if status == BotStatusDeleting {
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeDelete,
			Type:     BotCheckTypeDelete,
			TitleKey: "bots.checks.titles.botDelete",
			Status:   BotCheckStatusUnknown,
			Summary:  "Deletion is in progress.",
			Detail:   "Bot resources are being cleaned up.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerRecord,
			Type:     BotCheckTypeContainerRecord,
			TitleKey: "bots.checks.titles.containerRecord",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container record check is skipped.",
			Detail:   "Bot is deleting and container checks are paused.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerTask,
			Type:     BotCheckTypeContainerTask,
			TitleKey: "bots.checks.titles.containerTask",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container task check is skipped.",
			Detail:   "Bot is deleting and task checks are paused.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerData,
			Type:     BotCheckTypeContainerData,
			TitleKey: "bots.checks.titles.containerDataPath",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container host path check is skipped.",
			Detail:   "Bot is deleting and data path checks are paused.",
		})
		if includeDynamic {
			checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
		}
		return checks, nil
	}

	checks = append(checks, BotCheck{
		ID:       BotCheckTypeContainerInit,
		Type:     BotCheckTypeContainerInit,
		TitleKey: "bots.checks.titles.containerInit",
		Status:   BotCheckStatusOK,
		Summary:  "Initialization finished.",
	})

	containerRow, err := s.queries.GetContainerByBotID(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerRecord,
				Type:     BotCheckTypeContainerRecord,
				TitleKey: "bots.checks.titles.containerRecord",
				Status:   BotCheckStatusError,
				Summary:  "Container record is missing.",
				Detail:   "No container is attached to this bot.",
			})
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerTask,
				Type:     BotCheckTypeContainerTask,
				TitleKey: "bots.checks.titles.containerTask",
				Status:   BotCheckStatusUnknown,
				Summary:  "Container task state is unknown.",
				Detail:   "Task state cannot be determined without a container record.",
			})
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerData,
				Type:     BotCheckTypeContainerData,
				TitleKey: "bots.checks.titles.containerDataPath",
				Status:   BotCheckStatusUnknown,
				Summary:  "Container data path is unknown.",
				Detail:   "Data path cannot be determined without a container record.",
			})
			if includeDynamic {
				checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
			}
			return checks, nil
		}
		return nil, err
	}

	checks = append(checks, BotCheck{
		ID:       BotCheckTypeContainerRecord,
		Type:     BotCheckTypeContainerRecord,
		TitleKey: "bots.checks.titles.containerRecord",
		Status:   BotCheckStatusOK,
		Summary:  "Container record exists.",
		Detail:   fmt.Sprintf("container_id=%s", strings.TrimSpace(containerRow.ContainerID)),
		Metadata: map[string]any{
			"container_id": strings.TrimSpace(containerRow.ContainerID),
			"namespace":    strings.TrimSpace(containerRow.Namespace),
			"image":        strings.TrimSpace(containerRow.Image),
		},
	})

	taskStatus := strings.TrimSpace(strings.ToLower(containerRow.Status))
	taskCheck := BotCheck{
		ID:       BotCheckTypeContainerTask,
		Type:     BotCheckTypeContainerTask,
		TitleKey: "bots.checks.titles.containerTask",
		Status:   BotCheckStatusWarn,
		Summary:  "Container task state needs attention.",
	}
	switch taskStatus {
	case "running", "created", "stopped", "paused":
		taskCheck.Status = BotCheckStatusOK
		taskCheck.Summary = "Container task state is reported."
		taskCheck.Detail = fmt.Sprintf("status=%s", taskStatus)
	case "":
		taskCheck.Detail = "status is empty"
	default:
		taskCheck.Detail = fmt.Sprintf("unexpected status=%s", taskStatus)
	}
	taskCheck.Metadata = map[string]any{"status": taskStatus}
	checks = append(checks, taskCheck)

	hostPath := ""
	if containerRow.HostPath.Valid {
		hostPath = strings.TrimSpace(containerRow.HostPath.String)
	}
	dataCheck := BotCheck{
		ID:       BotCheckTypeContainerData,
		Type:     BotCheckTypeContainerData,
		TitleKey: "bots.checks.titles.containerDataPath",
		Status:   BotCheckStatusWarn,
		Summary:  "Container host path needs attention.",
		Metadata: map[string]any{"host_path": hostPath},
	}
	if hostPath == "" {
		dataCheck.Detail = "host path is empty"
		checks = append(checks, dataCheck)
		if includeDynamic {
			checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
		}
		return checks, nil
	}
	info, statErr := os.Stat(hostPath)
	switch {
	case statErr == nil && info != nil && info.IsDir():
		dataCheck.Status = BotCheckStatusOK
		dataCheck.Summary = "Container host path is accessible."
		dataCheck.Detail = hostPath
	case statErr == nil:
		dataCheck.Status = BotCheckStatusError
		dataCheck.Summary = "Container host path is invalid."
		dataCheck.Detail = "host path is not a directory"
	case errors.Is(statErr, os.ErrNotExist):
		dataCheck.Status = BotCheckStatusError
		dataCheck.Summary = "Container host path does not exist."
		dataCheck.Detail = hostPath
	default:
		dataCheck.Status = BotCheckStatusWarn
		dataCheck.Summary = "Container host path cannot be checked."
		dataCheck.Detail = statErr.Error()
	}
	checks = append(checks, dataCheck)
	if includeDynamic {
		checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
	}

	return checks, nil
}

// appendDynamicChecks appends checks from registered runtime checkers.
func (s *Service) appendDynamicChecks(ctx context.Context, botID string, checks []BotCheck) []BotCheck {
	for _, checker := range s.checkers {
		items := checker.ListChecks(ctx, botID)
		for _, item := range items {
			item.ID = strings.TrimSpace(item.ID)
			item.Type = strings.TrimSpace(item.Type)
			item.Status = strings.TrimSpace(item.Status)
			if item.ID == "" {
				if item.Type != "" {
					item.ID = item.Type
				} else {
					item.ID = "runtime.unknown"
					if s.logger != nil {
						s.logger.Warn("runtime checker returned check without id and type",
							slog.String("bot_id", botID))
					}
				}
			}
			if item.Type == "" {
				item.Type = item.ID
			}
			if item.Status == "" {
				item.Status = BotCheckStatusUnknown
			}
			checks = append(checks, item)
		}
	}
	return checks
}

func summarizeChecks(checks []BotCheck) (string, int32) {
	if len(checks) == 0 {
		return BotCheckStateUnknown, 0
	}
	var issueCount int32
	unknownCount := 0
	for _, check := range checks {
		switch check.Status {
		case BotCheckStatusWarn, BotCheckStatusError:
			issueCount++
		case BotCheckStatusUnknown:
			unknownCount++
		}
	}
	if issueCount > 0 {
		return BotCheckStateIssue, issueCount
	}
	if unknownCount == len(checks) {
		return BotCheckStateUnknown, 0
	}
	return BotCheckStateOK, 0
}
