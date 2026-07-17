package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acpprofile"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/runtimefence"
)

type runtimeFencedSessionWriter interface {
	UpdateSessionTitleWithRuntimeFence(ctx context.Context, arg sqlc.UpdateSessionTitleWithRuntimeFenceParams) (sqlc.BotSession, error)
	UpdateSessionMetadataWithRuntimeFence(ctx context.Context, arg sqlc.UpdateSessionMetadataWithRuntimeFenceParams) (sqlc.BotSession, error)
}

// Session represents a chat session within a bot.
type Session struct {
	ID                    string         `json:"id"`
	BotID                 string         `json:"bot_id"`
	RouteID               string         `json:"route_id,omitempty"`
	ChannelType           string         `json:"channel_type,omitempty"`
	Type                  string         `json:"type"`
	SessionMode           string         `json:"session_mode"`
	RuntimeType           string         `json:"runtime_type"`
	RuntimeMetadata       map[string]any `json:"runtime_metadata,omitempty"`
	Title                 string         `json:"title"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	ParentSessionID       string         `json:"parent_session_id,omitempty"`
	CreatedByUserID       string         `json:"created_by_user_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	RouteMetadata         map[string]any `json:"route_metadata,omitempty"`
	RouteConversationType string         `json:"route_conversation_type,omitempty"`
}

const (
	TypeChat              = "chat"
	TypeHeartbeat         = "heartbeat"
	TypeSchedule          = "schedule"
	TypeSubagent          = "subagent"
	TypeDiscuss           = "discuss"
	TypeACPAgent          = "acp_agent"
	RuntimeModel          = "model"
	RuntimeACPAgent       = "acp_agent"
	DefaultACPProjectMode = "project"
	DefaultACPProjectPath = "/data"
)

// userFacingSessionTypes lists the session types intended to appear in
// user-facing session lists. Heartbeat, schedule, and subagent sessions are
// system-internal — they back agent-driven loops and never surface in the UI.
var userFacingSessionTypes = []string{TypeChat, TypeDiscuss, TypeACPAgent}

// UserFacingSessionTypes returns a fresh copy of the user-facing session type
// list so callers can read or mutate it without disturbing the package-level
// source of truth.
func UserFacingSessionTypes() []string {
	out := make([]string, len(userFacingSessionTypes))
	copy(out, userFacingSessionTypes)
	return out
}

var (
	ErrACPAgentIDRequired     = errors.New("acp_agent_id is required for acp_agent sessions")
	ErrACPProjectPathMissing  = errors.New("project_path is required for acp_agent sessions")
	ErrACPUnknownAgent        = errors.New("unknown ACP agent")
	ErrACPAgentNotEnabled     = errors.New("ACP agent is not enabled for this bot")
	ErrACPAgentNotConfigured  = errors.New("ACP agent is not configured for this bot")
	ErrACPRuntimeOwnerMissing = errors.New("runtime_owner_account_id is required for acp_agent sessions")
	ErrACPProjectModeInvalid  = errors.New("unknown ACP project mode")
	ErrForkSourceNotFound     = errors.New("fork source session not found")
	ErrForkSourceNotReply     = errors.New("fork source must be a visible assistant reply")
	ErrForkSourceNotChat      = errors.New("fork source must be a chat session")
)

func IsKnownType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case TypeChat, TypeHeartbeat, TypeSchedule, TypeSubagent, TypeDiscuss, TypeACPAgent:
		return true
	default:
		return false
	}
}

// IsUserFacingType reports whether a session type is one that user-facing
// session list endpoints should return by default.
func IsUserFacingType(typ string) bool {
	return slices.Contains(userFacingSessionTypes, strings.TrimSpace(typ))
}

// CreateInput holds input for creating a new session.
type CreateInput struct {
	BotID           string
	RouteID         string
	ChannelType     string
	Type            string
	SessionMode     string
	RuntimeType     string
	Title           string
	Metadata        map[string]any
	RuntimeMetadata map[string]any
	ParentSessionID string
	CreatedByUserID string
}

// ForkFromAssistantInput creates a new chat session from the source session's
// visible history through the assistant message's turn.
type ForkFromAssistantInput struct {
	BotID           string
	SessionID       string
	MessageID       string
	Title           string
	CreatedByUserID string
}

// Service manages bot chat sessions.
type Service struct {
	queries     dbstore.Queries
	hookService *hooks.Service
	publisher   event.Publisher
	logger      *slog.Logger
}

// NewService creates a session service. publisher may be nil — session
// creation still succeeds when there is no event hub wired in (tests, or any
// caller that doesn't surface activity events).
func NewService(log *slog.Logger, queries dbstore.Queries, publisher event.Publisher) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:   queries,
		publisher: publisher,
		logger:    log.With(slog.String("service", "session")),
	}
}

func (s *Service) SetHookService(h *hooks.Service) {
	s.hookService = h
}

// canonicalChannelType maps the product's own inbound surfaces (web composer,
// bundled CLI) to the single "local" channel type before persisting. The web
// REST session-create path has always stored "local", and the web UI treats
// any other non-empty channel_type as an external channel thread (read-only,
// channel badge), so adapter names like "web"/"cli" must not leak into
// bot_sessions. External channel adapters (telegram, discord, ...) pass
// through unchanged. Keep the surface list in sync with isLocalChannelType in
// internal/channel/inbound.
func canonicalChannelType(ct string) string {
	trimmed := strings.TrimSpace(ct)
	switch strings.ToLower(trimmed) {
	case "web", "cli":
		return "local"
	}
	return trimmed
}

