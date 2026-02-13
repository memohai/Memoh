package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

var (
	ErrChatNotFound     = errors.New("chat not found")
	ErrNotParticipant   = errors.New("not a participant")
	ErrPermissionDenied = errors.New("permission denied")
)

// Service manages conversation lifecycle, participants, and settings.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a conversation service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "conversation")),
	}
}

// Create creates a new conversation and adds the creator as owner.
func (s *Service) Create(ctx context.Context, botID, channelIdentityID string, req CreateRequest) (Conversation, error) {
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = KindDirect
	}
	if kind != KindDirect && kind != KindGroup && kind != KindThread {
		return Conversation{}, fmt.Errorf("invalid conversation kind: %s", kind)
	}

	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Conversation{}, fmt.Errorf("invalid bot id: %w", err)
	}
	pgChannelIdentityID := pgtype.UUID{}
	if strings.TrimSpace(channelIdentityID) != "" {
		pgChannelIdentityID, err = parseUUID(channelIdentityID)
		if err != nil {
			return Conversation{}, fmt.Errorf("invalid channel identity id: %w", err)
		}
	}

	var pgParent pgtype.UUID
	if kind == KindThread && strings.TrimSpace(req.ParentChatID) != "" {
		pgParent, err = parseUUID(req.ParentChatID)
		if err != nil {
			return Conversation{}, fmt.Errorf("invalid parent conversation id: %w", err)
		}
	}

	metadata, err := json.Marshal(nonNilMap(req.Metadata))
	if err != nil {
		return Conversation{}, fmt.Errorf("marshal conversation metadata: %w", err)
	}

	row, err := s.queries.CreateChat(ctx, sqlc.CreateChatParams{
		BotID:           pgBotID,
		Kind:            kind,
		ParentChatID:    pgParent,
		Title:           strings.TrimSpace(req.Title),
		CreatedByUserID: pgChannelIdentityID,
		Metadata:        metadata,
	})
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}

	// Add creator as owner when the channel identity is available.
	if pgChannelIdentityID.Valid {
		if _, err := s.queries.AddChatParticipant(ctx, sqlc.AddChatParticipantParams{
			ChatID: row.ID,
			UserID: pgChannelIdentityID,
			Role:   RoleOwner,
		}); err != nil {
			return Conversation{}, fmt.Errorf("add owner participant: %w", err)
		}
	}

	// For threads, copy parent participants.
	if kind == KindThread && pgParent.Valid {
		if err := s.queries.CopyParticipantsToChat(ctx, sqlc.CopyParticipantsToChatParams{
			ChatID:  pgParent,
			ChatID2: row.ID,
		}); err != nil {
			s.logger.Warn("copy parent participants failed", slog.Any("error", err))
		}
	}

	return toChatFromCreate(row), nil
}

// Get returns a conversation by ID.
func (s *Service) Get(ctx context.Context, conversationID string) (Conversation, error) {
	pgID, err := parseUUID(conversationID)
	if err != nil {
		return Conversation{}, ErrChatNotFound
	}
	row, err := s.queries.GetChatByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversation{}, ErrChatNotFound
		}
		return Conversation{}, err
	}
	return toChatFromGet(row), nil
}

// GetReadAccess resolves whether a user can read a conversation.
func (s *Service) GetReadAccess(ctx context.Context, conversationID, channelIdentityID string) (ConversationReadAccess, error) {
	pgConversationID, err := parseUUID(conversationID)
	if err != nil {
		return ConversationReadAccess{}, ErrPermissionDenied
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return ConversationReadAccess{}, ErrPermissionDenied
	}
	row, err := s.queries.GetChatReadAccessByUser(ctx, sqlc.GetChatReadAccessByUserParams{
		ChatID: pgConversationID,
		UserID: pgChannelIdentityID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConversationReadAccess{}, ErrPermissionDenied
		}
		return ConversationReadAccess{}, err
	}
	return ConversationReadAccess{
		AccessMode:      row.AccessMode,
		ParticipantRole: strings.TrimSpace(row.ParticipantRole),
		LastObservedAt:  pgTimePtr(row.LastObservedAt),
	}, nil
}

