package chat

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

	"github.com/memohai/memoh/internal/db/sqlc"
)

var (
	ErrChatNotFound     = errors.New("chat not found")
	ErrNotParticipant   = errors.New("not a participant")
	ErrPermissionDenied = errors.New("permission denied")
)

// Service manages chat lifecycle, participants, settings, and routes.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a chat service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "chat")),
	}
}

// --- Chat CRUD ---

// Create creates a new chat and adds the creator as owner.
func (s *Service) Create(ctx context.Context, botID, channelIdentityID string, req CreateChatRequest) (Chat, error) {
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = KindDirect
	}
	if kind != KindDirect && kind != KindGroup && kind != KindThread {
		return Chat{}, fmt.Errorf("invalid chat kind: %s", kind)
	}

	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Chat{}, fmt.Errorf("invalid bot id: %w", err)
	}
	pgChannelIdentityID := pgtype.UUID{}
	if strings.TrimSpace(channelIdentityID) != "" {
		pgChannelIdentityID, err = parseUUID(channelIdentityID)
		if err != nil {
			return Chat{}, fmt.Errorf("invalid user id: %w", err)
		}
	}

	var pgParent pgtype.UUID
	if kind == KindThread && strings.TrimSpace(req.ParentChatID) != "" {
		pgParent, err = parseUUID(req.ParentChatID)
		if err != nil {
			return Chat{}, fmt.Errorf("invalid parent chat id: %w", err)
		}
	}

	metadata, err := json.Marshal(nonNilMap(req.Metadata))
	if err != nil {
		return Chat{}, fmt.Errorf("marshal chat metadata: %w", err)
	}

	row, err := s.queries.CreateChat(ctx, sqlc.CreateChatParams{
		BotID:           pgBotID,
		Kind:            kind,
		ParentChatID:    pgParent,
		Title:           toPgText(req.Title),
		CreatedByUserID: pgChannelIdentityID,
		Metadata:        metadata,
	})
	if err != nil {
		return Chat{}, fmt.Errorf("create chat: %w", err)
	}

	// Add creator as owner when user identity is available.
	if pgChannelIdentityID.Valid {
		if _, err := s.queries.AddChatParticipant(ctx, sqlc.AddChatParticipantParams{
			ChatID: row.ID,
			UserID: pgChannelIdentityID,
			Role:   RoleOwner,
		}); err != nil {
			return Chat{}, fmt.Errorf("add owner participant: %w", err)
		}
	}

	// Create default settings based on kind.
	enablePrivate := kind != KindGroup
	if _, err := s.queries.UpsertChatSettings(ctx, sqlc.UpsertChatSettingsParams{
		ID:                  row.ID,
		EnableChatMemory:    true,
		EnablePrivateMemory: enablePrivate,
		EnablePublicMemory:  false,
		SettingsMetadata:    []byte("{}"),
	}); err != nil {
		return Chat{}, fmt.Errorf("create default settings: %w", err)
	}

	// For threads, copy participants from parent.
	if kind == KindThread && pgParent.Valid {
		if err := s.queries.CopyParticipantsToChat(ctx, sqlc.CopyParticipantsToChatParams{
			ChatID:   pgParent,
			ChatID_2: row.ID,
		}); err != nil {
			s.logger.Warn("copy parent participants failed", slog.Any("error", err))
		}
	}

	return toChat(row), nil
}

// Get returns a chat by ID.
func (s *Service) Get(ctx context.Context, chatID string) (Chat, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return Chat{}, ErrChatNotFound
	}
	row, err := s.queries.GetChatByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Chat{}, ErrChatNotFound
		}
		return Chat{}, err
	}
	return toChat(row), nil
}

// GetReadAccess resolves whether a user can read a chat.
func (s *Service) GetReadAccess(ctx context.Context, chatID, channelIdentityID string) (ChatReadAccess, error) {
	pgChatID, err := parseUUID(chatID)
	if err != nil {
		return ChatReadAccess{}, ErrPermissionDenied
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return ChatReadAccess{}, ErrPermissionDenied
	}
	row, err := s.queries.GetChatReadAccessByUser(ctx, sqlc.GetChatReadAccessByUserParams{
		ChatID: pgChatID,
		UserID: pgChannelIdentityID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChatReadAccess{}, ErrPermissionDenied
		}
		return ChatReadAccess{}, err
	}
	return ChatReadAccess{
		AccessMode:      row.AccessMode,
		ParticipantRole: strings.TrimSpace(row.ParticipantRole),
		LastObservedAt:  pgTimePtr(row.LastObservedAt),
	}, nil
}

