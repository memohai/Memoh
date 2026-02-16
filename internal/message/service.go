package message

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/message/event"
)

// DBService persists and reads bot history messages.
type DBService struct {
	queries   *sqlc.Queries
	logger    *slog.Logger
	publisher event.Publisher
}

// NewService creates a message service.
func NewService(log *slog.Logger, queries *sqlc.Queries, publishers ...event.Publisher) *DBService {
	if log == nil {
		log = slog.Default()
	}
	var publisher event.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &DBService{
		queries:   queries,
		logger:    log.With(slog.String("service", "message")),
		publisher: publisher,
	}
}

// Persist writes a single message to bot_history_messages.
func (s *DBService) Persist(ctx context.Context, input PersistInput) (Message, error) {
	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return Message{}, fmt.Errorf("invalid bot id: %w", err)
	}

	pgRouteID, err := parseOptionalUUID(input.RouteID)
	if err != nil {
		return Message{}, fmt.Errorf("invalid route id: %w", err)
	}
	pgSenderChannelIdentityID, err := parseOptionalUUID(input.SenderChannelIdentityID)
	if err != nil {
		return Message{}, fmt.Errorf("invalid sender channel identity id: %w", err)
	}
	pgSenderUserID, err := parseOptionalUUID(input.SenderUserID)
	if err != nil {
		return Message{}, fmt.Errorf("invalid sender user id: %w", err)
	}

	metaBytes, err := json.Marshal(nonNilMap(input.Metadata))
	if err != nil {
		return Message{}, fmt.Errorf("marshal message metadata: %w", err)
	}

	content := input.Content
	if len(content) == 0 {
		content = []byte("{}")
	}

	row, err := s.queries.CreateMessage(ctx, sqlc.CreateMessageParams{
		BotID:                   pgBotID,
		RouteID:                 pgRouteID,
		SenderChannelIdentityID: pgSenderChannelIdentityID,
		SenderUserID:            pgSenderUserID,
		Platform:                toPgText(input.Platform),
		ExternalMessageID:       toPgText(input.ExternalMessageID),
		SourceReplyToMessageID:  toPgText(input.SourceReplyToMessageID),
		Role:                    input.Role,
		Content:                 content,
		Metadata:                metaBytes,
	})
	if err != nil {
		return Message{}, err
	}

	result := toMessageFromCreate(row)

	// Persist asset links if provided.
	for _, ref := range input.Assets {
		pgMsgID := row.ID
		pgAssetID, assetErr := dbpkg.ParseUUID(ref.AssetID)
		if assetErr != nil {
			s.logger.Warn("skip invalid asset ref", slog.String("asset_id", ref.AssetID), slog.Any("error", assetErr))
			continue
		}
		role := ref.Role
		if strings.TrimSpace(role) == "" {
			role = "attachment"
		}
		if _, assetErr := s.queries.CreateMessageAsset(ctx, sqlc.CreateMessageAssetParams{
			MessageID: pgMsgID,
			AssetID:   pgAssetID,
			Role:      role,
			Ordinal:   int32(ref.Ordinal),
		}); assetErr != nil {
			s.logger.Warn("create message asset link failed", slog.String("message_id", result.ID), slog.Any("error", assetErr))
		}
	}

	// Enrich assets before publishing so SSE consumers see them immediately.
	if len(input.Assets) > 0 {
		enriched := []Message{result}
		s.enrichAssets(ctx, enriched)
		result = enriched[0]
	}

	s.publishMessageCreated(result)
	return result, nil
}

