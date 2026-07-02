package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/message/event"
)

// DBService persists and reads bot history messages.
type DBService struct {
	queries   dbstore.Queries
	logger    *slog.Logger
	publisher event.Publisher
}

type txRunner interface {
	RunInTx(ctx context.Context, fn func(dbstore.Queries) error) error
}

// NewService creates a message service.
func NewService(log *slog.Logger, queries dbstore.Queries, publishers ...event.Publisher) *DBService {
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
	prepared, err := prepareCreateMessageInput(input)
	if err != nil {
		return Message{}, err
	}

	result, err := s.persistMessageAtomic(ctx, prepared, input.Assets)
	if err != nil {
		return Message{}, err
	}
	attachPersistedAssetRefs(&result, input.Assets)

	s.PublishMessageCreated(result)
	return result, nil
}

// PersistWithQueries writes a single message using a caller-provided query
// handle. The caller owns the surrounding transaction and event publication.
func (*DBService) PersistWithQueries(ctx context.Context, queries dbstore.Queries, input PersistInput) (Message, error) {
	if queries == nil {
		return Message{}, errors.New("persist with queries: queries is required")
	}
	prepared, err := prepareCreateMessageInput(input)
	if err != nil {
		return Message{}, err
	}
	result, err := persistMessageWithQueries(ctx, queries, prepared, input.Assets)
	if err != nil {
		return Message{}, err
	}
	attachPersistedAssetRefs(&result, input.Assets)
	return result, nil
}

func prepareCreateMessageInput(input PersistInput) (createMessageWithSeqInput, error) {
	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid bot id: %w", err)
	}

	pgSessionID, err := parseOptionalUUID(input.SessionID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid session id: %w", err)
	}
	pgTurnID, err := parseOptionalUUID(input.TurnID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid turn id: %w", err)
	}
	pgSenderChannelIdentityID, err := parseOptionalUUID(input.SenderChannelIdentityID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid sender channel identity id: %w", err)
	}
	pgSenderUserID, err := parseOptionalUUID(input.SenderUserID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid sender user id: %w", err)
	}
	pgModelID, err := parseOptionalUUID(input.ModelID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid model id: %w", err)
	}
	pgEventID, err := parseOptionalUUID(input.EventID)
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("invalid event id: %w", err)
	}

	metaBytes, err := json.Marshal(nonNilMap(input.Metadata))
	if err != nil {
		return createMessageWithSeqInput{}, fmt.Errorf("marshal message metadata: %w", err)
	}

	content := input.Content
	if len(content) == 0 {
		content = []byte("{}")
	}
	return createMessageWithSeqInput{
		BotID:                   pgBotID,
		SessionID:               pgSessionID,
		TurnID:                  pgTurnID,
		ExplicitTurnMessageSeq:  input.TurnMessageSeq,
		SenderChannelIdentityID: pgSenderChannelIdentityID,
		SenderUserID:            pgSenderUserID,
		ExternalMessageID:       input.ExternalMessageID,
		SourceReplyToMessageID:  input.SourceReplyToMessageID,
		Role:                    input.Role,
		Content:                 content,
		Metadata:                metaBytes,
		Usage:                   input.Usage,
		SessionMode:             input.SessionMode,
		RuntimeType:             input.RuntimeType,
		ModelID:                 pgModelID,
		EventID:                 pgEventID,
		DisplayText:             input.DisplayText,
	}, nil
}

func attachPersistedAssetRefs(message *Message, refs []AssetRef) {
	if message == nil || len(refs) == 0 {
		return
	}
	assets := make([]MessageAsset, 0, len(refs))
	for _, ref := range refs {
		ch := strings.TrimSpace(ref.ContentHash)
		if ch == "" {
			continue
		}
		assets = append(assets, MessageAsset{
			ContentHash: ch,
			Role:        coalesce(ref.Role, "attachment"),
			Ordinal:     ref.Ordinal,
			Mime:        ref.Mime,
			SizeBytes:   ref.SizeBytes,
			StorageKey:  ref.StorageKey,
			Name:        ref.Name,
			Metadata:    ref.Metadata,
		})
	}
	message.Assets = assets
}

func (s *DBService) PublishMessageCreated(message Message) {
	s.publishMessageCreated(message)
}