// ListByBotAndChannelIdentity returns all chats visible to the user for a bot.
func (s *Service) ListByBotAndChannelIdentity(ctx context.Context, botID, channelIdentityID string) ([]ChatListItem, error) {
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
	chats := make([]ChatListItem, 0, len(rows))
	for _, row := range rows {
		chats = append(chats, toChatListItem(row))
	}
	return chats, nil
}

// ListThreads returns threads for a parent chat.
func (s *Service) ListThreads(ctx context.Context, parentChatID string) ([]Chat, error) {
	pgID, err := parseUUID(parentChatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListThreadsByParent(ctx, pgID)
	if err != nil {
		return nil, err
	}
	chats := make([]Chat, 0, len(rows))
	for _, row := range rows {
		chats = append(chats, toChat(row))
	}
	return chats, nil
}

// Delete deletes a chat (cascade deletes messages, routes, participants, settings).
func (s *Service) Delete(ctx context.Context, chatID string) error {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return ErrChatNotFound
	}
	return s.queries.DeleteChat(ctx, pgID)
}

// --- Participants ---

// AddParticipant adds a user identity to a chat.
func (s *Service) AddParticipant(ctx context.Context, chatID, channelIdentityID, role string) (Participant, error) {
	pgChatID, err := parseUUID(chatID)
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
		ChatID: pgChatID,
		UserID: pgChannelIdentityID,
		Role:   role,
	})
	if err != nil {
		return Participant{}, err
	}
	return toParticipant(row), nil
}

// GetParticipant returns a participant record.
func (s *Service) GetParticipant(ctx context.Context, chatID, channelIdentityID string) (Participant, error) {
	pgChatID, err := parseUUID(chatID)
	if err != nil {
		return Participant{}, ErrNotParticipant
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return Participant{}, ErrNotParticipant
	}
	row, err := s.queries.GetChatParticipant(ctx, sqlc.GetChatParticipantParams{
		ChatID: pgChatID,
		UserID: pgChannelIdentityID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Participant{}, ErrNotParticipant
		}
		return Participant{}, err
	}
	return toParticipant(row), nil
}

// IsParticipant checks whether a user identity is a participant in a chat.
func (s *Service) IsParticipant(ctx context.Context, chatID, channelIdentityID string) (bool, error) {
	_, err := s.GetParticipant(ctx, chatID, channelIdentityID)
	if errors.Is(err, ErrNotParticipant) {
		return false, nil
	}
	return err == nil, err
}

// ListParticipants returns all participants for a chat.
func (s *Service) ListParticipants(ctx context.Context, chatID string) ([]Participant, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatParticipants(ctx, pgID)
	if err != nil {
		return nil, err
	}
	participants := make([]Participant, 0, len(rows))
	for _, row := range rows {
		participants = append(participants, toParticipant(row))
	}
	return participants, nil
}

// RemoveParticipant removes a user identity from a chat.
func (s *Service) RemoveParticipant(ctx context.Context, chatID, channelIdentityID string) error {
	pgChatID, err := parseUUID(chatID)
	if err != nil {
		return err
	}
	pgChannelIdentityID, err := parseUUID(channelIdentityID)
	if err != nil {
		return err
	}
	return s.queries.RemoveChatParticipant(ctx, sqlc.RemoveChatParticipantParams{
		ChatID: pgChatID,
		UserID: pgChannelIdentityID,
	})
}

// --- Settings ---

// GetSettings returns settings for a chat. Returns defaults if not found.
func (s *Service) GetSettings(ctx context.Context, chatID string) (Settings, error) {
	pgID, err := parseUUID(chatID)
	var current Settings
	if err != nil {
		current = defaultSettings(chatID)
		return current, nil
	}
	row, err := s.queries.GetChatSettings(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current = defaultSettings(chatID)
			if s.isGroupChat(ctx, chatID) {
				current.EnablePrivateMemory = false
			}
			return current, nil
		}
		return Settings{}, err
	}
	current = toSettingsFromRead(row)
	if s.isGroupChat(ctx, chatID) {
		current.EnablePrivateMemory = false
	}
	return current, nil
}