// Create creates a new session.
func (s *Service) Create(ctx context.Context, input CreateInput) (Session, error) {
	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid bot id: %w", err)
	}
	pgRouteID, err := parseOptionalUUID(input.RouteID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid route id: %w", err)
	}
	pgCreatedByUserID, err := parseOptionalUUID(input.CreatedByUserID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid created by user id: %w", err)
	}
	runtimeOwnerUserID := strings.TrimSpace(input.CreatedByUserID)

	channelType := pgtype.Text{}
	if ct := canonicalChannelType(input.ChannelType); ct != "" {
		channelType = pgtype.Text{String: ct, Valid: true}
	}

	meta := input.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	sessionType := strings.TrimSpace(input.Type)
	if sessionType == "" {
		sessionType = TypeChat
	}
	if !IsKnownType(sessionType) {
		return Session{}, fmt.Errorf("unknown session type %q", sessionType)
	}
	desc, err := normalizeDescriptor(sessionType, input.SessionMode, input.RuntimeType, meta, input.RuntimeMetadata)
	if err != nil {
		return Session{}, err
	}
	sessionType = desc.LegacyType
	meta = desc.Metadata
	runtimeMeta := desc.RuntimeMetadata
	if desc.RuntimeType == RuntimeACPAgent {
		meta = ApplyACPMetadataDefaults(meta)
		runtimeMeta = ApplyACPMetadataDefaults(runtimeMeta)
		meta = setACPRuntimeOwner(meta, runtimeOwnerUserID)
		runtimeMeta = setACPRuntimeOwner(runtimeMeta, runtimeOwnerUserID)
		meta = mergeACPMetadata(meta, runtimeMeta)
		runtimeMeta = mergeACPMetadata(runtimeMeta, meta)
		if err := validateACPMetadata(meta); err != nil {
			return Session{}, err
		}
		if err := s.validateACPCreatePolicy(ctx, pgBotID, meta); err != nil {
			return Session{}, err
		}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return Session{}, fmt.Errorf("marshal metadata: %w", err)
	}
	runtimeMetaBytes, err := json.Marshal(nonNilMap(runtimeMeta))
	if err != nil {
		return Session{}, fmt.Errorf("marshal runtime metadata: %w", err)
	}

	pgParentSessionID, err := parseOptionalUUID(input.ParentSessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid parent session id: %w", err)
	}

	row, err := s.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		BotID:           pgBotID,
		RouteID:         pgRouteID,
		ChannelType:     channelType,
		Type:            sessionType,
		SessionMode:     desc.SessionMode,
		RuntimeType:     desc.RuntimeType,
		RuntimeMetadata: runtimeMetaBytes,
		Title:           input.Title,
		Metadata:        metaBytes,
		ParentSessionID: pgParentSessionID,
		CreatedByUserID: pgCreatedByUserID,
	})
	if err != nil {
		return Session{}, err
	}
	sess := toSession(row)
	s.publishSessionCreated(sess)
	s.runSessionStartHook(context.WithoutCancel(ctx), sess)
	return sess, nil
}

// ForkFromAssistantMessage creates a new chat session containing the source
// session's visible linear history through the selected assistant turn.
func (s *Service) ForkFromAssistantMessage(ctx context.Context, input ForkFromAssistantInput) (Session, error) {
	pgBotID, err := dbpkg.ParseUUID(input.BotID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid bot id: %w", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(input.SessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	pgMessageID, err := dbpkg.ParseUUID(input.MessageID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid message id: %w", err)
	}
	pgCreatedByUserID, err := parseOptionalUUID(input.CreatedByUserID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid created by user id: %w", err)
	}

	sourceRow, err := s.queries.GetSessionByID(ctx, pgSessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrForkSourceNotFound
		}
		return Session{}, err
	}
	source := toSession(sourceRow)
	if source.BotID != pgBotID.String() {
		return Session{}, ErrForkSourceNotFound
	}
	if source.Type != TypeChat {
		return Session{}, ErrForkSourceNotChat
	}

	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "Untitled"
	}
	forkTitle := strings.TrimSpace(input.Title)
	if forkTitle == "" {
		forkTitle = title + " fork"
	}
	meta := nonNilMap(source.Metadata)
	// A fork is a new execution branch. It inherits conversation context, but
	// starts from the Bot's current Primary Computer instead of silently
	// carrying the source Session's last request-scoped target.
	delete(meta, "workspace_target_id")
	delete(meta, "workspace_target")
	meta["forked_from"] = map[string]any{
		"session_id": source.ID,
		"title":      title,
		"message_id": pgMessageID.String(),
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return Session{}, fmt.Errorf("marshal metadata: %w", err)
	}

	row, err := s.queries.ForkSessionFromAssistantMessage(ctx, sqlc.ForkSessionFromAssistantMessageParams{
		SessionID:       pgSessionID,
		BotID:           pgBotID,
		MessageID:       pgMessageID,
		Title:           forkTitle,
		Metadata:        metaBytes,
		CreatedByUserID: pgCreatedByUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, ErrForkSourceNotReply
		}
		return Session{}, err
	}
	sess := toSessionFromForkRow(row)
	s.publishSessionCreated(sess)
	s.runSessionStartHook(context.WithoutCancel(ctx), sess)
	return sess, nil
}

// publishSessionCreated emits a session_created event for the new session.
// Best-effort: failures are logged but never fail the create.
func (s *Service) publishSessionCreated(sess Session) {
	if s.publisher == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": sess.ID,
		"bot_id":     sess.BotID,
		"type":       sess.Type,
		"title":      sess.Title,
		"created_at": sess.CreatedAt,
	})
	if err != nil {
		s.logger.Warn("marshal session_created event failed", slog.Any("error", err))
		return
	}
	s.publisher.Publish(event.Event{
		Type:  event.EventTypeSessionCreated,
		BotID: strings.TrimSpace(sess.BotID),
		Data:  payload,
	})
}