func resolveRuntimeSnapshotWithQueries(ctx context.Context, queries dbstore.Queries, sessionID pgtype.UUID, sessionMode, runtimeType string) (string, string) {
	sessionMode = normalizeSessionMode(sessionMode)
	runtimeType = normalizeRuntimeType(runtimeType)
	if sessionMode != "" && runtimeType != "" && sessionMode != "subagent" {
		return sessionMode, runtimeType
	}
	if sessionID.Valid && queries != nil {
		if row, err := queries.GetSessionByID(ctx, sessionID); err == nil {
			rowMode, rowRuntime := sessionSnapshotFromRow(row)
			if rowMode == "subagent" && row.ParentSessionID.Valid {
				if parent, parentErr := queries.GetSessionByID(ctx, row.ParentSessionID); parentErr == nil {
					parentMode, parentRuntime := sessionSnapshotFromRow(parent)
					if sessionMode == "" || sessionMode == "subagent" {
						sessionMode = parentMode
					}
					if runtimeType == "" {
						runtimeType = parentRuntime
					}
				}
			}
			if sessionMode == "" {
				sessionMode = rowMode
			}
			if runtimeType == "" {
				runtimeType = rowRuntime
			}
		}
	}
	if sessionMode == "" {
		sessionMode = "chat"
	}
	if runtimeType == "" {
		runtimeType = "model"
	}
	return sessionMode, runtimeType
}

func sessionSnapshotFromRow(row sqlc.BotSession) (string, string) {
	sessionMode := normalizeSessionMode(row.SessionMode)
	if sessionMode == "" {
		sessionMode = legacySessionMode(row.Type)
	}
	runtimeType := normalizeRuntimeType(row.RuntimeType)
	if runtimeType == "" {
		runtimeType = legacyRuntimeType(row.Type)
	}
	return sessionMode, runtimeType
}

func normalizeSessionMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "chat", "discuss", "heartbeat", "schedule", "subagent":
		return strings.TrimSpace(mode)
	default:
		return ""
	}
}

func normalizeRuntimeType(runtimeType string) string {
	switch strings.TrimSpace(runtimeType) {
	case "model", "acp_agent":
		return strings.TrimSpace(runtimeType)
	default:
		return ""
	}
}

func legacySessionMode(typ string) string {
	switch strings.TrimSpace(typ) {
	case "acp_agent":
		return "chat"
	case "discuss", "heartbeat", "schedule", "subagent":
		return strings.TrimSpace(typ)
	default:
		return "chat"
	}
}