// UpdateSettings updates chat settings.
func (s *Service) UpdateSettings(ctx context.Context, chatID string, req UpdateSettingsRequest) (Settings, error) {
	current, err := s.GetSettings(ctx, chatID)
	if err != nil {
		return Settings{}, err
	}
	isGroup := s.isGroupChat(ctx, chatID)
	if req.EnableChatMemory != nil {
		current.EnableChatMemory = *req.EnableChatMemory
	}
	if req.EnablePrivateMemory != nil {
		current.EnablePrivateMemory = *req.EnablePrivateMemory
	}
	if req.EnablePublicMemory != nil {
		current.EnablePublicMemory = *req.EnablePublicMemory
	}
	if req.ModelID != nil {
		current.ModelID = *req.ModelID
	}
	if isGroup {
		// Group chats are shared contexts, so private memory stays disabled.
		current.EnablePrivateMemory = false
	}

	pgID, err := parseUUID(chatID)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.queries.UpsertChatSettings(ctx, sqlc.UpsertChatSettingsParams{
		ID:                  pgID,
		EnableChatMemory:    current.EnableChatMemory,
		EnablePrivateMemory: current.EnablePrivateMemory,
		EnablePublicMemory:  current.EnablePublicMemory,
		ModelID:             toPgText(current.ModelID),
		SettingsMetadata:    []byte("{}"),
	})
	if err != nil {
		return Settings{}, err
	}
	return toSettingsFromUpsert(row), nil
}

// --- Routes ---

// CreateRoute creates a new chat route.
func (s *Service) CreateRoute(ctx context.Context, chatID string, r Route) (Route, error) {
	pgChatID, err := parseUUID(chatID)
	if err != nil {
		return Route{}, err
	}
	pgBotID, err := parseUUID(r.BotID)
	if err != nil {
		return Route{}, err
	}
	var pgConfigID pgtype.UUID
	if strings.TrimSpace(r.ChannelConfigID) != "" {
		pgConfigID, err = parseUUID(r.ChannelConfigID)
		if err != nil {
			return Route{}, err
		}
	}
	metadata, err := json.Marshal(nonNilMap(r.Metadata))
	if err != nil {
		return Route{}, fmt.Errorf("marshal route metadata: %w", err)
	}
	row, err := s.queries.CreateChatRoute(ctx, sqlc.CreateChatRouteParams{
		ChatID:          pgChatID,
		BotID:           pgBotID,
		Platform:        r.Platform,
		ChannelConfigID: pgConfigID,
		ConversationID:  r.ConversationID,
		ThreadID:        toPgText(r.ThreadID),
		ReplyTarget:     toPgText(r.ReplyTarget),
		Metadata:        metadata,
	})
	if err != nil {
		return Route{}, fmt.Errorf("create route: %w", err)
	}
	return toRoute(row), nil
}

// FindRoute looks up a route by (bot_id, platform, conversation_id, thread_id).
func (s *Service) FindRoute(ctx context.Context, botID, platform, conversationID, threadID string) (Route, error) {
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Route{}, err
	}
	row, err := s.queries.FindChatRoute(ctx, sqlc.FindChatRouteParams{
		BotID:          pgBotID,
		Platform:       platform,
		ConversationID: conversationID,
		ThreadID:       toPgText(threadID),
	})
	if err != nil {
		return Route{}, err
	}
	return toRoute(row), nil
}

// GetRouteByID returns a single route by its ID.
func (s *Service) GetRouteByID(ctx context.Context, routeID string) (Route, error) {
	pgID, err := parseUUID(routeID)
	if err != nil {
		return Route{}, err
	}
	row, err := s.queries.GetChatRouteByID(ctx, pgID)
	if err != nil {
		return Route{}, err
	}
	return toRoute(row), nil
}

// ListRoutes lists all routes for a chat.
func (s *Service) ListRoutes(ctx context.Context, chatID string) ([]Route, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatRoutes(ctx, pgID)
	if err != nil {
		return nil, err
	}
	routes := make([]Route, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, toRoute(row))
	}
	return routes, nil
}

// DeleteRoute deletes a route.
func (s *Service) DeleteRoute(ctx context.Context, routeID string) error {
	pgID, err := parseUUID(routeID)
	if err != nil {
		return err
	}
	return s.queries.DeleteChatRoute(ctx, pgID)
}