// ListByBotAndChannelIdentity returns all visible conversations for a bot and channel identity.
func (s *Service) ListByBotAndChannelIdentity(ctx context.Context, botID, channelIdentityID string) ([]ConversationListItem, error) {
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListVisibleChatsByBotAndUser(ctx, sqlc.ListVisibleChatsByBotAndUserParams{
		BotID:  pgBotID,
		UserID: pgChannelIdentityID,
	})
	if err != nil {
		return nil, err
	}
	conversations := make([]ConversationListItem, 0, len(rows))
	for _, row := range rows {
		conversations = append(conversations, toChatListItem(row))
	}
	return conversations, nil
}

// ListThreads returns threads for a parent conversation.
func (s *Service) ListThreads(ctx context.Context, parentConversationID string) ([]Conversation, error) {
	pgID, err := parseUUID(parentConversationID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListThreadsByParent(ctx, pgID)
	if err != nil {
		return nil, err
	}
	conversations := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		conversations = append(conversations, toChatFromThread(row))
	}
	return conversations, nil
}

// Delete deletes a conversation and linked records.
func (s *Service) Delete(ctx context.Context, conversationID string) error {
	pgID, err := parseUUID(conversationID)
	if err != nil {
		return ErrChatNotFound
	}
	return s.queries.DeleteChat(ctx, pgID)
}

// AddParticipant adds a channel identity to a conversation.
func (s *Service) AddParticipant(ctx context.Context, conversationID, channelIdentityID, role string) (Participant, error) {
	pgConversationID, err := parseUUID(conversationID)
	if err != nil {
		return Participant{}, err
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return Participant{}, err
	}
	if role == "" {
		role = RoleMember
	}
	row, err := s.queries.AddChatParticipant(ctx, sqlc.AddChatParticipantParams{
		ChatID: pgConversationID,
		UserID: pgChannelIdentityID,
		Role:   role,
	})
	if err != nil {
		return Participant{}, err
	}
	return toParticipantFromAdd(row), nil
}

// GetParticipant returns a conversation participant.
func (s *Service) GetParticipant(ctx context.Context, conversationID, channelIdentityID string) (Participant, error) {
	pgConversationID, err := parseUUID(conversationID)
	if err != nil {
		return Participant{}, ErrNotParticipant
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return Participant{}, ErrNotParticipant
	}
	row, err := s.queries.GetChatParticipant(ctx, sqlc.GetChatParticipantParams{
		ChatID: pgConversationID,
		UserID: pgChannelIdentityID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Participant{}, ErrNotParticipant
		}
		return Participant{}, err
	}
	return toParticipantFromGet(row), nil
}

// IsParticipant checks whether a channel identity is a participant.
func (s *Service) IsParticipant(ctx context.Context, conversationID, channelIdentityID string) (bool, error) {
	_, err := s.GetParticipant(ctx, conversationID, channelIdentityID)
	if errors.Is(err, ErrNotParticipant) {
		return false, nil
	}
	return err == nil, err
}

// ListParticipants returns all participants for a conversation.
func (s *Service) ListParticipants(ctx context.Context, conversationID string) ([]Participant, error) {
	pgID, err := parseUUID(conversationID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatParticipants(ctx, pgID)
	if err != nil {
		return nil, err
	}
	participants := make([]Participant, 0, len(rows))
	for _, row := range rows {
		participants = append(participants, toParticipantFromList(row))
	}
	return participants, nil
}

// RemoveParticipant removes a participant from a conversation.
func (s *Service) RemoveParticipant(ctx context.Context, conversationID, channelIdentityID string) error {
	pgConversationID, err := parseUUID(conversationID)
	if err != nil {
		return err
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return err
	}
	return s.queries.RemoveChatParticipant(ctx, sqlc.RemoveChatParticipantParams{
		ChatID: pgConversationID,
		UserID: pgChannelIdentityID,
	})
}

// GetSettings returns conversation settings and falls back to defaults when missing.
func (s *Service) GetSettings(ctx context.Context, conversationID string) (Settings, error) {
	pgID, err := parseUUID(conversationID)
	if err != nil {
		return Settings{}, fmt.Errorf("invalid conversation id: %w", err)
	}
	row, err := s.queries.GetChatSettings(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultSettings(conversationID), nil
		}
		return Settings{}, err
	}
	return toSettingsFromRead(row), nil
}

