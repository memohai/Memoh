package route

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// ConversationService contains the minimal conversation behavior required by route resolution.
type ConversationService interface {
	Create(ctx context.Context, botID, channelIdentityID string, req conversation.CreateRequest) (conversation.Conversation, error)
	IsParticipant(ctx context.Context, conversationID, channelIdentityID string) (bool, error)
	AddParticipant(ctx context.Context, conversationID, channelIdentityID, role string) (conversation.Participant, error)
}

// DBService manages channel routes and route-to-conversation resolution.
type DBService struct {
	queries      *sqlc.Queries
	conversation ConversationService
	logger       *slog.Logger
}

// NewService creates a channel route service.
func NewService(log *slog.Logger, queries *sqlc.Queries, conversationService ConversationService) *DBService {
	if log == nil {
		log = slog.Default()
	}
	return &DBService{
		queries:      queries,
		conversation: conversationService,
		logger:       log.With(slog.String("service", "channel/route")),
	}
}

// Create creates a route.
func (s *DBService) Create(ctx context.Context, input CreateInput) (Route, error) {
	pgConversationID, err := dbpkg.ParseUUID(input.ChatID)
	if err != nil {
		return Route{}, err
	}
	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return Route{}, err
	}
	var pgConfigID pgtype.UUID
	if strings.TrimSpace(input.ChannelConfigID) != "" {
		pgConfigID, err = dbpkg.ParseUUID(input.ChannelConfigID)
		if err != nil {
			return Route{}, err
		}
	}
	metadata, err := json.Marshal(nonNilMap(input.Metadata))
	if err != nil {
		return Route{}, fmt.Errorf("marshal route metadata: %w", err)
	}

	row, err := s.queries.CreateChatRoute(ctx, sqlc.CreateChatRouteParams{
		ChatID:           pgConversationID,
		BotID:            pgBotID,
		Platform:         input.Platform,
		ChannelConfigID:  pgConfigID,
		ConversationID:   input.ConversationID,
		ThreadID:         toPgText(input.ThreadID),
		ConversationType: toPgText(input.ConversationType),
		ReplyTarget:      toPgText(input.ReplyTarget),
		Metadata:         metadata,
	})
	if err != nil {
		return Route{}, fmt.Errorf("create route: %w", err)
	}

	return toRouteFromCreate(row), nil
}

// Find finds a route by bot/platform/external-conversation/thread.
func (s *DBService) Find(ctx context.Context, botID, platform, conversationID, threadID string) (Route, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
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
	return toRouteFromFind(row), nil
}

// GetByID gets a route by ID.
func (s *DBService) GetByID(ctx context.Context, routeID string) (Route, error) {
	pgID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return Route{}, err
	}
	row, err := s.queries.GetChatRouteByID(ctx, pgID)
	if err != nil {
		return Route{}, err
	}
	return toRouteFromGet(row), nil
}

// List lists all routes for a conversation.
func (s *DBService) List(ctx context.Context, conversationID string) ([]Route, error) {
	pgID, err := dbpkg.ParseUUID(conversationID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListChatRoutes(ctx, pgID)
	if err != nil {
		return nil, err
	}
	routes := make([]Route, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, toRouteFromList(row))
	}
	return routes, nil
}

// Delete deletes a route by ID.
func (s *DBService) Delete(ctx context.Context, routeID string) error {
	pgID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return err
	}
	return s.queries.DeleteChatRoute(ctx, pgID)
}

// UpdateReplyTarget updates default reply target.
func (s *DBService) UpdateReplyTarget(ctx context.Context, routeID, replyTarget string) error {
	pgID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return err
	}
	return s.queries.UpdateChatRouteReplyTarget(ctx, sqlc.UpdateChatRouteReplyTargetParams{
		ID:          pgID,
		ReplyTarget: toPgText(replyTarget),
	})
}