func (s *Service) runSessionStartHook(ctx context.Context, sess Session) {
	if s == nil || s.hookService == nil {
		return
	}
	req := hooks.Request{
		Version:   1,
		Event:     hooks.EventSessionStart,
		BotID:     sess.BotID,
		SessionID: sess.ID,
		Workspace: hooks.WorkspaceInfo{
			CWD: hooks.DefaultWorkDir,
		},
		Turn: map[string]any{
			"session_type": sess.Type,
			"route_id":     sess.RouteID,
			"channel_type": sess.ChannelType,
		},
	}
	if _, err := s.hookService.Run(ctx, req, nil); err != nil {
		s.logger.Warn("session start hook failed",
			slog.String("bot_id", sess.BotID),
			slog.String("session_id", sess.ID),
			slog.Any("error", err),
		)
	}
}

// UpdateTypeAndMetadata updates a session's runtime type and metadata in one
// statement so callers don't expose a half-updated agent selection.
func (s *Service) UpdateTypeAndMetadata(ctx context.Context, sessionID, typ string, metadata map[string]any) (Session, error) {
	return s.updateTypeAndMetadata(ctx, sessionID, typ, metadata, "")
}

// UpdateTypeAndMetadataWithOwner updates a session descriptor and binds any
// ACP runtime ownership to a server-confirmed account id. The metadata owner
// field is never trusted from callers.
func (s *Service) UpdateTypeAndMetadataWithOwner(ctx context.Context, sessionID, typ string, metadata map[string]any, runtimeOwnerUserID string) (Session, error) {
	return s.updateTypeAndMetadata(ctx, sessionID, typ, metadata, strings.TrimSpace(runtimeOwnerUserID))
}

func (s *Service) updateTypeAndMetadata(ctx context.Context, sessionID, typ string, metadata map[string]any, runtimeOwnerUserID string) (Session, error) {
	return s.updateDescriptorAndMetadata(ctx, sessionID, typ, "", "", metadata, nil, strings.TrimSpace(runtimeOwnerUserID))
}

// UpdateDescriptorAndMetadataWithOwner updates the session mode/runtime
// descriptor directly. Callers that only patch metadata for a Phase 3 session
// must pass the existing descriptor so discuss+ACP sessions keep their runtime.
func (s *Service) UpdateDescriptorAndMetadataWithOwner(ctx context.Context, sessionID, typ, sessionMode, runtimeType string, metadata, runtimeMetadata map[string]any, runtimeOwnerUserID string) (Session, error) {
	return s.updateDescriptorAndMetadata(ctx, sessionID, typ, sessionMode, runtimeType, metadata, runtimeMetadata, strings.TrimSpace(runtimeOwnerUserID))
}

func (s *Service) updateDescriptorAndMetadata(ctx context.Context, sessionID, typ, sessionMode, runtimeType string, metadata, runtimeMetadata map[string]any, runtimeOwnerUserID string) (Session, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	sessionType := strings.TrimSpace(typ)
	if sessionType == "" {
		sessionType = TypeChat
	}
	if !IsKnownType(sessionType) {
		return Session{}, fmt.Errorf("unknown session type %q", sessionType)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	existing, err := s.queries.GetSessionByID(ctx, pgID)
	if err != nil {
		return Session{}, err
	}
	existingRuntimeMeta := parseJSONMap(existing.RuntimeMetadata)
	existingMeta := parseJSONMap(existing.Metadata)
	if normalizeRuntimeType(existing.RuntimeType, existing.Type) == RuntimeACPAgent {
		existingRuntimeOwnerUserID := metadataString(existingRuntimeMeta, "runtime_owner_account_id")
		if existingRuntimeOwnerUserID == "" {
			existingRuntimeOwnerUserID = metadataString(existingMeta, "runtime_owner_account_id")
		}
		if existingRuntimeOwnerUserID != "" {
			runtimeOwnerUserID = existingRuntimeOwnerUserID
		}
	}
	if runtimeMetadata == nil {
		runtimeMetadata = existingRuntimeMeta
	}
	desc, err := normalizeDescriptor(sessionType, sessionMode, runtimeType, metadata, runtimeMetadata)
	if err != nil {
		return Session{}, err
	}
	sessionType = desc.LegacyType
	metadata = desc.Metadata
	runtimeMeta := desc.RuntimeMetadata
	if desc.RuntimeType != RuntimeACPAgent {
		runtimeMeta = map[string]any{}
	}
	if desc.RuntimeType == RuntimeACPAgent {
		metadata = ApplyACPMetadataDefaults(metadata)
		runtimeMeta = ApplyACPMetadataDefaults(runtimeMeta)
		metadata = setACPRuntimeOwner(metadata, runtimeOwnerUserID)
		runtimeMeta = setACPRuntimeOwner(runtimeMeta, runtimeOwnerUserID)
		metadata = mergeACPMetadata(metadata, runtimeMeta)
		runtimeMeta = mergeACPMetadata(runtimeMeta, metadata)
		if err := validateACPMetadata(metadata); err != nil {
			return Session{}, err
		}
		if err := s.validateACPCreatePolicy(ctx, existing.BotID, metadata); err != nil {
			return Session{}, err
		}
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return Session{}, fmt.Errorf("marshal metadata: %w", err)
	}
	runtimeMetaBytes, err := json.Marshal(nonNilMap(runtimeMeta))
	if err != nil {
		return Session{}, fmt.Errorf("marshal runtime metadata: %w", err)
	}
	row, err := s.queries.UpdateSessionTypeAndMetadata(ctx, sqlc.UpdateSessionTypeAndMetadataParams{
		ID:              pgID,
		Type:            sessionType,
		SessionMode:     desc.SessionMode,
		RuntimeType:     desc.RuntimeType,
		RuntimeMetadata: runtimeMetaBytes,
		Metadata:        metaBytes,
	})
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// Get returns a session by ID.
func (s *Service) Get(ctx context.Context, sessionID string) (Session, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	row, err := s.queries.GetSessionByID(ctx, pgID)
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// ListByBot returns all active sessions for a bot.
func (s *Service) ListByBot(ctx context.Context, botID string) ([]Session, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot id: %w", err)
	}
	rows, err := s.queries.ListSessionsByBot(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSessionFromListRow(row))
	}
	return sessions, nil
}

// SessionCursor identifies the position in a session listing for keyset
// pagination. The zero value means "start from the head".
type SessionCursor struct {
	UpdatedAt time.Time
	ID        string
}

type ListFilter struct {
	ParentSessionID string
}

// IsZero reports whether the cursor carries neither half — the start-of-list
// signal that pagedCursorParams maps to "no cursor predicate". A
// partially-populated cursor (only the timestamp or only the id) is not zero;
// pagedCursorParams rejects it as a programmer error so we never send
// malformed bindings down to SQL.
func (c SessionCursor) IsZero() bool {
	return c.ID == "" && c.UpdatedAt.IsZero()
}

// ListByBotPaged returns one page of sessions for a bot, filtered to the given
// types and starting after the given cursor. Callers that want a "has more"
// signal pass limit+1 and look for an extra row.
func (s *Service) ListByBotPaged(ctx context.Context, botID string, types []string, cursor SessionCursor, limit int64) ([]Session, error) {
	return s.ListByBotPagedWithFilter(ctx, botID, types, cursor, limit, ListFilter{})
}

func (s *Service) ListByBotPagedWithFilter(ctx context.Context, botID string, types []string, cursor SessionCursor, limit int64, filter ListFilter) ([]Session, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot id: %w", err)
	}
	parentSessionID, useParentSession, err := pagedParentSessionParam(filter)
	if err != nil {
		return nil, err
	}
	cursorUpdatedAt, cursorID, useCursor, err := pagedCursorParams(cursor)
	if err != nil {
		return nil, err
	}
	limitParam, err := pagedLimitToInt32(limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSessionsByBotPaged(ctx, sqlc.ListSessionsByBotPagedParams{
		BotID:            pgBotID,
		Types:            types,
		UseParentSession: useParentSession,
		ParentSessionID:  parentSessionID,
		UseCursor:        useCursor,
		CursorUpdatedAt:  cursorUpdatedAt,
		CursorID:         cursorID,
		LimitCount:       limitParam,
	})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSessionFromPagedRow(row))
	}
	return sessions, nil
}