func legacyRuntimeType(typ string) string {
	if strings.TrimSpace(typ) == "acp_agent" {
		return "acp_agent"
	}
	return "model"
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

// ListActiveSince returns bot messages since a given time, excluding passive_sync messages.
func (s *DBService) ListActiveSince(ctx context.Context, botID string, since time.Time) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListActiveMessagesSince(ctx, sqlc.ListActiveMessagesSinceParams{
		BotID:     pgBotID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromActiveSince(rows)
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

// ListBefore returns up to limit messages older than before, ordered oldest-first.
func (s *DBService) ListBefore(ctx context.Context, botID string, before time.Time, beforeID string, limit int32) ([]Message, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	_ = beforeID
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

// --- Session-scoped queries ---

// ListBySession returns all messages for a session.
func (s *DBService) ListBySession(ctx context.Context, sessionID string) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesBySession(ctx, pgSessionID)
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromSessionList(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListSinceBySession returns session messages since a given time.
func (s *DBService) ListSinceBySession(ctx context.Context, sessionID string, since time.Time) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesSinceBySession(ctx, sqlc.ListMessagesSinceBySessionParams{
		SessionID: pgSessionID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromSinceBySession(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListActiveSinceBySession returns session messages since a given time, excluding passive_sync messages.
func (s *DBService) ListActiveSinceBySession(ctx context.Context, sessionID string, since time.Time) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListActiveMessagesSinceBySession(ctx, sqlc.ListActiveMessagesSinceBySessionParams{
		SessionID: pgSessionID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromActiveSinceBySession(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListActiveSinceByTurn returns active messages on the ancestor path ending at headTurnID.
func (s *DBService) ListActiveSinceByTurn(ctx context.Context, headTurnID string, since time.Time) ([]Message, error) {
	pgHeadTurnID, err := dbpkg.ParseUUID(headTurnID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListActiveMessagesSinceByTurn(ctx, sqlc.ListActiveMessagesSinceByTurnParams{
		HeadTurnID: pgHeadTurnID,
		CreatedAt:  pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromActiveSinceByTurn(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListLatestBySession returns the latest N session messages.
func (s *DBService) ListLatestBySession(ctx context.Context, sessionID string, limit int32) ([]Message, error) {
	return s.ListLatestBySessionHead(ctx, sessionID, "", limit)
}

// ListLatestBySessionHead returns latest session messages on the selected head
// path. Empty headTurnID means the session default head.
func (s *DBService) ListLatestBySessionHead(ctx context.Context, sessionID string, headTurnID string, limit int32) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgHeadTurnID, err := parseOptionalUUID(headTurnID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesLatestBySession(ctx, sqlc.ListMessagesLatestBySessionParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgHeadTurnID,
		MaxCount:   limit,
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromLatestBySession(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

// ListBeforeBySession returns up to limit session messages older than before.
func (s *DBService) ListBeforeBySession(ctx context.Context, sessionID string, before time.Time, beforeID string, limit int32) ([]Message, error) {
	return s.ListBeforeBySessionHead(ctx, sessionID, "", before, beforeID, limit)
}

// ListBeforeBySessionHead returns older session messages on the selected head
// path. Empty headTurnID means the session default head.
func (s *DBService) ListBeforeBySessionHead(ctx context.Context, sessionID string, headTurnID string, before time.Time, beforeID string, limit int32) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgHeadTurnID, err := parseOptionalUUID(headTurnID)
	if err != nil {
		return nil, err
	}
	pgBeforeID, err := parseOptionalUUID(beforeID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMessagesBeforeBySession(ctx, sqlc.ListMessagesBeforeBySessionParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgHeadTurnID,
		CreatedAt:  pgtype.Timestamptz{Time: before, Valid: true},
		BeforeID:   pgBeforeID,
		MaxCount:   limit,
	})
	if err != nil {
		return nil, err
	}
	msgs := toMessagesFromBeforeBySession(rows)
	s.enrichAssets(ctx, msgs)
	return msgs, nil
}

func (s *DBService) GetSessionTurnGraph(ctx context.Context, sessionID string) (SessionTurnGraph, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return SessionTurnGraph{}, err
	}

	sess, err := s.queries.GetSessionByID(ctx, pgSessionID)
	if err != nil {
		return SessionTurnGraph{}, err
	}
	headRows, err := s.queries.ListSessionTurnHeads(ctx, pgSessionID)
	if err != nil {
		return SessionTurnGraph{}, err
	}
	metadataRows, err := s.queries.ListSessionTurnGraphNodeMetadata(ctx, pgSessionID)
	if err != nil {
		return SessionTurnGraph{}, err
	}

	graph := SessionTurnGraph{
		DefaultHeadTurnID: uuidToString(sess.DefaultHeadTurnID),
		HeadTurnIDs:       make([]string, 0, len(headRows)),
		Nodes:             make([]SessionTurnGraphNode, 0, len(metadataRows)),
	}
	for _, head := range headRows {
		if id := uuidToString(head.HeadTurnID); id != "" {
			graph.HeadTurnIDs = append(graph.HeadTurnIDs, id)
		}
	}

	for _, row := range metadataRows {
		turnID := uuidToString(row.TurnID)
		if turnID == "" {
			continue
		}
		graph.Nodes = append(graph.Nodes, SessionTurnGraphNode{
			TurnID:       turnID,
			ParentTurnID: uuidToString(row.ParentTurnID),
			Timestamp:    formatSessionTurnGraphTimestamp(row.NodeCreatedAt.Time),
			RequestKey:   uuidToString(row.RequestGroupID),
			HasUser:      row.HasUser,
			HasAssistant: row.HasAssistant,
		})
	}
	return graph, nil
}

// ListSessionTurnHeadIDs returns the session's active head turn ids without
// loading turn graph node metadata. The heads table holds at most a few rows
// per session, so this is a cheap indexed lookup compared with the recursive
// graph metadata query.
func (s *DBService) ListSessionTurnHeadIDs(ctx context.Context, sessionID string) ([]string, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	headRows, err := s.queries.ListSessionTurnHeads(ctx, pgSessionID)
	if err != nil {
		return nil, err
	}
	headIDs := make([]string, 0, len(headRows))
	for _, head := range headRows {
		if id := uuidToString(head.HeadTurnID); id != "" {
			headIDs = append(headIDs, id)
		}
	}
	return headIDs, nil
}

// IsSessionTurnHead reports whether headTurnID is currently an active head for
// the session without loading the full turn graph.
func (s *DBService) IsSessionTurnHead(ctx context.Context, sessionID string, headTurnID string) (bool, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return false, err
	}
	pgHeadTurnID, err := parseOptionalUUID(headTurnID)
	if err != nil {
		return false, err
	}
	if !pgHeadTurnID.Valid {
		return false, nil
	}
	_, err = s.queries.GetSessionTurnHead(ctx, sqlc.GetSessionTurnHeadParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgHeadTurnID,
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func formatSessionTurnGraphTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *DBService) LocateByExternalIDBySession(ctx context.Context, sessionID string, externalMessageID string, beforeLimit int32, afterLimit int32) (LocateResult, error) {
	return s.LocateByExternalIDBySessionHead(ctx, sessionID, "", externalMessageID, beforeLimit, afterLimit)
}

// LocateByExternalIDBySessionHead locates a message inside the selected
// session head path. Empty headTurnID keeps the server default-head view.
func (s *DBService) LocateByExternalIDBySessionHead(ctx context.Context, sessionID string, headTurnID string, externalMessageID string, beforeLimit int32, afterLimit int32) (LocateResult, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return LocateResult{}, err
	}
	pgHeadTurnID, err := parseOptionalUUID(headTurnID)
	if err != nil {
		return LocateResult{}, err
	}
	externalMessageID = strings.TrimSpace(externalMessageID)
	if externalMessageID == "" {
		return LocateResult{}, errors.New("external message id is required")
	}
	if beforeLimit < 0 {
		beforeLimit = 0
	}
	if afterLimit < 0 {
		afterLimit = 0
	}

	targetRow, err := s.queries.GetMessageByExternalIDBySession(ctx, sqlc.GetMessageByExternalIDBySessionParams{
		SessionID:         pgSessionID,
		HeadTurnID:        pgHeadTurnID,
		ExternalMessageID: toPgText(externalMessageID),
	})
	if err != nil {
		return LocateResult{}, err
	}
	target := toMessageFromExternalIDBySessionRow(targetRow)

	beforeRows, err := s.queries.ListMessagesBeforeBySession(ctx, sqlc.ListMessagesBeforeBySessionParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgHeadTurnID,
		CreatedAt: pgtype.Timestamptz{
			Time:  target.CreatedAt,
			Valid: true,
		},
		BeforeID: pgtype.UUID{Bytes: targetRow.ID.Bytes, Valid: true},
		MaxCount: beforeLimit,
	})
	if err != nil {
		return LocateResult{}, err
	}
	afterRows, err := s.queries.ListMessagesAfterBySession(ctx, sqlc.ListMessagesAfterBySessionParams{
		SessionID:  pgSessionID,
		HeadTurnID: pgHeadTurnID,
		CreatedAt: pgtype.Timestamptz{
			Time:  target.CreatedAt,
			Valid: true,
		},
		AfterID:  pgtype.UUID{Bytes: targetRow.ID.Bytes, Valid: true},
		MaxCount: afterLimit,
	})
	if err != nil {
		return LocateResult{}, err
	}

	messages := make([]Message, 0, len(beforeRows)+1+len(afterRows))
	messages = append(messages, toMessagesFromBeforeBySession(beforeRows)...)
	messages = append(messages, target)
	messages = append(messages, toMessagesFromAfterBySession(afterRows)...)
	s.enrichAssets(ctx, messages)
	return LocateResult{Messages: messages, TargetID: target.ID}, nil
}

// LinkAssets links asset refs to an existing persisted message.
func (s *DBService) LinkAssets(ctx context.Context, messageID string, assets []AssetRef) error {
	pgMsgID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return fmt.Errorf("invalid message id: %w", err)
	}
	for _, ref := range assets {
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		role := ref.Role
		if strings.TrimSpace(role) == "" {
			role = "attachment"
		}
		if ref.Ordinal < math.MinInt32 || ref.Ordinal > math.MaxInt32 {
			return fmt.Errorf("asset ordinal out of range: %d", ref.Ordinal)
		}
		if _, assetErr := s.queries.CreateMessageAsset(ctx, sqlc.CreateMessageAssetParams{
			MessageID:   pgMsgID,
			Role:        role,
			Ordinal:     int32(ref.Ordinal),
			ContentHash: contentHash,
			Name:        ref.Name,
			Metadata:    marshalMetadata(ref.Metadata),
		}); assetErr != nil {
			s.logger.Warn("link asset failed", slog.String("message_id", messageID), slog.Any("error", assetErr))
		}
	}
	return nil
}

// DeleteByBot deletes all messages for a bot.
func (s *DBService) DeleteByBot(ctx context.Context, botID string) error {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return err
	}
	deleteFn := func(q dbstore.Queries) error {
		if err := q.ClearSessionTurnPointersByBot(ctx, pgBotID); err != nil {
			return err
		}
		if err := q.DeleteSessionTurnHeadsByBot(ctx, pgBotID); err != nil {
			return err
		}
		if err := q.ClearHistoryTurnMessagePointersByBot(ctx, pgBotID); err != nil {
			return err
		}
		if err := q.DeleteMessagesByBot(ctx, pgBotID); err != nil {
			return err
		}
		return q.DeleteHistoryTurnsByBot(ctx, pgBotID)
	}
	if runner, ok := s.queries.(txRunner); ok && runner != nil {
		return runner.RunInTx(ctx, deleteFn)
	}
	return deleteFn(s.queries)
}

// DeleteBySession deletes all messages for a session.
func (s *DBService) DeleteBySession(ctx context.Context, sessionID string) error {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return err
	}
	return s.queries.DeleteMessagesBySession(ctx, pgSessionID)
}

// createFallbackTurn is the legacy/non-TurnRun adapter for message writes that
// still enter through message.DBService directly. Main chat/rewrite/retry runs
// create turns and apply head transitions in conversation/flow; this fallback
// exists for passive channel writes and older integration paths.
func createFallbackTurn(ctx context.Context, q dbstore.Queries, botID, sessionID pgtype.UUID, role string) (pgtype.UUID, error) {
	for attempts := 0; attempts < 2; attempts++ {
		sess, err := q.GetSessionByID(ctx, sessionID)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("get session for history turn: %w", err)
		}
		baseHeadTurnID := sess.DefaultHeadTurnID
		if baseHeadTurnID.Valid {
			head, err := q.GetHistoryTurnByID(ctx, baseHeadTurnID)
			if err != nil {
				return pgtype.UUID{}, fmt.Errorf("get session head turn: %w", err)
			}
			if canAppendFallbackMessageToTurn(head, sessionID, role) {
				return baseHeadTurnID, nil
			}
		}

		turn, err := q.CreateHistoryTurn(ctx, sqlc.CreateHistoryTurnParams{
			BotID:          botID,
			OwnerSessionID: sessionID,
			ParentTurnID:   baseHeadTurnID,
			// Matches conversation.TurnOriginMessage; the literal avoids a
			// message -> conversation import cycle.
			OriginKind: pgtype.Text{String: "message", Valid: true},
		})
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("create history turn: %w", err)
		}
		if baseHeadTurnID.Valid {
			if _, err := q.ReplaceSessionTurnHead(ctx, sqlc.ReplaceSessionTurnHeadParams{
				TargetSessionID: sessionID,
				OldHeadTurnID:   baseHeadTurnID,
				NewHeadTurnID:   turn.ID,
			}); err != nil {
				_ = q.DeleteHistoryTurnByID(ctx, turn.ID)
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return pgtype.UUID{}, fmt.Errorf("replace session turn head: %w", err)
			}
		} else {
			if _, err := q.CreateSessionTurnHead(ctx, sqlc.CreateSessionTurnHeadParams{
				SessionID:  sessionID,
				HeadTurnID: turn.ID,
			}); err != nil {
				_ = q.DeleteHistoryTurnByID(ctx, turn.ID)
				return pgtype.UUID{}, fmt.Errorf("create session turn head: %w", err)
			}
		}
		if _, err := q.UpdateSessionDefaultHeadTurnIfValid(ctx, sqlc.UpdateSessionDefaultHeadTurnIfValidParams{
			ID:                sessionID,
			DefaultHeadTurnID: turn.ID,
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("update session default head turn: %w", err)
		}
		return turn.ID, nil
	}
	sess, err := q.GetSessionByID(ctx, sessionID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("get session for history turn after retry: %w", err)
	}
	if sess.DefaultHeadTurnID.Valid {
		head, err := q.GetHistoryTurnByID(ctx, sess.DefaultHeadTurnID)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("get session head turn after retry: %w", err)
		}
		if canAppendFallbackMessageToTurn(head, sessionID, role) {
			return sess.DefaultHeadTurnID, nil
		}
	}
	return pgtype.UUID{}, errors.New("session head changed while creating history turn")
}

func canAppendFallbackMessageToTurn(turn sqlc.BotHistoryTurn, sessionID pgtype.UUID, role string) bool {
	if turn.OwnerSessionID != sessionID {
		return false
	}
	switch strings.TrimSpace(role) {
	case "user":
		return !turn.RequestMessageID.Valid && !turn.FinalAssistantMessageID.Valid
	default:
		return !turn.FinalAssistantMessageID.Valid
	}
}

func resolveTurnMessageSeq(ctx context.Context, q dbstore.Queries, turnID pgtype.UUID, explicit int64) (pgtype.Int8, error) {
	if !turnID.Valid {
		return pgtype.Int8{}, nil
	}
	if explicit > 0 {
		return pgtype.Int8{Int64: explicit, Valid: true}, nil
	}
	seq, err := q.GetNextTurnMessageSeq(ctx, turnID)
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("get next turn message seq: %w", err)
	}
	return pgtype.Int8{Int64: seq, Valid: true}, nil
}

type createMessageWithSeqInput struct {
	BotID                   pgtype.UUID
	SessionID               pgtype.UUID
	TurnID                  pgtype.UUID
	ExplicitTurnMessageSeq  int64
	SenderChannelIdentityID pgtype.UUID
	SenderUserID            pgtype.UUID
	ExternalMessageID       string
	SourceReplyToMessageID  string
	Role                    string
	Content                 []byte
	Metadata                []byte
	Usage                   []byte
	ModelID                 pgtype.UUID
	SessionMode             string
	RuntimeType             string
	EventID                 pgtype.UUID
	DisplayText             string
}

func (s *DBService) persistMessageAtomic(ctx context.Context, input createMessageWithSeqInput, assets []AssetRef) (Message, error) {
	const maxAttempts = 3
	for attempt := 0; ; attempt++ {
		var result Message
		err := s.runInTx(ctx, func(q dbstore.Queries) error {
			stored, err := persistMessageWithQueries(ctx, q, input, assets)
			if err != nil {
				return err
			}
			result = stored
			return nil
		})
		if err == nil {
			return result, nil
		}
		if input.ExplicitTurnMessageSeq > 0 || (!input.TurnID.Valid && !input.SessionID.Valid) || attempt+1 >= maxAttempts || !dbpkg.IsUniqueViolation(err) {
			return Message{}, err
		}
	}
}

func persistMessageWithQueries(ctx context.Context, q dbstore.Queries, input createMessageWithSeqInput, assets []AssetRef) (Message, error) {
	txInput := input
	if !txInput.TurnID.Valid && txInput.SessionID.Valid {
		turnID, err := createFallbackTurn(ctx, q, txInput.BotID, txInput.SessionID, txInput.Role)
		if err != nil {
			return Message{}, err
		}
		txInput.TurnID = turnID
	}
	row, err := createMessageWithTurnSeq(ctx, q, txInput)
	if err != nil {
		return Message{}, err
	}
	result := toMessageFromCreate(row)
	if txInput.TurnID.Valid {
		if err := updateTurnPointers(ctx, q, txInput.TurnID, result); err != nil {
			return Message{}, err
		}
	}
	if err := createMessageAssetLinks(ctx, q, row.ID, result.ID, assets); err != nil {
		return Message{}, err
	}
	return result, nil
}

func (s *DBService) runInTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	if runner, ok := s.queries.(txRunner); ok && runner != nil {
		return runner.RunInTx(ctx, fn)
	}
	return fn(s.queries)
}

func createMessageWithTurnSeq(ctx context.Context, q dbstore.Queries, input createMessageWithSeqInput) (sqlc.CreateMessageRow, error) {
	turnMessageSeq, err := resolveTurnMessageSeq(ctx, q, input.TurnID, input.ExplicitTurnMessageSeq)
	if err != nil {
		return sqlc.CreateMessageRow{}, err
	}
	sessionMode, runtimeType := resolveRuntimeSnapshotWithQueries(ctx, q, input.SessionID, input.SessionMode, input.RuntimeType)
	return q.CreateMessage(ctx, sqlc.CreateMessageParams{
		BotID:                   input.BotID,
		SessionID:               input.SessionID,
		TurnID:                  input.TurnID,
		TurnMessageSeq:          turnMessageSeq,
		SenderChannelIdentityID: input.SenderChannelIdentityID,
		SenderUserID:            input.SenderUserID,
		ExternalMessageID:       toPgText(input.ExternalMessageID),
		SourceReplyToMessageID:  toPgText(input.SourceReplyToMessageID),
		Role:                    input.Role,
		Content:                 input.Content,
		Metadata:                input.Metadata,
		Usage:                   input.Usage,
		SessionMode:             sessionMode,
		RuntimeType:             runtimeType,
		ModelID:                 input.ModelID,
		EventID:                 input.EventID,
		DisplayText:             toPgText(input.DisplayText),
	})
}

func updateTurnPointers(ctx context.Context, q dbstore.Queries, turnID pgtype.UUID, msg Message) error {
	switch strings.TrimSpace(msg.Role) {
	case "user":
		if _, err := q.UpdateHistoryTurnRequestMessage(ctx, sqlc.UpdateHistoryTurnRequestMessageParams{
			ID:               turnID,
			RequestMessageID: uuidFromString(msg.ID),
		}); err != nil {
			return fmt.Errorf("update history turn request message: %w", err)
		}
	case "assistant":
		if _, err := q.UpdateHistoryTurnFinalAssistantMessage(ctx, sqlc.UpdateHistoryTurnFinalAssistantMessageParams{
			ID:                      turnID,
			FinalAssistantMessageID: uuidFromString(msg.ID),
		}); err != nil {
			return fmt.Errorf("update history turn final assistant message: %w", err)
		}
	}
	return nil
}

func createMessageAssetLinks(ctx context.Context, q dbstore.Queries, messageID pgtype.UUID, messageIDText string, assets []AssetRef) error {
	for _, ref := range assets {
		role := ref.Role
		if strings.TrimSpace(role) == "" {
			role = "attachment"
		}
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		if ref.Ordinal < math.MinInt32 || ref.Ordinal > math.MaxInt32 {
			return fmt.Errorf("asset ordinal out of range: %d", ref.Ordinal)
		}
		if _, err := q.CreateMessageAsset(ctx, sqlc.CreateMessageAssetParams{
			MessageID:   messageID,
			Role:        role,
			Ordinal:     int32(ref.Ordinal),
			ContentHash: contentHash,
			Name:        ref.Name,
			Metadata:    marshalMetadata(ref.Metadata),
		}); err != nil {
			return fmt.Errorf("create message asset link for %s: %w", messageIDText, err)
		}
	}
	return nil
}

// --- Conversion helpers ---

func toMessageFromCreate(row sqlc.CreateMessageRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
		row.SenderChannelIdentityID,
		row.SenderUserID,
		pgtype.Text{},
		pgtype.Text{},
		extractPlatformFromMetadata(row.Metadata),
		row.ExternalMessageID,
		row.SourceReplyToMessageID,
		row.Role,
		row.Content,
		row.Metadata,
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func extractPlatformFromMetadata(metadata []byte) pgtype.Text {
	m := parseJSONMap(metadata)
	if v, ok := m["platform"].(string); ok && strings.TrimSpace(v) != "" {
		return pgtype.Text{String: strings.TrimSpace(v), Valid: true}
	}
	return pgtype.Text{}
}

func toMessageFromListRow(row sqlc.ListMessagesRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromSessionListRow(row sqlc.ListMessagesBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromSinceRow(row sqlc.ListMessagesSinceRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromSinceBySessionRow(row sqlc.ListMessagesSinceBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromActiveSinceRow(row sqlc.ListActiveMessagesSinceRow) Message {
	m := toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
	if row.CompactID.Valid {
		m.CompactID = row.CompactID.String()
	}
	return m
}

func toMessageFromActiveSinceBySessionRow(row sqlc.ListActiveMessagesSinceBySessionRow) Message {
	m := toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
	if row.CompactID.Valid {
		m.CompactID = row.CompactID.String()
	}
	return m
}

func toMessageFromActiveSinceByTurnRow(row sqlc.ListActiveMessagesSinceByTurnRow) Message {
	m := toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
	if row.CompactID.Valid {
		m.CompactID = row.CompactID.String()
	}
	return m
}

func toMessageFromLatestRow(row sqlc.ListMessagesLatestRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromLatestBySessionRow(row sqlc.ListMessagesLatestBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromBeforeRow(row sqlc.ListMessagesBeforeRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromBeforeBySessionRow(row sqlc.ListMessagesBeforeBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromExternalIDBySessionRow(row sqlc.GetMessageByExternalIDBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFromAfterBySessionRow(row sqlc.ListMessagesAfterBySessionRow) Message {
	return toMessageFields(
		row.ID,
		row.BotID,
		row.SessionID,
		row.TurnID,
		row.TurnMessageSeq,
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
		row.Usage,
		row.SessionMode,
		row.RuntimeType,
		row.EventID,
		row.DisplayText,
		row.CreatedAt,
	)
}

func toMessageFields(
	id pgtype.UUID,
	botID pgtype.UUID,
	sessionID pgtype.UUID,
	turnID pgtype.UUID,
	turnMessageSeq pgtype.Int8,
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
	usage []byte,
	sessionMode string,
	runtimeType string,
	eventID pgtype.UUID,
	displayText pgtype.Text,
	createdAt pgtype.Timestamptz,
) Message {
	m := Message{
		ID:                      id.String(),
		BotID:                   botID.String(),
		SessionID:               sessionID.String(),
		TurnID:                  uuidToString(turnID),
		TurnMessageSeq:          int8ToInt64(turnMessageSeq),
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
		Usage:                   json.RawMessage(usage),
		SessionMode:             sessionMode,
		RuntimeType:             runtimeType,
		DisplayContent:          dbpkg.TextToString(displayText),
		CreatedAt:               createdAt.Time,
	}
	if eventID.Valid {
		m.EventID = eventID.String()
	}
	return m
}

func toMessagesFromList(rows []sqlc.ListMessagesRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromListRow(row))
	}
	return messages
}

func toMessagesFromSessionList(rows []sqlc.ListMessagesBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromSessionListRow(row))
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

func toMessagesFromSinceBySession(rows []sqlc.ListMessagesSinceBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromSinceBySessionRow(row))
	}
	return messages
}

func toMessagesFromActiveSince(rows []sqlc.ListActiveMessagesSinceRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromActiveSinceRow(row))
	}
	return messages
}

func toMessagesFromActiveSinceBySession(rows []sqlc.ListActiveMessagesSinceBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromActiveSinceBySessionRow(row))
	}
	return messages
}

func toMessagesFromActiveSinceByTurn(rows []sqlc.ListActiveMessagesSinceByTurnRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromActiveSinceByTurnRow(row))
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

func toMessagesFromLatestBySession(rows []sqlc.ListMessagesLatestBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromLatestBySessionRow(row))
	}
	return messages
}

// toMessagesFromBefore returns messages in oldest-first order (ListMessagesBefore returns DESC; we reverse).
func toMessagesFromBefore(rows []sqlc.ListMessagesBeforeRow) []Message {
	messages := make([]Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		messages = append(messages, toMessageFromBeforeRow(rows[i]))
	}
	return messages
}

func toMessagesFromBeforeBySession(rows []sqlc.ListMessagesBeforeBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		messages = append(messages, toMessageFromBeforeBySessionRow(rows[i]))
	}
	return messages
}

func toMessagesFromAfterBySession(rows []sqlc.ListMessagesAfterBySessionRow) []Message {
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageFromAfterBySessionRow(row))
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

func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

func int8ToInt64(value pgtype.Int8) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func uuidFromString(value string) pgtype.UUID {
	id, err := dbpkg.ParseUUID(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
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

// enrichAssets batch-loads asset links for a list of messages (single-table query).
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
		s.logger.Warn("enrich assets failed, returning messages without assets", slog.Any("error", err))
		ensureAssetsSlice(messages)
		return
	}
	assetMap := map[string][]MessageAsset{}
	for _, row := range rows {
		msgID := row.MessageID.String()
		contentHash := strings.TrimSpace(row.ContentHash)
		if contentHash == "" {
			continue
		}
		assetMap[msgID] = append(assetMap[msgID], MessageAsset{
			ContentHash: contentHash,
			Role:        row.Role,
			Ordinal:     int(row.Ordinal),
			Name:        row.Name,
			Metadata:    unmarshalMetadata(row.Metadata),
		})
	}
	for i := range messages {
		if assets, ok := assetMap[messages[i].ID]; ok {
			messages[i].Assets = assets
		} else {
			messages[i].Assets = []MessageAsset{}
		}
	}
}

func ensureAssetsSlice(messages []Message) {
	for i := range messages {
		if messages[i].Assets == nil {
			messages[i].Assets = []MessageAsset{}
		}
	}
}

func marshalMetadata(m map[string]any) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func unmarshalMetadata(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}
