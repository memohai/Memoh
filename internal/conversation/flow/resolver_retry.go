package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type retryStore interface {
	GetSessionByID(context.Context, pgtype.UUID) (dbsqlc.BotSession, error)
	GetVisibleAssistantTurnForRetry(context.Context, dbsqlc.GetVisibleAssistantTurnForRetryParams) (dbsqlc.GetVisibleAssistantTurnForRetryRow, error)
	turnStore
	rewriteAssetStore
}

type rewriteStore interface {
	turnStore
	rewriteAssetStore
}

type rewriteAssetStore interface {
	ListMessageAssets(context.Context, pgtype.UUID) ([]dbsqlc.ListMessageAssetsRow, error)
}

type RewriteSource struct {
	TargetMessageID string
	Query           string
}

type rewritePlan struct {
	Anchor conversation.TurnAnchor
	Query  string
}

// StreamRetryWS regenerates the request that produced a visible assistant reply.
// It reuses the rewrite turn path by targeting the original user message,
// creating a sibling turn under the same parent and moving the current session
// head only after the regenerated reply is persisted.
func (r *Resolver) StreamRetryWS(
	ctx context.Context,
	req conversation.ChatRequest,
	replyMessageID string,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) error {
	if r == nil || r.queries == nil {
		return errors.New("retry is not configured")
	}
	store, ok := r.queries.(retryStore)
	if !ok {
		return errors.New("retry is not supported by this database")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	pgReplyID, err := dbpkg.ParseUUID(replyMessageID)
	if err != nil {
		return fmt.Errorf("invalid retry message id: %w", err)
	}
	if err := ensureSessionSupportsTurnVariants(ctx, store, pgSessionID); err != nil {
		return err
	}
	baseHead, err := resolveBaseSessionHead(ctx, store, pgSessionID, req.BaseHeadTurnID)
	if err != nil {
		return fmt.Errorf("resolve retry base head: %w", err)
	}
	pgBaseHeadTurnID := baseHead.HeadTurnID
	if !pgBaseHeadTurnID.Valid {
		return errors.New("retry requires a session head")
	}
	row, err := store.GetVisibleAssistantTurnForRetry(ctx, dbsqlc.GetVisibleAssistantTurnForRetryParams{
		MessageID:      pgReplyID,
		BaseHeadTurnID: pgBaseHeadTurnID,
		SessionID:      pgSessionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("retry source is not a visible assistant reply")
		}
		return fmt.Errorf("resolve retry source: %w", err)
	}
	query := conversation.ExtractDisplayUserText(row.RequestContent, dbpkg.TextToString(row.RequestDisplayText))
	attachments, err := rewriteRequestAttachments(ctx, store, row.RequestMessageID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(query) == "" && len(attachments) == 0 {
		return errors.New("rewrite request is empty")
	}
	anchor := conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleUser,
		MessageID:      row.RequestMessageID.String(),
		TurnID:         row.TurnID.String(),
		ParentTurnID:   uuidString(row.ParentTurnID),
		BaseHeadTurnID: pgBaseHeadTurnID.String(),
		OriginKind:     conversation.TurnOriginRetry,
		RequestGroupID: uuidString(row.RequestGroupID),
	}
	retryReq := req
	retryReq.Attachments = attachments
	return r.streamRewritePlanWS(ctx, retryReq, rewritePlan{
		Anchor: anchor,
		Query:  query,
	}, eventCh, abortCh)
}

func (r *Resolver) StreamRewriteWS(
	ctx context.Context,
	req conversation.ChatRequest,
	source RewriteSource,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) error {
	if r == nil || r.queries == nil {
		return errors.New("rewrite is not configured")
	}
	store, ok := r.queries.(rewriteStore)
	if !ok {
		return errors.New("rewrite is not supported by this database")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	targetMessageID := strings.TrimSpace(source.TargetMessageID)
	if strings.TrimSpace(targetMessageID) == "" {
		return errors.New("rewrite target message id is required")
	}
	pgMessageID, err := dbpkg.ParseUUID(targetMessageID)
	if err != nil {
		return fmt.Errorf("invalid rewrite target message id: %w", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	if err := ensureSessionSupportsTurnVariants(ctx, store, pgSessionID); err != nil {
		return err
	}
	baseHead, err := resolveBaseSessionHead(ctx, store, pgSessionID, req.BaseHeadTurnID)
	if err != nil {
		return err
	}
	pgBaseHeadTurnID := baseHead.HeadTurnID
	if !pgBaseHeadTurnID.Valid {
		return errors.New("rewrite requires a session head")
	}
	anchor, err := resolveVisibleUserTurnAnchor(ctx, store, pgSessionID, pgBaseHeadTurnID, targetMessageID)
	if err != nil {
		return err
	}
	attachments, err := rewriteRequestAttachments(ctx, store, pgMessageID)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(source.Query)
	if query == "" {
		query = strings.TrimSpace(req.Query)
	}
	if query == "" && len(attachments) == 0 {
		return errors.New("rewrite request is empty")
	}

	rewriteReq := req
	rewriteReq.Query = query
	rewriteReq.RawQuery = query
	rewriteReq.Attachments = attachments
	return r.streamRewritePlanWS(ctx, rewriteReq, rewritePlan{
		Anchor: anchor,
		Query:  query,
	}, eventCh, abortCh)
}

func (r *Resolver) streamRewritePlanWS(
	ctx context.Context,
	req conversation.ChatRequest,
	plan rewritePlan,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) error {
	rewriteReq := req
	rewriteReq.Query = plan.Query
	rewriteReq.RawQuery = plan.Query
	rewriteReq.RewriteTargetMessageID = ""
	return r.streamChatWSWithRewriteAnchor(ctx, rewriteReq, &plan.Anchor, eventCh, abortCh)
}

func ensureSessionSupportsTurnVariants(ctx context.Context, store sessionHeadStore, sessionID pgtype.UUID) error {
	sess, err := store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if !sessionpkg.SupportsTurnVariants(sess.Type) {
		return errors.New("turn variants are only supported for chat sessions")
	}
	return nil
}

func rewriteRequestAttachments(ctx context.Context, store rewriteAssetStore, requestMessageID pgtype.UUID) ([]conversation.ChatAttachment, error) {
	rows, err := store.ListMessageAssets(ctx, requestMessageID)
	if err != nil {
		return nil, fmt.Errorf("load rewrite request attachments: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	attachments := make([]conversation.ChatAttachment, 0, len(rows))
	for _, row := range rows {
		contentHash := strings.TrimSpace(row.ContentHash)
		if contentHash == "" {
			continue
		}
		meta := unmarshalRetryAssetMetadata(row.Metadata)
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = attachmentpkg.MetadataString(meta, attachmentpkg.MetadataKeyName)
		}
		storageKey := attachmentpkg.MetadataString(meta, attachmentpkg.MetadataKeyStorageKey)
		attachment := conversation.ChatAttachment{
			Type:        attachmentpkg.InferTypeFromExt(firstNonEmptyRetryAssetRef(name, storageKey)),
			ContentHash: contentHash,
			Name:        name,
			Metadata:    meta,
		}
		if storageKey != "" {
			attachment.Path = attachmentpkg.MediaAccessPath(storageKey)
		}
		if attachment.Type == "" {
			attachment.Type = "file"
		}
		attachments = append(attachments, conversation.ChatAttachmentFromBundle(conversation.BundleFromChatAttachment(attachment)))
	}
	if len(attachments) == 0 {
		return nil, nil
	}
	return attachments, nil
}

func firstNonEmptyRetryAssetRef(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func unmarshalRetryAssetMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil || len(meta) == 0 {
		return nil
	}
	return meta
}