// ListByBotAndCreatedByUserPaged is the paged variant scoped to a single user.
func (s *Service) ListByBotAndCreatedByUserPaged(ctx context.Context, botID, userID string, types []string, cursor SessionCursor, limit int64) ([]Session, error) {
	return s.ListByBotAndCreatedByUserPagedWithFilter(ctx, botID, userID, types, cursor, limit, ListFilter{})
}

func (s *Service) ListByBotAndCreatedByUserPagedWithFilter(ctx context.Context, botID, userID string, types []string, cursor SessionCursor, limit int64, filter ListFilter) ([]Session, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot id: %w", err)
	}
	pgUserID, err := dbpkg.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	parentSessionID, useParentSession, err := pagedParentSessionParam(filter)
	if err != nil {
		return nil, err
	}
	cursorUpdatedAt, cursorID, useCursor, err := pagedCursorParams(cursor)
	if err != nil {
		return nil, err
	}
	limitParam, err := pagedLimitToInt32(limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSessionsByBotAndCreatedByUserPaged(ctx, sqlc.ListSessionsByBotAndCreatedByUserPagedParams{
		BotID:            pgBotID,
		CreatedByUserID:  pgUserID,
		Types:            types,
		UseParentSession: useParentSession,
		ParentSessionID:  parentSessionID,
		UseCursor:        useCursor,
		CursorUpdatedAt:  cursorUpdatedAt,
		CursorID:         cursorID,
		LimitCount:       limitParam,
	})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSessionFromUserPagedRow(row))
	}
	return sessions, nil
}

// pagedLimitToInt32 narrows the int64 page-size that flows through the
// service signatures into the int32 sqlc binds. The handler caps the user-
// supplied limit at sessionListMaxLimit and bumps it by one for the
// has-more probe; both fit in int32 by construction, so any out-of-range value
// here is a programmer error and surfaces as such.
func pagedLimitToInt32(limit int64) (int32, error) {
	if limit < 1 || limit > math.MaxInt32 {
		return 0, fmt.Errorf("session: paged limit %d is out of range", limit)
	}
	return int32(limit), nil
}

func pagedParentSessionParam(filter ListFilter) (pgtype.UUID, bool, error) {
	parentID := strings.TrimSpace(filter.ParentSessionID)
	if parentID == "" {
		return pgtype.UUID{}, false, nil
	}
	parsed, err := dbpkg.ParseUUID(parentID)
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("invalid parent session id: %w", err)
	}
	return parsed, true, nil
}

func pagedCursorParams(cursor SessionCursor) (pgtype.Timestamptz, pgtype.UUID, bool, error) {
	if cursor.IsZero() {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, nil
	}
	if cursor.ID == "" || cursor.UpdatedAt.IsZero() {
		// The handler-side decoder rejects half-built cursors as 400, so by the
		// time we get here a partial cursor is an internal-construction bug.
		// Surface it loudly rather than silently restarting from the head and
		// returning a duplicate page.
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, errors.New("session: cursor must carry both updated_at and id")
	}
	pgID, err := dbpkg.ParseUUID(cursor.ID)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, fmt.Errorf("invalid cursor id: %w", err)
	}
	return pgtype.Timestamptz{Time: cursor.UpdatedAt, Valid: true}, pgID, true, nil
}