// UpdateRouteReplyTarget updates the reply target for a route.
func (s *Service) UpdateRouteReplyTarget(ctx context.Context, routeID, replyTarget string) error {
	pgID, err := parseUUID(routeID)
	if err != nil {
		return err
	}
	return s.queries.UpdateChatRouteReplyTarget(ctx, sqlc.UpdateChatRouteReplyTargetParams{
		ID:          pgID,
		ReplyTarget: toPgText(replyTarget),
	})
}

// --- ResolveChat ---

// ResolveChat finds or creates a chat for a channel inbound message.
func (s *Service) ResolveChat(ctx context.Context, botID, platform, conversationID, threadID, conversationType, channelIdentityID, channelConfigID, replyTarget string) (ResolveChatResult, error) {
	// Look up existing route.
	route, err := s.FindRoute(ctx, botID, platform, conversationID, threadID)
	if err == nil {
		// Route found, ensure the sender identity is a participant.
		if strings.TrimSpace(channelIdentityID) != "" {
			ok, checkErr := s.IsParticipant(ctx, route.ChatID, channelIdentityID)
			if checkErr != nil {
				return ResolveChatResult{}, fmt.Errorf("check chat participant: %w", checkErr)
			}
			if !ok {
				if _, err := s.AddParticipant(ctx, route.ChatID, channelIdentityID, RoleMember); err != nil {
					s.logger.Warn("auto-add participant failed", slog.Any("error", err))
				}
			}
		}
		// Update reply target if changed.
		if strings.TrimSpace(replyTarget) != "" && replyTarget != route.ReplyTarget {
			if err := s.UpdateRouteReplyTarget(ctx, route.ID, replyTarget); err != nil && s.logger != nil {
				s.logger.Warn("update route reply target failed", slog.Any("error", err))
			}
		}
		pgRouteChatID, parseErr := parseUUID(route.ChatID)
		if parseErr != nil {
			return ResolveChatResult{}, fmt.Errorf("parse route chat id: %w", parseErr)
		}
		if err := s.queries.TouchChat(ctx, pgRouteChatID); err != nil && s.logger != nil {
			s.logger.Warn("touch chat failed", slog.Any("error", err))
		}
		return ResolveChatResult{ChatID: route.ChatID, RouteID: route.ID, Created: false}, nil
	}

	// Route not found, create chat + route + participant.
	kind := determineChatKind(threadID, conversationType)
	creatorChannelIdentityID := s.resolveChatCreatorChannelIdentityID(ctx, botID, channelIdentityID, kind)

	var parentChatID string
	if kind == KindThread {
		parentRoute, parentErr := s.FindRoute(ctx, botID, platform, conversationID, "")
		if parentErr == nil {
			parentChatID = parentRoute.ChatID
		}
	}

	c, err := s.Create(ctx, botID, creatorChannelIdentityID, CreateChatRequest{
		Kind:         kind,
		ParentChatID: parentChatID,
	})
	if err != nil {
		return ResolveChatResult{}, fmt.Errorf("create chat: %w", err)
	}
	if strings.TrimSpace(channelIdentityID) != "" && strings.TrimSpace(channelIdentityID) != strings.TrimSpace(creatorChannelIdentityID) {
		if _, err := s.AddParticipant(ctx, c.ID, channelIdentityID, RoleMember); err != nil {
			s.logger.Warn("auto-add creator participant failed", slog.Any("error", err))
		}
	}

	newRoute, err := s.CreateRoute(ctx, c.ID, Route{
		BotID:           botID,
		Platform:        platform,
		ChannelConfigID: channelConfigID,
		ConversationID:  conversationID,
		ThreadID:        threadID,
		ReplyTarget:     replyTarget,
	})
	if err != nil {
		return ResolveChatResult{}, fmt.Errorf("create route: %w", err)
	}

	return ResolveChatResult{ChatID: c.ID, RouteID: newRoute.ID, Created: true}, nil
}

// --- Messages ---