// List returns all messages for a bot.
func (s *DBService) List(ctx context.Context, botID string) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessages(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromList(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListSince returns bot messages since a given time.
func (s *DBService) ListSince(ctx context.Context, botID string, since time.Time) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesSince(ctx, sqlc.ListMessagesSinceParams{
		BotID:     pgBotID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromSince(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListLatest returns the latest N bot messages (newest first in DB; caller may reverse for ASC).
func (s *DBService) ListLatest(ctx context.Context, botID string, limit int32) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesLatest(ctx, sqlc.ListMessagesLatestParams{
		BotID:    pgBotID,
		MaxCount: limit,
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromLatest(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListBefore returns up to limit messages older than before (created_at < before), ordered oldest-first.
func (s *DBService) ListBefore(ctx context.Context, botID string, before time.Time, limit int32) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesBefore(ctx, sqlc.ListMessagesBeforeParams{
		BotID:     pgBotID,
		CreatedAt: pgtype.Timestamptz{Time: before, Valid: true},
		MaxCount:  limit,
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromBefore(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// DeleteByBot deletes all messages for a bot.
func (s *DBService) DeleteByBot(ctx context.Context, botID string) error {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteMessagesByBot(ctx, pgBotID)
}

func toMessageFromCreate(row sqlc.CreateMessageRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.RouteID,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		pgtype.Text{},
		pgtype.Text{},
		row.Platform,
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.CreatedAt,
	)
}

func toMessageFromListRow(row sqlc.ListMessagesRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.RouteID,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		row.SenderDisplayName,
		row.SenderAvatarUrl,
		row.Platform,
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.CreatedAt,
	)
}

func toMessageFromSinceRow(row sqlc.ListMessagesSinceRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.RouteID,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		row.SenderDisplayName,
		row.SenderAvatarUrl,
		row.Platform,
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.CreatedAt,
	)
}

func toMessageFromLatestRow(row sqlc.ListMessagesLatestRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.RouteID,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		row.SenderDisplayName,
		row.SenderAvatarUrl,
		row.Platform,
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.CreatedAt,
	)
}

func toMessageFields(
	id pgtype.UUID,
	botID pgtype.UUID,
	routeID pgtype.UUID,
	senderChannelIdentityID pgtype.UUID,
	senderUserID pgtype.UUID,
	senderDisplayName pgtype.Text,
	senderAvatarURL pgtype.Text,
	platform pgtype.Text,
	externalMessageID pgtype.Text,
	sourceReplyToMessageID pgtype.Text,
	role string,
	content []byte,
	metadata []byte,
	createdAt pgtype.Timestamptz,
) Message {
	return Message{
		ID:                      id.String(),
		BotID:                   botID.String(),
		RouteID:                 routeID.String(),
		SenderChannelIdentityID: senderChannelIdentityID.String(),
		SenderUserID:            senderUserID.String(),
		SenderDisplayName:       dbpkg.TextToString(senderDisplayName),
		SenderAvatarURL:         dbpkg.TextToString(senderAvatarURL),
		Platform:                dbpkg.TextToString(platform),
		ExternalMessageID:       dbpkg.TextToString(externalMessageID),
		SourceReplyToMessageID:  dbpkg.TextToString(sourceReplyToMessageID),
		Role:                    role,
		Content:                 json.RawMessage(content),
		Metadata:                parseJSONMap(metadata),
		CreatedAt:               createdAt.Time,
	}
}

func toMessagesFromList(rows []sqlc.ListMessagesRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromListRow(row))
	}
	return messages
}

func toMessagesFromSince(rows []sqlc.ListMessagesSinceRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromSinceRow(row))
	}
	return messages
}

func toMessagesFromLatest(rows []sqlc.ListMessagesLatestRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromLatestRow(row))
	}
	return messages
}

func toMessageFromBeforeRow(row sqlc.ListMessagesBeforeRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.RouteID,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		row.SenderDisplayName,
		row.SenderAvatarUrl,
		row.Platform,
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.CreatedAt,
	)
}

// toMessagesFromBefore returns messages in oldest-first order (ListMessagesBefore returns DESC; we reverse).
func toMessagesFromBefore(rows []sqlc.ListMessagesBeforeRow) []Message {
	messages := make([]Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		messages = append(messages, toMessageFromBeforeRow(rows[i]))
	}
	return messages
}

func parseOptionalUUID(id string) (pgtype.UUID, error) {
	if strings.TrimSpace(id) == "" {
		return pgtype.UUID{}, nil
	}
	return dbpkg.ParseUUID(id)
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

func (s *DBService) publishMessageCreated(message Message) {
	if s.publisher == nil {
		return
	}
	payload, err := json.Marshal(message)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("marshal message event failed", slog.Any("error", err))
		}
		return
	}
	s.publisher.Publish(event.Event{
		Type:  event.EventTypeMessageCreated,
		BotID: strings.TrimSpace(message.BotID),
		Data:  payload,
	})
}

// enrichAssets batch-loads asset links for a list of messages.
func (s *DBService) enrichAssets(ctx context.Context, messages []Message) {
	if len(messages) == 0 {
		return
	}
	ids := make([]pgtype.UUID, 0, len(messages))
	for _, m := range messages {
		pgID, err := dbpkg.ParseUUID(m.ID)
		if err != nil {
			continue
		}
		ids = append(ids, pgID)
	}
	if len(ids) == 0 {
		return
	}
	rows, err := s.queries.ListMessageAssetsBatch(ctx, ids)
	if err != nil {
		s.logger.Warn("enrich assets failed", slog.Any("error", err))
		return
	}
	assetMap := map[string][]MessageAsset{}
	for _, row := range rows {
		msgID := row.MessageID.String()
		assetMap[msgID] = append(assetMap[msgID], MessageAsset{
			AssetID:      row.AssetID.String(),
			Role:         row.Role,
			Ordinal:      int(row.Ordinal),
			MediaType:    row.MediaType,
			Mime:         row.Mime,
			SizeBytes:    row.SizeBytes,
			StorageKey:   row.StorageKey,
			OriginalName: dbpkg.TextToString(row.OriginalName),
			Width:        int(row.Width.Int32),
			Height:       int(row.Height.Int32),
			DurationMs:   row.DurationMs.Int64,
		})
	}
	for i := range messages {
		if assets, ok := assetMap[messages[i].ID]; ok {
			messages[i].Assets = assets
		}
	}
}