// ListByBotAndCreatedByUser returns all active sessions for a bot created by a user.
func (s *Service) ListByBotAndCreatedByUser(ctx context.Context, botID, userID string) ([]Session, error) {
	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, fmt.Errorf("invalid bot id: %w", err)
	}
	pgUserID, err := dbpkg.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	rows, err := s.queries.ListSessionsByBotAndCreatedByUser(ctx, sqlc.ListSessionsByBotAndCreatedByUserParams{
		BotID:           pgBotID,
		CreatedByUserID: pgUserID,
	})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSessionFromUserListRow(row))
	}
	return sessions, nil
}

// ListByRoute returns all active sessions for a route.
func (s *Service) ListByRoute(ctx context.Context, routeID string) ([]Session, error) {
	pgRouteID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return nil, fmt.Errorf("invalid route id: %w", err)
	}
	rows, err := s.queries.ListSessionsByRoute(ctx, pgRouteID)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

// ListSubagentsByParent returns active subagent sessions created under a
// parent session.
func (s *Service) ListSubagentsByParent(ctx context.Context, parentSessionID string) ([]Session, error) {
	pgParentSessionID, err := dbpkg.ParseUUID(parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid parent session id: %w", err)
	}
	rows, err := s.queries.ListSubagentSessionsByParent(ctx, pgParentSessionID)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

// GetActiveForRoute returns the active session for a route.
func (s *Service) GetActiveForRoute(ctx context.Context, routeID string) (Session, error) {
	pgRouteID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid route id: %w", err)
	}
	row, err := s.queries.GetActiveSessionForRoute(ctx, pgRouteID)
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// UpdateTitle updates a session's title.
func (s *Service) UpdateTitle(ctx context.Context, sessionID, title string) (Session, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	var row sqlc.BotSession
	if fence, fenced := runtimefence.FromContext(ctx); fenced {
		if strings.TrimSpace(sessionID) != fence.SessionID {
			return Session{}, runtimefence.ErrStale
		}
		writer, ok := s.queries.(runtimeFencedSessionWriter)
		if !ok {
			return Session{}, errors.New("session store does not support runtime fencing")
		}
		pgBotID, parseErr := dbpkg.ParseUUID(fence.BotID)
		if parseErr != nil {
			return Session{}, fmt.Errorf("invalid runtime fence bot id: %w", parseErr)
		}
		row, err = writer.UpdateSessionTitleWithRuntimeFence(ctx, sqlc.UpdateSessionTitleWithRuntimeFenceParams{
			Title:               title,
			ID:                  pgID,
			BotID:               pgBotID,
			RuntimeFencingToken: fence.Token,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, runtimefence.ErrStale
		}
	} else {
		row, err = s.queries.UpdateSessionTitle(ctx, sqlc.UpdateSessionTitleParams{
			ID:    pgID,
			Title: title,
		})
	}
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// UpdateMetadata updates a session's metadata.
func (s *Service) UpdateMetadata(ctx context.Context, sessionID string, metadata map[string]any) (Session, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return Session{}, fmt.Errorf("marshal metadata: %w", err)
	}
	var row sqlc.BotSession
	if fence, fenced := runtimefence.FromContext(ctx); fenced {
		if strings.TrimSpace(sessionID) != fence.SessionID {
			return Session{}, runtimefence.ErrStale
		}
		writer, ok := s.queries.(runtimeFencedSessionWriter)
		if !ok {
			return Session{}, errors.New("session store does not support runtime fencing")
		}
		pgBotID, parseErr := dbpkg.ParseUUID(fence.BotID)
		if parseErr != nil {
			return Session{}, fmt.Errorf("invalid runtime fence bot id: %w", parseErr)
		}
		row, err = writer.UpdateSessionMetadataWithRuntimeFence(ctx, sqlc.UpdateSessionMetadataWithRuntimeFenceParams{
			Metadata:            metaBytes,
			ID:                  pgID,
			BotID:               pgBotID,
			RuntimeFencingToken: fence.Token,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, runtimefence.ErrStale
		}
	} else {
		row, err = s.queries.UpdateSessionMetadata(ctx, sqlc.UpdateSessionMetadataParams{
			ID:       pgID,
			Metadata: metaBytes,
		})
	}
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// SoftDelete marks a session as deleted.
func (s *Service) SoftDelete(ctx context.Context, sessionID string) error {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	return s.queries.SoftDeleteSession(ctx, pgID)
}

func (s *Service) MessageCount(ctx context.Context, sessionID string) (int64, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return 0, fmt.Errorf("invalid session id: %w", err)
	}
	return s.queries.CountMessagesBySession(ctx, pgID)
}

// Touch updates a session's updated_at timestamp.
func (s *Service) Touch(ctx context.Context, sessionID string) error {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	return s.queries.TouchSession(ctx, pgID)
}

// SetRouteActiveSession sets the active session for a route.
func (s *Service) SetRouteActiveSession(ctx context.Context, routeID, sessionID string) error {
	pgRouteID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return fmt.Errorf("invalid route id: %w", err)
	}
	pgSessionID, err := parseOptionalUUID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session id: %w", err)
	}
	return s.queries.SetRouteActiveSession(ctx, sqlc.SetRouteActiveSessionParams{
		ID:              pgRouteID,
		ActiveSessionID: pgSessionID,
	})
}

// CreateNewSession always creates a fresh session and sets it as the active
// session for the given route, replacing any previous active session.
// sessionType defaults to TypeChat if empty.
func (s *Service) CreateNewSession(ctx context.Context, botID, routeID, channelType, sessionType string) (Session, error) {
	if strings.TrimSpace(sessionType) == "" {
		sessionType = TypeChat
	}
	return s.CreateNewSessionWithInput(ctx, CreateInput{
		BotID:       botID,
		RouteID:     routeID,
		ChannelType: channelType,
		Type:        sessionType,
	})
}

// CreateNewSessionWithInput creates a fresh active route session from a full
// CreateInput. The route and bot identity are taken from input, so callers can
// pass ACP metadata and descriptor fields without overloading the legacy type.
func (s *Service) CreateNewSessionWithInput(ctx context.Context, input CreateInput) (Session, error) {
	if strings.TrimSpace(input.Type) == "" {
		input.Type = TypeChat
	}
	sess, err := s.Create(ctx, input)
	if err != nil {
		return Session{}, fmt.Errorf("create new session: %w", err)
	}

	if err := s.SetRouteActiveSession(ctx, input.RouteID, sess.ID); err != nil {
		s.logger.Warn("failed to set active session on route", slog.Any("error", err))
	}
	return sess, nil
}

// EnsureActiveSession returns the active session for a route, creating one if it doesn't exist.
func (s *Service) EnsureActiveSession(ctx context.Context, botID, routeID, channelType string) (Session, error) {
	sess, err := s.GetActiveForRoute(ctx, routeID)
	if err == nil {
		return sess, nil
	}

	sess, err = s.Create(ctx, CreateInput{
		BotID:       botID,
		RouteID:     routeID,
		ChannelType: channelType,
	})
	if err != nil {
		return Session{}, fmt.Errorf("auto-create session: %w", err)
	}

	if err := s.SetRouteActiveSession(ctx, routeID, sess.ID); err != nil {
		s.logger.Warn("failed to set active session on route", slog.Any("error", err))
	}
	return sess, nil
}

func toSession(row sqlc.BotSession) Session {
	parentID := ""
	if row.ParentSessionID.Valid {
		parentID = row.ParentSessionID.String()
	}
	createdByUserID := ""
	if row.CreatedByUserID.Valid {
		createdByUserID = row.CreatedByUserID.String()
	}
	return Session{
		ID:              row.ID.String(),
		BotID:           row.BotID.String(),
		RouteID:         row.RouteID.String(),
		ChannelType:     dbpkg.TextToString(row.ChannelType),
		Type:            row.Type,
		SessionMode:     normalizeSessionMode(row.SessionMode, row.Type),
		RuntimeType:     normalizeRuntimeType(row.RuntimeType, row.Type),
		RuntimeMetadata: parseJSONMap(row.RuntimeMetadata),
		Title:           row.Title,
		Metadata:        parseJSONMap(row.Metadata),
		ParentSessionID: parentID,
		CreatedByUserID: createdByUserID,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toSessionFromForkRow(row sqlc.ForkSessionFromAssistantMessageRow) Session {
	return toSession(sqlc.BotSession(row))
}

func validateACPMetadata(meta map[string]any) error {
	if strings.TrimSpace(metadataString(meta, "acp_agent_id")) == "" {
		return ErrACPAgentIDRequired
	}
	if strings.TrimSpace(metadataString(meta, "project_path")) == "" {
		return ErrACPProjectPathMissing
	}
	if strings.TrimSpace(metadataString(meta, "runtime_owner_account_id")) == "" {
		return ErrACPRuntimeOwnerMissing
	}
	switch strings.TrimSpace(metadataString(meta, "acp_project_mode")) {
	case "", DefaultACPProjectMode, "none":
	default:
		return fmt.Errorf("%w %q", ErrACPProjectModeInvalid, metadataString(meta, "acp_project_mode"))
	}
	return nil
}

// ApplyACPMetadataDefaults fills omitted ACP session project fields.
func ApplyACPMetadataDefaults(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta)+2)
	for key, value := range meta {
		out[key] = value
	}
	if strings.TrimSpace(metadataString(out, "project_path")) == "" {
		out["project_path"] = DefaultACPProjectPath
	}
	if strings.TrimSpace(metadataString(out, "acp_project_mode")) == "" {
		out["acp_project_mode"] = DefaultACPProjectMode
	}
	return out
}

type descriptor struct {
	LegacyType      string
	SessionMode     string
	RuntimeType     string
	Metadata        map[string]any
	RuntimeMetadata map[string]any
}

// ResolveDescriptor returns the normalized compatibility type plus split
// session-mode/runtime descriptor without applying metadata side effects.
func ResolveDescriptor(legacyType, sessionMode, runtimeType string) (string, string, string, error) {
	// Reject a contradictory request body up front: the legacy acp_agent type
	// unambiguously means an ACP runtime, so an explicit non-ACP runtime_type
	// alongside it must fail loudly rather than silently degrade to a plain
	// model chat session.
	if rt := strings.TrimSpace(runtimeType); strings.TrimSpace(legacyType) == TypeACPAgent && rt != "" && rt != RuntimeACPAgent {
		return "", "", "", fmt.Errorf("session type %q conflicts with runtime_type %q", TypeACPAgent, rt)
	}
	desc, err := normalizeDescriptor(legacyType, sessionMode, runtimeType, nil, nil)
	if err != nil {
		return "", "", "", err
	}
	return desc.LegacyType, desc.SessionMode, desc.RuntimeType, nil
}

func normalizeDescriptor(legacyType, sessionMode, runtimeType string, metadata, runtimeMetadata map[string]any) (descriptor, error) {
	legacyType = strings.TrimSpace(legacyType)
	sessionMode = strings.TrimSpace(sessionMode)
	runtimeType = strings.TrimSpace(runtimeType)
	if legacyType == "" {
		legacyType = TypeChat
	}
	if sessionMode == "" || runtimeType == "" {
		derivedMode, derivedRuntime := descriptorFromLegacyType(legacyType)
		if sessionMode == "" {
			sessionMode = derivedMode
		}
		if runtimeType == "" {
			runtimeType = derivedRuntime
		}
	}
	if !IsKnownSessionMode(sessionMode) {
		return descriptor{}, fmt.Errorf("unknown session mode %q", sessionMode)
	}
	if !IsKnownRuntimeType(runtimeType) {
		return descriptor{}, fmt.Errorf("unknown runtime type %q", runtimeType)
	}
	if runtimeType == RuntimeACPAgent && sessionMode != TypeChat && sessionMode != TypeDiscuss {
		return descriptor{}, fmt.Errorf("runtime type %q is only supported for %s or %s session modes", RuntimeACPAgent, TypeChat, TypeDiscuss)
	}
	out := descriptor{
		LegacyType:      legacyTypeForDescriptor(sessionMode, runtimeType),
		SessionMode:     sessionMode,
		RuntimeType:     runtimeType,
		Metadata:        nonNilMap(metadata),
		RuntimeMetadata: nonNilMap(runtimeMetadata),
	}
	if runtimeType == RuntimeACPAgent {
		out.RuntimeMetadata = mergeACPMetadata(out.RuntimeMetadata, out.Metadata)
		out.Metadata = mergeACPMetadata(out.Metadata, out.RuntimeMetadata)
	}
	return out, nil
}

func descriptorFromLegacyType(typ string) (string, string) {
	switch strings.TrimSpace(typ) {
	case TypeACPAgent:
		return TypeChat, RuntimeACPAgent
	case TypeDiscuss:
		return TypeDiscuss, RuntimeModel
	case TypeHeartbeat:
		return TypeHeartbeat, RuntimeModel
	case TypeSchedule:
		return TypeSchedule, RuntimeModel
	case TypeSubagent:
		return TypeSubagent, RuntimeModel
	default:
		return TypeChat, RuntimeModel
	}
}

// DescriptorFromLegacyType maps the legacy single type column to the split
// session mode/runtime descriptor used by new code paths.
func DescriptorFromLegacyType(typ string) (string, string) {
	return descriptorFromLegacyType(typ)
}

func legacyTypeForDescriptor(sessionMode, runtimeType string) string {
	if runtimeType == RuntimeACPAgent && sessionMode == TypeChat {
		return TypeACPAgent
	}
	return sessionMode
}

// LegacyTypeForDescriptor returns the compatibility type value for a split
// session descriptor.
func LegacyTypeForDescriptor(sessionMode, runtimeType string) string {
	return legacyTypeForDescriptor(sessionMode, runtimeType)
}

func IsKnownSessionMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case TypeChat, TypeDiscuss, TypeHeartbeat, TypeSchedule, TypeSubagent:
		return true
	default:
		return false
	}
}