// PersistMessage writes a single message to chat_messages.
func (s *Service) PersistMessage(ctx context.Context, chatID, botID, routeID, senderChannelIdentityID, senderUserID, platform, externalMessageID, role string, content json.RawMessage, metadata map[string]any) (Message, error) {
	pgChatID, err := parseUUID(chatID)
	if err != nil {
		return Message{}, err
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return Message{}, err
	}
	var pgRouteID pgtype.UUID
	if strings.TrimSpace(routeID) != "" {
		pgRouteID, err = parseUUID(routeID)
		if err != nil {
			return Message{}, err
		}
	}
	var pgSender pgtype.UUID
	if strings.TrimSpace(senderChannelIdentityID) != "" {
		pgSender, err = parseUUID(senderChannelIdentityID)
		if err != nil {
			return Message{}, fmt.Errorf("invalid sender channel identity id: %w", err)
		}
	}
	var pgSenderUser pgtype.UUID
	if strings.TrimSpace(senderUserID) != "" {
		pgSenderUser, err = parseUUID(senderUserID)
		if err != nil {
			return Message{}, fmt.Errorf("invalid sender user id: %w", err)
		}
	}
	metaBytes, err := json.Marshal(nonNilMap(metadata))
	if err != nil {
		return Message{}, fmt.Errorf("marshal message metadata: %w", err)
	}
	if len(content) == 0 {
		content = []byte("{}")
	}

	row, err := s.queries.CreateChatMessage(ctx, sqlc.CreateChatMessageParams{
		ChatID:                  pgChatID,
		BotID:                   pgBotID,
		RouteID:                 pgRouteID,
		SenderChannelIdentityID: pgSender,
		SenderUserID:            pgSenderUser,
		Platform:                toPgText(platform),
		ExternalMessageID:       toPgText(externalMessageID),
		Role:                    role,
		Content:                 content,
		Metadata:                metaBytes,
	})
	if err != nil {
		return Message{}, err
	}
	if pgSender.Valid {
		if err := s.queries.UpsertChatChannelIdentityPresence(ctx, sqlc.UpsertChatChannelIdentityPresenceParams{
			ChatID:            pgChatID,
			ChannelIdentityID: pgSender,
		}); err != nil && s.logger != nil {
			// Presence is a derived cache. Keep message persistence successful even if cache update fails.
			s.logger.Warn("upsert chat channel identity presence failed", slog.Any("error", err))
		}
	}
	return toMessage(row), nil
}

// ListMessages returns all messages for a chat.
func (s *Service) ListMessages(ctx context.Context, chatID string) ([]Message, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatMessages(ctx, pgID)
	if err != nil {
		return nil, err
	}
	return toMessages(rows), nil
}

