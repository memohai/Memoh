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
	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/message/event"
)

// Session represents a chat session within a bot.
type Session struct {
	ID                    string         `json:"id"`
	BotID                 string         `json:"bot_id"`
	RouteID               string         `json:"route_id,omitempty"`
	ChannelType           string         `json:"channel_type,omitempty"`
	Type                  string         `json:"type"`
	Title                 string         `json:"title"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	DefaultHeadTurnID     string         `json:"default_head_turn_id,omitempty"`
	ForkedFromSessionID   string         `json:"forked_from_session_id,omitempty"`
	ForkedFromTurnID      string         `json:"forked_from_turn_id,omitempty"`
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
	ErrACPAgentIDRequired    = errors.New("acp_agent_id is required for acp_agent sessions")
	ErrACPProjectPathMissing = errors.New("project_path is required for acp_agent sessions")
	ErrACPUnknownAgent       = errors.New("unknown ACP agent")
	ErrACPAgentNotEnabled    = errors.New("ACP agent is not enabled for this bot")
	ErrForkSourceNotFound    = errors.New("fork source session not found")
	ErrForkSourceNotReply    = errors.New("fork source must be an assistant reply")
	ErrForkSourceNotChat     = errors.New("fork source must be a chat session")
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

// SupportsTurnVariants reports whether a session type may expose multiple
// selectable turn heads and history rewrite actions. Non-chat sessions still use
// the turn graph as storage, but they must remain linear at the product layer.
func SupportsTurnVariants(typ string) bool {
	return strings.TrimSpace(typ) == TypeChat
}

// CreateInput holds input for creating a new session.
type CreateInput struct {
	BotID               string
	RouteID             string
	ChannelType         string
	Type                string
	Title               string
	Metadata            map[string]any
	DefaultHeadTurnID   string
	ForkedFromSessionID string
	ForkedFromTurnID    string
	ParentSessionID     string
	CreatedByUserID     string
}

// ForkFromAssistantInput creates a new session whose head points at the turn
// that produced a visible assistant reply in the source session.
type ForkFromAssistantInput struct {
	BotID           string
	SessionID       string
	MessageID       string
	BaseHeadTurnID  string
	CreatedByUserID string
}

// Service manages bot chat sessions.
type Service struct {
	queries     dbstore.Queries
	hookService *hooks.Service
	publisher   event.Publisher
	logger      *slog.Logger
}

type sessionTxRunner interface {
	RunInTx(ctx context.Context, fn func(dbstore.Queries) error) error
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

	channelType := pgtype.Text{}
	if ct := strings.TrimSpace(input.ChannelType); ct != "" {
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
	if sessionType == TypeACPAgent {
		meta = ApplyACPMetadataDefaults(meta)
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

	pgParentSessionID, err := parseOptionalUUID(input.ParentSessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid parent session id: %w", err)
	}
	pgCreatedByUserID, err := parseOptionalUUID(input.CreatedByUserID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid created by user id: %w", err)
	}
	pgDefaultHeadTurnID, err := parseOptionalUUID(input.DefaultHeadTurnID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid default head turn id: %w", err)
	}
	pgForkedFromSessionID, err := parseOptionalUUID(input.ForkedFromSessionID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid forked from session id: %w", err)
	}
	pgForkedFromTurnID, err := parseOptionalUUID(input.ForkedFromTurnID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid forked from turn id: %w", err)
	}

	createParams := sqlc.CreateSessionParams{
		BotID:               pgBotID,
		RouteID:             pgRouteID,
		ChannelType:         channelType,
		Type:                sessionType,
		Title:               input.Title,
		Metadata:            metaBytes,
		DefaultHeadTurnID:   pgDefaultHeadTurnID,
		ForkedFromSessionID: pgForkedFromSessionID,
		ForkedFromTurnID:    pgForkedFromTurnID,
		ParentSessionID:     pgParentSessionID,
		CreatedByUserID:     pgCreatedByUserID,
	}

	create := func(queries dbstore.Queries) (sqlc.BotSession, error) {
		row, err := queries.CreateSession(ctx, createParams)
		if err != nil {
			return sqlc.BotSession{}, err
		}
		if pgDefaultHeadTurnID.Valid {
			if _, err := queries.CreateSessionTurnHead(ctx, sqlc.CreateSessionTurnHeadParams{
				SessionID:  row.ID,
				HeadTurnID: pgDefaultHeadTurnID,
			}); err != nil {
				return sqlc.BotSession{}, fmt.Errorf("create session turn head: %w", err)
			}
		}
		return row, nil
	}

	var row sqlc.BotSession
	if runner, ok := s.queries.(sessionTxRunner); ok && runner != nil {
		if err := runner.RunInTx(ctx, func(queries dbstore.Queries) error {
			created, err := create(queries)
			if err != nil {
				return err
			}
			row = created
			return nil
		}); err != nil {
			return Session{}, err
		}
	} else {
		var err error
		row, err = create(s.queries)
		if err != nil {
			return Session{}, err
		}
	}
	sess := toSession(row)
	s.publishSessionCreated(sess)
	s.runSessionStartHook(context.WithoutCancel(ctx), sess)
	return sess, nil
}

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
	pgBaseHeadTurnID, err := parseOptionalUUID(input.BaseHeadTurnID)
	if err != nil {
		return Session{}, fmt.Errorf("invalid base head turn id: %w", err)
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
	if !pgBaseHeadTurnID.Valid {
		pgBaseHeadTurnID = sourceRow.DefaultHeadTurnID
	}
	if !pgBaseHeadTurnID.Valid {
		return Session{}, ErrForkSourceNotReply
	}

	anchor, err := s.resolveForkTurnAnchor(ctx, pgSessionID, pgBaseHeadTurnID, pgMessageID)
	if err != nil {
		return Session{}, err
	}

	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "Untitled"
	}
	return s.Create(ctx, CreateInput{
		BotID:               source.BotID,
		ChannelType:         source.ChannelType,
		Type:                TypeChat,
		Title:               title + " fork",
		Metadata:            cloneMetadata(source.Metadata),
		DefaultHeadTurnID:   anchor.TurnID,
		ForkedFromSessionID: source.ID,
		ForkedFromTurnID:    anchor.TurnID,
		CreatedByUserID:     input.CreatedByUserID,
	})
}

func (s *Service) resolveForkTurnAnchor(ctx context.Context, sessionID, baseHeadTurnID, messageID pgtype.UUID) (conversation.TurnAnchor, error) {
	turn, err := s.queries.GetVisibleAssistantMessageTurnForFork(ctx, sqlc.GetVisibleAssistantMessageTurnForForkParams{
		MessageID:      messageID,
		BaseHeadTurnID: baseHeadTurnID,
		SessionID:      sessionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return conversation.TurnAnchor{}, ErrForkSourceNotReply
		}
		return conversation.TurnAnchor{}, err
	}
	if !turn.TurnID.Valid {
		return conversation.TurnAnchor{}, ErrForkSourceNotReply
	}
	return conversation.TurnAnchor{
		Role:           conversation.TurnAnchorRoleAssistant,
		MessageID:      turn.MessageID.String(),
		TurnID:         turn.TurnID.String(),
		ParentTurnID:   uuidString(turn.ParentTurnID),
		BaseHeadTurnID: baseHeadTurnID.String(),
	}, nil
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
	if sessionType == TypeACPAgent {
		metadata = ApplyACPMetadataDefaults(metadata)
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
	row, err := s.queries.UpdateSessionTypeAndMetadata(ctx, sqlc.UpdateSessionTypeAndMetadataParams{
		ID:       pgID,
		Type:     sessionType,
		Metadata: metaBytes,
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
	row, err := s.queries.UpdateSessionTitle(ctx, sqlc.UpdateSessionTitleParams{
		ID:    pgID,
		Title: title,
	})
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
	row, err := s.queries.UpdateSessionMetadata(ctx, sqlc.UpdateSessionMetadataParams{
		ID:       pgID,
		Metadata: metaBytes,
	})
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
	deleteSession := func(queries dbstore.Queries) error {
		if err := queries.SoftDeleteSession(ctx, pgID); err != nil {
			return err
		}
		return s.cleanupDeletedSessionTurns(ctx, queries, pgID)
	}
	if runner, ok := s.queries.(sessionTxRunner); ok && runner != nil {
		return runner.RunInTx(ctx, deleteSession)
	}
	return deleteSession(s.queries)
}

type sessionTurnCleanupQueries interface {
	DeleteSessionTurnHeads(context.Context, pgtype.UUID) error
	ListSessionOwnedTurnsForCleanup(context.Context, pgtype.UUID) ([]sqlc.BotHistoryTurn, error)
	ListOtherActiveSessionVisibleTurnIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error)
	DeleteMessagesByTurnID(context.Context, pgtype.UUID) error
	DeleteHistoryTurnByID(context.Context, pgtype.UUID) error
}

func (*Service) cleanupDeletedSessionTurns(ctx context.Context, rawQueries dbstore.Queries, sessionID pgtype.UUID) error {
	queries, ok := rawQueries.(sessionTurnCleanupQueries)
	if !ok {
		return nil
	}
	candidates, err := queries.ListSessionOwnedTurnsForCleanup(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list owned history turns for deleted session: %w", err)
	}
	if err := queries.DeleteSessionTurnHeads(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session turn heads: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}
	sharedRows, err := queries.ListOtherActiveSessionVisibleTurnIDs(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list shared history turns for deleted session: %w", err)
	}
	shared := make(map[pgtype.UUID]struct{}, len(sharedRows))
	for _, id := range sharedRows {
		if id.Valid {
			shared[id] = struct{}{}
		}
	}
	for _, turn := range candidates {
		if !turn.ID.Valid {
			continue
		}
		if _, ok := shared[turn.ID]; ok {
			continue
		}
		if err := queries.DeleteMessagesByTurnID(ctx, turn.ID); err != nil {
			return fmt.Errorf("delete messages for session history turn %s: %w", turn.ID.String(), err)
		}
		if err := queries.DeleteHistoryTurnByID(ctx, turn.ID); err != nil {
			return fmt.Errorf("delete session history turn %s: %w", turn.ID.String(), err)
		}
	}
	return nil
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
	sess, err := s.Create(ctx, CreateInput{
		BotID:       botID,
		RouteID:     routeID,
		ChannelType: channelType,
		Type:        sessionType,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create new session: %w", err)
	}

	if err := s.SetRouteActiveSession(ctx, routeID, sess.ID); err != nil {
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
	defaultHeadTurnID := ""
	if row.DefaultHeadTurnID.Valid {
		defaultHeadTurnID = row.DefaultHeadTurnID.String()
	}
	forkedFromSessionID := ""
	if row.ForkedFromSessionID.Valid {
		forkedFromSessionID = row.ForkedFromSessionID.String()
	}
	forkedFromTurnID := ""
	if row.ForkedFromTurnID.Valid {
		forkedFromTurnID = row.ForkedFromTurnID.String()
	}
	parentID := ""
	if row.ParentSessionID.Valid {
		parentID = row.ParentSessionID.String()
	}
	createdByUserID := ""
	if row.CreatedByUserID.Valid {
		createdByUserID = row.CreatedByUserID.String()
	}
	return Session{
		ID:                  row.ID.String(),
		BotID:               row.BotID.String(),
		RouteID:             row.RouteID.String(),
		ChannelType:         dbpkg.TextToString(row.ChannelType),
		Type:                row.Type,
		Title:               row.Title,
		Metadata:            parseJSONMap(row.Metadata),
		DefaultHeadTurnID:   defaultHeadTurnID,
		ForkedFromSessionID: forkedFromSessionID,
		ForkedFromTurnID:    forkedFromTurnID,
		ParentSessionID:     parentID,
		CreatedByUserID:     createdByUserID,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func validateACPMetadata(meta map[string]any) error {
	if strings.TrimSpace(metadataString(meta, "acp_agent_id")) == "" {
		return ErrACPAgentIDRequired
	}
	if strings.TrimSpace(metadataString(meta, "project_path")) == "" {
		return ErrACPProjectPathMissing
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

func (s *Service) validateACPCreatePolicy(ctx context.Context, botID pgtype.UUID, meta map[string]any) error {
	agentID := metadataString(meta, "acp_agent_id")
	if _, ok := acpprofile.Lookup(agentID); !ok {
		return fmt.Errorf("%w: %s", ErrACPUnknownAgent, agentID)
	}
	bot, err := s.queries.GetBotByID(ctx, botID)
	if err != nil {
		return err
	}
	botMeta := parseJSONMap(bot.Metadata)
	if !acpprofile.MetadataAgentEnabled(botMeta, agentID) {
		return fmt.Errorf("%w: %s", ErrACPAgentNotEnabled, agentID)
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

func cloneMetadata(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]any, len(meta))
	for key, value := range meta {
		out[key] = value
	}
	return out
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
	defaultHeadTurnID := ""
	if row.DefaultHeadTurnID.Valid {
		defaultHeadTurnID = row.DefaultHeadTurnID.String()
	}
	forkedFromSessionID := ""
	if row.ForkedFromSessionID.Valid {
		forkedFromSessionID = row.ForkedFromSessionID.String()
	}
	forkedFromTurnID := ""
	if row.ForkedFromTurnID.Valid {
		forkedFromTurnID = row.ForkedFromTurnID.String()
	}
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
		Title:                 row.Title,
		Metadata:              parseJSONMap(row.Metadata),
		DefaultHeadTurnID:     defaultHeadTurnID,
		ForkedFromSessionID:   forkedFromSessionID,
		ForkedFromTurnID:      forkedFromTurnID,
		ParentSessionID:       parentID,
		CreatedByUserID:       createdByUserID,
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(row.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(row.RouteConversationType),
	}
}

func toSessionFromUserListRow(row sqlc.ListSessionsByBotAndCreatedByUserRow) Session {
	defaultHeadTurnID := ""
	if row.DefaultHeadTurnID.Valid {
		defaultHeadTurnID = row.DefaultHeadTurnID.String()
	}
	forkedFromSessionID := ""
	if row.ForkedFromSessionID.Valid {
		forkedFromSessionID = row.ForkedFromSessionID.String()
	}
	forkedFromTurnID := ""
	if row.ForkedFromTurnID.Valid {
		forkedFromTurnID = row.ForkedFromTurnID.String()
	}
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
		Title:                 row.Title,
		Metadata:              parseJSONMap(row.Metadata),
		DefaultHeadTurnID:     defaultHeadTurnID,
		ForkedFromSessionID:   forkedFromSessionID,
		ForkedFromTurnID:      forkedFromTurnID,
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
		Type: row.Type, Title: row.Title, Metadata: row.Metadata,
		DefaultHeadTurnID: row.DefaultHeadTurnID, ForkedFromSessionID: row.ForkedFromSessionID, ForkedFromTurnID: row.ForkedFromTurnID,
		ParentSessionID: row.ParentSessionID, CreatedByUserID: row.CreatedByUserID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		RouteMetadata: row.RouteMetadata, RouteConversationType: row.RouteConversationType,
	})
}

func toSessionFromUserPagedRow(row sqlc.ListSessionsByBotAndCreatedByUserPagedRow) Session {
	return sessionFromPagedColumns(pagedColumns{
		ID: row.ID, BotID: row.BotID, RouteID: row.RouteID, ChannelType: row.ChannelType,
		Type: row.Type, Title: row.Title, Metadata: row.Metadata,
		DefaultHeadTurnID: row.DefaultHeadTurnID, ForkedFromSessionID: row.ForkedFromSessionID, ForkedFromTurnID: row.ForkedFromTurnID,
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
	Title                 string
	Metadata              []byte
	ParentSessionID       pgtype.UUID
	CreatedByUserID       pgtype.UUID
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	RouteMetadata         []byte
	RouteConversationType pgtype.Text
	DefaultHeadTurnID     pgtype.UUID
	ForkedFromSessionID   pgtype.UUID
	ForkedFromTurnID      pgtype.UUID
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
		Title:                 c.Title,
		Metadata:              parseJSONMap(c.Metadata),
		DefaultHeadTurnID:     uuidString(c.DefaultHeadTurnID),
		ForkedFromSessionID:   uuidString(c.ForkedFromSessionID),
		ForkedFromTurnID:      uuidString(c.ForkedFromTurnID),
		ParentSessionID:       parentID,
		CreatedByUserID:       createdByUserID,
		CreatedAt:             c.CreatedAt.Time,
		UpdatedAt:             c.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(c.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(c.RouteConversationType),
	}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}