func IsKnownRuntimeType(runtimeType string) bool {
	switch strings.TrimSpace(runtimeType) {
	case RuntimeModel, RuntimeACPAgent:
		return true
	default:
		return false
	}
}

// IsACPRuntime reports whether a session is backed by an ACP runtime. It keeps
// legacy chat ACP sessions (`type=acp_agent`) working while allowing newer
// descriptors such as `session_mode=discuss` + `runtime_type=acp_agent`.
func IsACPRuntime(sess Session) bool {
	return normalizeRuntimeType(sess.RuntimeType, sess.Type) == RuntimeACPAgent
}

// SupportsSkillActivation reports whether a session shape can accept
// user-requested skill activation: chat mode (with the usual legacy-type
// fallback) on the built-in model runtime. This is the single definition for
// the rule — the web WS pre-check, the channel inbound gate, and the flow
// resolver's final guard all call it, so the three surfaces cannot drift.
func SupportsSkillActivation(sessionMode, legacyType, runtimeType string) bool {
	return normalizeSessionMode(sessionMode, legacyType) == TypeChat &&
		normalizeRuntimeType(runtimeType, legacyType) != RuntimeACPAgent
}

func normalizeSessionMode(mode, legacyType string) string {
	if IsKnownSessionMode(mode) {
		return strings.TrimSpace(mode)
	}
	derived, _ := descriptorFromLegacyType(legacyType)
	return derived
}