// UpdateSettings updates conversation settings.
func (s *Service) UpdateSettings(ctx context.Context, conversationID string, req UpdateSettingsRequest) (Settings, error) {
	current, err := s.GetSettings(ctx, conversationID)
	if err != nil {
		return Settings{}, err
	}
	if req.ModelID != nil {
		current.ModelID = *req.ModelID
	}

	pgID, err := parseUUID(conversationID)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.queries.UpsertChatSettings(ctx, sqlc.UpsertChatSettingsParams{
		ID:      pgID,
		ModelID: toPgText(current.ModelID),
	})
	if err != nil {
		return Settings{}, err
	}
	return toSettingsFromUpsert(row), nil
}

func toChatFromCreate(row sqlc.CreateChatRow) Conversation {
	return toChatFields(
		row.ID,
		row.BotID,
		row.Kind,
		row.ParentChatID,
		row.Title,
		row.CreatedByUserID,
		row.Metadata,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func toChatFromGet(row sqlc.GetChatByIDRow) Conversation {
	return toChatFields(
		row.ID,
		row.BotID,
		row.Kind,
		row.ParentChatID,
		row.Title,
		row.CreatedByUserID,
		row.Metadata,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func toChatFromThread(row sqlc.ListThreadsByParentRow) Conversation {
	return toChatFields(
		row.ID,
		row.BotID,
		row.Kind,
		row.ParentChatID,
		row.Title,
		row.CreatedByUserID,
		row.Metadata,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func toChatFields(id, botID pgtype.UUID, kind string, parentChatID pgtype.UUID, title pgtype.Text, createdBy pgtype.UUID, metadata []byte, createdAt, updatedAt pgtype.Timestamptz) Conversation {
	return Conversation{
		ID:           id.String(),
		BotID:        botID.String(),
		Kind:         kind,
		ParentChatID: parentChatID.String(),
		Title:        dbpkg.TextToString(title),
		CreatedBy:    createdBy.String(),
		Metadata:     parseJSONMap(metadata),
		CreatedAt:    createdAt.Time,
		UpdatedAt:    updatedAt.Time,
	}
}

func toChatListItem(row sqlc.ListVisibleChatsByBotAndUserRow) ConversationListItem {
	return ConversationListItem{
		ID:              row.ID.String(),
		BotID:           row.BotID.String(),
		Kind:            row.Kind,
		ParentChatID:    row.ParentChatID.String(),
		Title:           dbpkg.TextToString(row.Title),
		CreatedBy:       row.CreatedByUserID.String(),
		Metadata:        parseJSONMap(row.Metadata),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		AccessMode:      row.AccessMode,
		ParticipantRole: strings.TrimSpace(row.ParticipantRole),
		LastObservedAt:  pgTimePtr(row.LastObservedAt),
	}
}

func toParticipantFromAdd(row sqlc.AddChatParticipantRow) Participant {
	return toParticipantFields(row.ChatID, row.UserID, row.Role, row.JoinedAt)
}

func toParticipantFromGet(row sqlc.GetChatParticipantRow) Participant {
	return toParticipantFields(row.ChatID, row.UserID, row.Role, row.JoinedAt)
}

func toParticipantFromList(row sqlc.ListChatParticipantsRow) Participant {
	return toParticipantFields(row.ChatID, row.UserID, row.Role, row.JoinedAt)
}

func toParticipantFields(conversationID, userID pgtype.UUID, role string, joinedAt pgtype.Timestamptz) Participant {
	return Participant{
		ChatID:   conversationID.String(),
		UserID:   userID.String(),
		Role:     role,
		JoinedAt: joinedAt.Time,
	}
}

func toSettingsFromRead(row sqlc.GetChatSettingsRow) Settings {
	return Settings{
		ChatID:  row.ChatID.String(),
		ModelID: dbpkg.TextToString(row.ModelID),
	}
}

func toSettingsFromUpsert(row sqlc.UpsertChatSettingsRow) Settings {
	return Settings{
		ChatID:  row.ChatID.String(),
		ModelID: dbpkg.TextToString(row.ModelID),
	}
}

func defaultSettings(conversationID string) Settings {
	return Settings{
		ChatID: conversationID,
	}
}

func parseUUID(id string) (pgtype.UUID, error) {
	return dbpkg.ParseUUID(id)
}

func toPgText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	value := ts.Time
	return &value
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func parseJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("parseJSONMap: unmarshal failed", slog.Any("error", err))
	}
	return m
}