// ListMessagesSince returns messages since a given time.
func (s *Service) ListMessagesSince(ctx context.Context, chatID string, since time.Time) ([]Message, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatMessagesSince(ctx, sqlc.ListChatMessagesSinceParams{
		ChatID:    pgID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return toMessages(rows), nil
}

// ListMessagesLatest returns the latest N messages (most recent first).
func (s *Service) ListMessagesLatest(ctx context.Context, chatID string, limit int32) ([]Message, error) {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatMessagesLatest(ctx, sqlc.ListChatMessagesLatestParams{
		ChatID: pgID,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	return toMessages(rows), nil
}

// DeleteMessages deletes all messages for a chat.
func (s *Service) DeleteMessages(ctx context.Context, chatID string) error {
	pgID, err := parseUUID(chatID)
	if err != nil {
		return err
	}
	return s.queries.DeleteChatMessagesByChat(ctx, pgID)
}

// --- conversion helpers ---

func toChat(row sqlc.Chat) Chat {
	return Chat{
		ID:           uuidString(row.ID),
		BotID:        uuidString(row.BotID),
		Kind:         row.Kind,
		ParentChatID: uuidString(row.ParentChatID),
		Title:        pgTextString(row.Title),
		CreatedBy:    uuidString(row.CreatedByUserID),
		Metadata:     parseJSONMap(row.Metadata),
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toChatListItem(row sqlc.ListVisibleChatsByBotAndUserRow) ChatListItem {
	return ChatListItem{
		ID:              uuidString(row.ID),
		BotID:           uuidString(row.BotID),
		Kind:            row.Kind,
		ParentChatID:    uuidString(row.ParentChatID),
		Title:           pgTextString(row.Title),
		CreatedBy:       uuidString(row.CreatedByUserID),
		Metadata:        parseJSONMap(row.Metadata),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		AccessMode:      row.AccessMode,
		ParticipantRole: strings.TrimSpace(row.ParticipantRole),
		LastObservedAt:  pgTimePtr(row.LastObservedAt),
	}
}

func toParticipant(row sqlc.ChatParticipant) Participant {
	return Participant{
		ChatID:   uuidString(row.ChatID),
		UserID:   uuidString(row.UserID),
		Role:     row.Role,
		JoinedAt: row.JoinedAt.Time,
	}
}

func toSettingsFromRead(row sqlc.GetChatSettingsRow) Settings {
	return Settings{
		ChatID:              uuidString(row.ChatID),
		EnableChatMemory:    row.EnableChatMemory,
		EnablePrivateMemory: row.EnablePrivateMemory,
		EnablePublicMemory:  row.EnablePublicMemory,
		ModelID:             pgTextString(row.ModelID),
		Metadata:            parseJSONMap(row.Metadata),
	}
}

func toSettingsFromUpsert(row sqlc.UpsertChatSettingsRow) Settings {
	return Settings{
		ChatID:              uuidString(row.ChatID),
		EnableChatMemory:    row.EnableChatMemory,
		EnablePrivateMemory: row.EnablePrivateMemory,
		EnablePublicMemory:  row.EnablePublicMemory,
		ModelID:             pgTextString(row.ModelID),
		Metadata:            parseJSONMap(row.Metadata),
	}
}

func toRoute(row sqlc.ChatRoute) Route {
	return Route{
		ID:              uuidString(row.ID),
		ChatID:          uuidString(row.ChatID),
		BotID:           uuidString(row.BotID),
		Platform:        row.Platform,
		ChannelConfigID: uuidString(row.ChannelConfigID),
		ConversationID:  row.ConversationID,
		ThreadID:        pgTextString(row.ThreadID),
		ReplyTarget:     pgTextString(row.ReplyTarget),
		Metadata:        parseJSONMap(row.Metadata),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toMessage(row sqlc.ChatMessage) Message {
	return Message{
		ID:                      uuidString(row.ID),
		ChatID:                  uuidString(row.ChatID),
		BotID:                   uuidString(row.BotID),
		RouteID:                 uuidString(row.RouteID),
		SenderChannelIdentityID: uuidString(row.SenderChannelIdentityID),
		SenderUserID:            uuidString(row.SenderUserID),
		Platform:                pgTextString(row.Platform),
		ExternalMessageID:       pgTextString(row.ExternalMessageID),
		Role:                    row.Role,
		Content:                 json.RawMessage(row.Content),
		Metadata:                parseJSONMap(row.Metadata),
		CreatedAt:               row.CreatedAt.Time,
	}
}

func toMessages(rows []sqlc.ChatMessage) []Message {
	msgs := make([]Message, 0, len(rows))
	for _, row := range rows {
		msgs = append(msgs, toMessage(row))
	}
	return msgs
}

func defaultSettings(chatID string) Settings {
	return Settings{
		ChatID:              chatID,
		EnableChatMemory:    true,
		EnablePrivateMemory: true,
		EnablePublicMemory:  false,
	}
}

func determineChatKind(threadID, conversationType string) string {
	if strings.TrimSpace(threadID) != "" {
		return KindThread
	}
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	if ct == "p2p" || ct == "private" || ct == "" {
		return KindDirect
	}
	return KindGroup
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func pgTextString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
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
	_ = json.Unmarshal(data, &m)
	return m
}

func (s *Service) resolveChatCreatorChannelIdentityID(ctx context.Context, botID, fallbackChannelIdentityID, kind string) string {
	fallback := strings.TrimSpace(fallbackChannelIdentityID)
	if kind != KindGroup || s.queries == nil {
		return fallback
	}
	pgBotID, err := parseUUID(botID)
	if err != nil {
		return fallback
	}
	row, err := s.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		s.logger.Warn("resolve bot owner for group chat failed", slog.Any("error", err))
		return fallback
	}
	ownerChannelIdentityID := uuidString(row.OwnerUserID)
	if strings.TrimSpace(ownerChannelIdentityID) == "" {
		return fallback
	}
	return ownerChannelIdentityID
}

func (s *Service) isGroupChat(ctx context.Context, chatID string) bool {
	chatObj, err := s.Get(ctx, chatID)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(chatObj.Kind), KindGroup)
}