func normalizeRuntimeType(runtimeType, legacyType string) string {
	if IsKnownRuntimeType(runtimeType) {
		return strings.TrimSpace(runtimeType)
	}
	_, derived := descriptorFromLegacyType(legacyType)
	return derived
}

func mergeACPMetadata(base, overlay map[string]any) map[string]any {
	out := nonNilMap(base)
	for _, key := range []string{"acp_agent_id", "project_path", "acp_project_mode", "runtime_owner_account_id"} {
		if value, ok := overlay[key]; ok {
			out[key] = value
		}
	}
	return out
}

func setACPRuntimeOwner(metadata map[string]any, ownerUserID string) map[string]any {
	out := nonNilMap(metadata)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		delete(out, "runtime_owner_account_id")
		return out
	}
	out["runtime_owner_account_id"] = ownerUserID
	return out
}

func nonNilMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (s *Service) validateACPCreatePolicy(ctx context.Context, botID pgtype.UUID, meta map[string]any) error {
	agentID := metadataString(meta, "acp_agent_id")
	profile, ok := acpprofile.Lookup(agentID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrACPUnknownAgent, agentID)
	}
	bot, err := s.queries.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	botMeta := parseJSONMap(bot.Metadata)
	setup := acpprofile.ParseAgentSetup(botMeta, agentID)
	if !setup.Enabled {
		return fmt.Errorf("%w: %s", ErrACPAgentNotEnabled, agentID)
	}
	if field, missing := acpprofile.MissingRequiredManagedFieldForPreflight(profile, setup); missing {
		return fmt.Errorf("%w: %s missing %s", ErrACPAgentNotConfigured, agentID, field.ID)
	}
	return nil
}