// ResolveConversation finds or creates a conversation route for an inbound message.
func (s *DBService) ResolveConversation(ctx context.Context, input ResolveInput) (ResolveConversationResult, error) {
	route, err := s.Find(ctx, input.BotID, input.Platform, input.ConversationID, input.ThreadID)
	if err == nil {
		if strings.TrimSpace(input.ChannelIdentityID) != "" && s.conversation != nil {
			ok, checkErr := s.conversation.IsParticipant(ctx, route.ChatID, input.ChannelIdentityID)
			if checkErr != nil {
				return ResolveConversationResult{}, fmt.Errorf("check conversation participant: %w", checkErr)
			}
			if !ok {
				if _, addErr := s.conversation.AddParticipant(ctx, route.ChatID, input.ChannelIdentityID, conversation.RoleMember); addErr != nil && s.logger != nil {
					s.logger.Warn("auto-add participant failed", slog.Any("error", addErr))
				}
			}
		}
		if strings.TrimSpace(input.ReplyTarget) != "" && input.ReplyTarget != route.ReplyTarget {
			if updateErr := s.UpdateReplyTarget(ctx, route.ID, input.ReplyTarget); updateErr != nil && s.logger != nil {
				s.logger.Warn("update route reply target failed", slog.Any("error", updateErr))
			}
		}
		pgConversationID, parseErr := dbpkg.ParseUUID(route.ChatID)
		if parseErr != nil {
			return ResolveConversationResult{}, fmt.Errorf("parse route conversation id: %w", parseErr)
		}
		if touchErr := s.queries.TouchChat(ctx, pgConversationID); touchErr != nil && s.logger != nil {
			s.logger.Warn("touch conversation failed", slog.Any("error", touchErr))
		}
		return ResolveConversationResult{ChatID: route.ChatID, RouteID: route.ID, Created: false}, nil
	}

	if s.conversation == nil {
		return ResolveConversationResult{}, fmt.Errorf("conversation service not configured")
	}

	kind := determineConversationKind(input.ThreadID, input.ConversationType)
	creatorChannelIdentityID := s.resolveConversationCreatorChannelIdentityID(ctx, input.BotID, input.ChannelIdentityID, kind)

	var parentConversationID string
	if kind == conversation.KindThread {
		parentRoute, parentErr := s.Find(ctx, input.BotID, input.Platform, input.ConversationID, "")
		if parentErr == nil {
			parentConversationID = parentRoute.ChatID
		}
	}

	createdConversation, err := s.conversation.Create(ctx, input.BotID, creatorChannelIdentityID, conversation.CreateRequest{
		Kind:         kind,
		ParentChatID: parentConversationID,
	})
	if err != nil {
		return ResolveConversationResult{}, fmt.Errorf("create conversation: %w", err)
	}

	if strings.TrimSpace(input.ChannelIdentityID) != "" && strings.TrimSpace(input.ChannelIdentityID) != strings.TrimSpace(creatorChannelIdentityID) {
		if _, addErr := s.conversation.AddParticipant(ctx, createdConversation.ID, input.ChannelIdentityID, conversation.RoleMember); addErr != nil && s.logger != nil {
			s.logger.Warn("auto-add creator participant failed", slog.Any("error", addErr))
		}
	}

	newRoute, err := s.Create(ctx, CreateInput{
		ChatID:           createdConversation.ID,
		BotID:            input.BotID,
		Platform:         input.Platform,
		ChannelConfigID:  input.ChannelConfigID,
		ConversationID:   input.ConversationID,
		ThreadID:         input.ThreadID,
		ConversationType: input.ConversationType,
		ReplyTarget:      input.ReplyTarget,
	})
	if err != nil {
		return ResolveConversationResult{}, fmt.Errorf("create route: %w", err)
	}

	return ResolveConversationResult{ChatID: createdConversation.ID, RouteID: newRoute.ID, Created: true}, nil
}

func determineConversationKind(threadID, conversationType string) string {
	if strings.TrimSpace(threadID) != "" {
		return conversation.KindThread
	}
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	if ct == "p2p" || ct == "private" || ct == "" {
		return conversation.KindDirect
	}
	return conversation.KindGroup
}

func (s *DBService) resolveConversationCreatorChannelIdentityID(ctx context.Context, botID, fallbackChannelIdentityID, kind string) string {
	fallback := strings.TrimSpace(fallbackChannelIdentityID)
	if kind != conversation.KindGroup || s.queries == nil {
		return fallback
	}
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return fallback
	}
	row, err := s.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("resolve bot owner for group conversation failed", slog.Any("error", err))
		}
		return fallback
	}
	// NOTE: OwnerUserID is the bot owner's user ID. Used as fallback creator for group conversations.
	ownerUserID := row.OwnerUserID.String()
	if strings.TrimSpace(ownerUserID) == "" {
		return fallback
	}
	return ownerUserID
}

func toRouteFromCreate(row sqlc.CreateChatRouteRow) Route {
	return toRouteFields(
		row.ID, row.ChatID, row.BotID, row.Platform, row.ChannelConfigID,
		row.ConversationID, row.ThreadID, row.ConversationType, row.ReplyTarget,
		row.Metadata, row.CreatedAt, row.UpdatedAt,
	)
}

func toRouteFromFind(row sqlc.FindChatRouteRow) Route {
	return toRouteFields(
		row.ID, row.ChatID, row.BotID, row.Platform, row.ChannelConfigID,
		row.ConversationID, row.ThreadID, row.ConversationType, row.ReplyTarget,
		row.Metadata, row.CreatedAt, row.UpdatedAt,
	)
}

func toRouteFromGet(row sqlc.GetChatRouteByIDRow) Route {
	return toRouteFields(
		row.ID, row.ChatID, row.BotID, row.Platform, row.ChannelConfigID,
		row.ConversationID, row.ThreadID, row.ConversationType, row.ReplyTarget,
		row.Metadata, row.CreatedAt, row.UpdatedAt,
	)
}

func toRouteFromList(row sqlc.ListChatRoutesRow) Route {
	return toRouteFields(
		row.ID, row.ChatID, row.BotID, row.Platform, row.ChannelConfigID,
		row.ConversationID, row.ThreadID, row.ConversationType, row.ReplyTarget,
		row.Metadata, row.CreatedAt, row.UpdatedAt,
	)
}

func toRouteFields(id, conversationID, botID pgtype.UUID, platform string, channelConfigID pgtype.UUID, externalConversationID string, threadID, conversationType, replyTarget pgtype.Text, metadata []byte, createdAt, updatedAt pgtype.Timestamptz) Route {
	return Route{
		ID:               id.String(),
		ChatID:           conversationID.String(),
		BotID:            botID.String(),
		Platform:         platform,
		ChannelConfigID:  channelConfigID.String(),
		ConversationID:   externalConversationID,
		ThreadID:         dbpkg.TextToString(threadID),
		ConversationType: dbpkg.TextToString(conversationType),
		ReplyTarget:      dbpkg.TextToString(replyTarget),
		Metadata:         parseJSONMap(metadata),
		CreatedAt:        createdAt.Time,
		UpdatedAt:        updatedAt.Time,
	}
}

func toPgText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
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