func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}

func parseOptionalUUID(id string) (pgtype.UUID, error) {
	if strings.TrimSpace(id) == "" {
		return pgtype.UUID{}, nil
	}
	return dbpkg.ParseUUID(id)
}

func parseJSONMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

func toSessionFromListRow(row sqlc.ListSessionsByBotRow) Session {
	parentID := ""
	if row.ParentSessionID.Valid {
		parentID = row.ParentSessionID.String()
	}
	createdByUserID := ""
	if row.CreatedByUserID.Valid {
		createdByUserID = row.CreatedByUserID.String()
	}
	return Session{
		ID:                    row.ID.String(),
		BotID:                 row.BotID.String(),
		RouteID:               row.RouteID.String(),
		ChannelType:           dbpkg.TextToString(row.ChannelType),
		Type:                  row.Type,
		SessionMode:           normalizeSessionMode(row.SessionMode, row.Type),
		RuntimeType:           normalizeRuntimeType(row.RuntimeType, row.Type),
		RuntimeMetadata:       parseJSONMap(row.RuntimeMetadata),
		Title:                 row.Title,
		Metadata:              parseJSONMap(row.Metadata),
		ParentSessionID:       parentID,
		CreatedByUserID:       createdByUserID,
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(row.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(row.RouteConversationType),
	}
}

func toSessionFromUserListRow(row sqlc.ListSessionsByBotAndCreatedByUserRow) Session {
	parentID := ""
	if row.ParentSessionID.Valid {
		parentID = row.ParentSessionID.String()
	}
	createdByUserID := ""
	if row.CreatedByUserID.Valid {
		createdByUserID = row.CreatedByUserID.String()
	}
	return Session{
		ID:                    row.ID.String(),
		BotID:                 row.BotID.String(),
		RouteID:               row.RouteID.String(),
		ChannelType:           dbpkg.TextToString(row.ChannelType),
		Type:                  row.Type,
		SessionMode:           normalizeSessionMode(row.SessionMode, row.Type),
		RuntimeType:           normalizeRuntimeType(row.RuntimeType, row.Type),
		RuntimeMetadata:       parseJSONMap(row.RuntimeMetadata),
		Title:                 row.Title,
		Metadata:              parseJSONMap(row.Metadata),
		ParentSessionID:       parentID,
		CreatedByUserID:       createdByUserID,
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(row.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(row.RouteConversationType),
	}
}

func toSessionFromPagedRow(row sqlc.ListSessionsByBotPagedRow) Session {
	return sessionFromPagedColumns(pagedColumns{
		ID: row.ID, BotID: row.BotID, RouteID: row.RouteID, ChannelType: row.ChannelType,
		Type: row.Type, SessionMode: row.SessionMode, RuntimeType: row.RuntimeType, RuntimeMetadata: row.RuntimeMetadata,
		Title: row.Title, Metadata: row.Metadata,
		ParentSessionID: row.ParentSessionID, CreatedByUserID: row.CreatedByUserID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		RouteMetadata: row.RouteMetadata, RouteConversationType: row.RouteConversationType,
	})
}

func toSessionFromUserPagedRow(row sqlc.ListSessionsByBotAndCreatedByUserPagedRow) Session {
	return sessionFromPagedColumns(pagedColumns{
		ID: row.ID, BotID: row.BotID, RouteID: row.RouteID, ChannelType: row.ChannelType,
		Type: row.Type, SessionMode: row.SessionMode, RuntimeType: row.RuntimeType, RuntimeMetadata: row.RuntimeMetadata,
		Title: row.Title, Metadata: row.Metadata,
		ParentSessionID: row.ParentSessionID, CreatedByUserID: row.CreatedByUserID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		RouteMetadata: row.RouteMetadata, RouteConversationType: row.RouteConversationType,
	})
}

// pagedColumns is the shared shape of the bot/route-joined session row
// returned by both paged list queries. The two sqlc-generated row structs
// happen to be structurally identical; centralizing the projection here
// keeps the conversion logic in one place.
type pagedColumns struct {
	ID                    pgtype.UUID
	BotID                 pgtype.UUID
	RouteID               pgtype.UUID
	ChannelType           pgtype.Text
	Type                  string
	SessionMode           string
	RuntimeType           string
	RuntimeMetadata       []byte
	Title                 string
	Metadata              []byte
	ParentSessionID       pgtype.UUID
	CreatedByUserID       pgtype.UUID
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	RouteMetadata         []byte
	RouteConversationType pgtype.Text
}

func sessionFromPagedColumns(c pagedColumns) Session {
	parentID := ""
	if c.ParentSessionID.Valid {
		parentID = c.ParentSessionID.String()
	}
	createdByUserID := ""
	if c.CreatedByUserID.Valid {
		createdByUserID = c.CreatedByUserID.String()
	}
	return Session{
		ID:                    c.ID.String(),
		BotID:                 c.BotID.String(),
		RouteID:               c.RouteID.String(),
		ChannelType:           dbpkg.TextToString(c.ChannelType),
		Type:                  c.Type,
		SessionMode:           normalizeSessionMode(c.SessionMode, c.Type),
		RuntimeType:           normalizeRuntimeType(c.RuntimeType, c.Type),
		RuntimeMetadata:       parseJSONMap(c.RuntimeMetadata),
		Title:                 c.Title,
		Metadata:              parseJSONMap(c.Metadata),
		ParentSessionID:       parentID,
		CreatedByUserID:       createdByUserID,
		CreatedAt:             c.CreatedAt.Time,
		UpdatedAt:             c.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(c.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(c.RouteConversationType),
	}
}
