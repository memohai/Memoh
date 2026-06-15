package session

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

	"github.com/memohai/memoh/internal/acpprofile"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/hooks"
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
	ParentSessionID       string         `json:"parent_session_id,omitempty"`
	CreatedByUserID       string         `json:"created_by_user_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	RouteMetadata         map[string]any `json:"route_metadata,omitempty"`
	RouteConversationType string         `json:"route_conversation_type,omitempty"`
}

// BranchGraph is the session branch graph returned to the Web UI.
type BranchGraph struct {
	ActiveBranchID string       `json:"active_branch_id,omitempty"`
	Branches       []BranchNode `json:"branches"`
	Turns          []BranchTurn `json:"turns"`
}

// BranchNode is a single in-session branch node.
type BranchNode struct {
	ID                string      `json:"id"`
	SessionID         string      `json:"session_id"`
	ParentBranchID    string      `json:"parent_branch_id,omitempty"`
	ForkFromMessageID string      `json:"fork_from_message_id,omitempty"`
	ForkFromSeq       int64       `json:"fork_from_seq,omitempty"`
	ForkFromTurnID    string      `json:"fork_from_turn_id,omitempty"`
	ForkFromTurnSeq   int64       `json:"fork_from_turn_seq,omitempty"`
	Title             string      `json:"title,omitempty"`
	Active            bool        `json:"active"`
	Preview           TurnPreview `json:"preview"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// BranchTurn is one request/reply card in the branch graph.
type BranchTurn struct {
	ID           string      `json:"id"`
	BranchID     string      `json:"branch_id"`
	ParentTurnID string      `json:"parent_turn_id,omitempty"`
	Title        string      `json:"title,omitempty"`
	AssistantID  string      `json:"assistant_message_id,omitempty"`
	UserID       string      `json:"user_message_id,omitempty"`
	BranchSeq    int64       `json:"branch_seq,omitempty"`
	TurnSeq      int64       `json:"turn_seq,omitempty"`
	Depth        int         `json:"depth"`
	Active       bool        `json:"active"`
	Preview      TurnPreview `json:"preview"`
	ForkFromSeq  int64       `json:"fork_from_seq,omitempty"`
	CreatedAt    time.Time   `json:"created_at,omitempty"`
}

// TurnPreview summarizes one user request + assistant reply turn.
type TurnPreview struct {
	UserText      string    `json:"user_text,omitempty"`
	AssistantText string    `json:"assistant_text,omitempty"`
	MessageID     string    `json:"message_id,omitempty"`
	Timestamp     time.Time `json:"timestamp,omitempty"`
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

var (
	ErrACPAgentIDRequired    = errors.New("acp_agent_id is required for acp_agent sessions")
	ErrACPProjectPathMissing = errors.New("project_path is required for acp_agent sessions")
	ErrACPUnknownAgent       = errors.New("unknown ACP agent")
	ErrACPAgentNotEnabled    = errors.New("ACP agent is not enabled for this bot")
	ErrBranchNotFound        = errors.New("session branch not found")
	ErrForkMessageNotFound   = errors.New("fork source message not found")
	ErrForkSourceUnsupported = errors.New("fork source message must be a user or assistant message")
)

func IsKnownType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case TypeChat, TypeHeartbeat, TypeSchedule, TypeSubagent, TypeDiscuss, TypeACPAgent:
		return true
	default:
		return false
	}
}

// CreateInput holds input for creating a new session.
type CreateInput struct {
	BotID           string
	RouteID         string
	ChannelType     string
	Type            string
	Title           string
	Metadata        map[string]any
	ParentSessionID string
	CreatedByUserID string
}

// Service manages bot chat sessions.
type Service struct {
	queries     dbstore.Queries
	hookService *hooks.Service
	logger      *slog.Logger
}

// NewService creates a session service.
func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "session")),
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

	row, err := s.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		BotID:           pgBotID,
		RouteID:         pgRouteID,
		ChannelType:     channelType,
		Type:            sessionType,
		Title:           input.Title,
		Metadata:        metaBytes,
		ParentSessionID: pgParentSessionID,
		CreatedByUserID: pgCreatedByUserID,
	})
	if err != nil {
		return Session{}, err
	}
	sess := toSession(row)
	s.runSessionStartHook(context.WithoutCancel(ctx), sess)
	return sess, nil
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
	if _, err := s.hookService.Run(ctx, req, nil); err != nil && s.logger != nil {
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
	return s.queries.SoftDeleteSession(ctx, pgID)
}

func (s *Service) MessageCount(ctx context.Context, sessionID string) (int64, error) {
	pgID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return 0, fmt.Errorf("invalid session id: %w", err)
	}
	return s.queries.CountMessagesBySession(ctx, pgID)
}

// ListBranches returns the session branch graph and turn previews.
func (s *Service) ListBranches(ctx context.Context, sessionID string) (BranchGraph, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("invalid session id: %w", err)
	}
	if _, err := s.ensureActiveBranchForSession(ctx, pgSessionID); err != nil {
		return BranchGraph{}, fmt.Errorf("ensure session branch: %w", err)
	}
	return s.listBranchesByUUID(ctx, pgSessionID)
}

// ForkBranchFromMessage creates and activates a new session branch from a reply
// or from a request that should be edited and resent.
func (s *Service) ForkBranchFromMessage(ctx context.Context, sessionID, messageID string) (BranchGraph, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("invalid session id: %w", err)
	}
	pgMessageID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("invalid message id: %w", err)
	}
	if _, err := s.ensureActiveBranchForSession(ctx, pgSessionID); err != nil {
		return BranchGraph{}, fmt.Errorf("ensure session branch: %w", err)
	}
	row, err := s.queries.GetMessageForSessionBranchFork(ctx, sqlc.GetMessageForSessionBranchForkParams{
		MessageID: pgMessageID,
		SessionID: pgSessionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchGraph{}, ErrForkMessageNotFound
	}
	if err != nil {
		return BranchGraph{}, err
	}
	if !row.BranchID.Valid || !row.TurnID.Valid {
		return BranchGraph{}, errors.New("fork source message has no turn position")
	}
	forkFromTurnID := row.TurnID
	forkFromTurnSeq := pgtype.Int8{Int64: row.TurnSeq, Valid: true}
	forkFromSeq := int64(0)
	if row.BranchSeq.Valid {
		forkFromSeq = row.BranchSeq.Int64
	}
	forkFromSeqValid := row.BranchSeq.Valid
	switch row.Role {
	case "assistant":
	case "user":
		forkFromTurnID = row.PreviousTurnID
		forkFromTurnSeq = pgtype.Int8{Int64: 0, Valid: true}
		if row.PreviousTurnID.Valid {
			forkFromTurnSeq.Int64 = row.PreviousTurnSeq
		}
		forkFromSeq = 0
		forkFromSeqValid = true
		if row.PreviousBranchSeq.Valid {
			forkFromSeq = row.PreviousBranchSeq.Int64
		}
	default:
		return BranchGraph{}, ErrForkSourceUnsupported
	}
	branchID, err := s.queries.CreateSessionBranchFromMessage(ctx, sqlc.CreateSessionBranchFromMessageParams{
		SessionID:         pgSessionID,
		ParentBranchID:    row.BranchID,
		ForkFromMessageID: row.ID,
		ForkFromSeq:       pgtype.Int8{Int64: forkFromSeq, Valid: forkFromSeqValid},
		ForkFromTurnID:    forkFromTurnID,
		ForkFromTurnSeq:   forkFromTurnSeq,
		Title:             pgtype.Text{},
	})
	if err != nil {
		return BranchGraph{}, err
	}
	if err := s.queries.SetActiveSessionBranch(ctx, sqlc.SetActiveSessionBranchParams{
		SessionID: pgSessionID,
		BranchID:  branchID,
	}); err != nil {
		return BranchGraph{}, err
	}
	return s.listBranchesByUUID(ctx, pgSessionID)
}

// SetActiveBranch switches the active branch for a session.
func (s *Service) SetActiveBranch(ctx context.Context, sessionID, branchID string) (BranchGraph, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("invalid session id: %w", err)
	}
	pgBranchID, err := dbpkg.ParseUUID(branchID)
	if err != nil {
		return BranchGraph{}, fmt.Errorf("invalid branch id: %w", err)
	}
	rows, err := s.queries.ListSessionBranches(ctx, pgSessionID)
	if err != nil {
		return BranchGraph{}, err
	}
	found := false
	for _, row := range rows {
		if row.ID == pgBranchID {
			found = true
			break
		}
	}
	if !found {
		return BranchGraph{}, ErrBranchNotFound
	}
	if err := s.queries.SetActiveSessionBranch(ctx, sqlc.SetActiveSessionBranchParams{
		SessionID: pgSessionID,
		BranchID:  pgBranchID,
	}); err != nil {
		return BranchGraph{}, err
	}
	return s.listBranchesByUUID(ctx, pgSessionID)
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

func (s *Service) ensureActiveBranchForSession(ctx context.Context, sessionID pgtype.UUID) (pgtype.UUID, error) {
	branchID, err := s.queries.GetActiveSessionBranch(ctx, sessionID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if branchID.Valid {
		return branchID, nil
	}

	branchID, err = s.queries.GetRootSessionBranch(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		branchID, err = s.queries.CreateRootSessionBranch(ctx, sessionID)
		if err != nil {
			if existing, getErr := s.queries.GetRootSessionBranch(ctx, sessionID); getErr == nil {
				branchID = existing
			} else {
				return pgtype.UUID{}, err
			}
		}
	} else if err != nil {
		return pgtype.UUID{}, err
	}

	if err := s.queries.SetActiveSessionBranch(ctx, sqlc.SetActiveSessionBranchParams{
		SessionID: sessionID,
		BranchID:  branchID,
	}); err != nil {
		return pgtype.UUID{}, err
	}
	return branchID, nil
}

func (s *Service) listBranchesByUUID(ctx context.Context, sessionID pgtype.UUID) (BranchGraph, error) {
	rows, err := s.queries.ListSessionBranches(ctx, sessionID)
	if err != nil {
		return BranchGraph{}, err
	}
	previews, err := s.queries.ListSessionBranchPreviewMessages(ctx, sessionID)
	if err != nil {
		return BranchGraph{}, err
	}
	turnRows, err := s.queries.ListSessionBranchTurnMessages(ctx, sessionID)
	if err != nil {
		return BranchGraph{}, err
	}
	previewByBranch := buildBranchPreviewMap(previews)
	graph := BranchGraph{Branches: make([]BranchNode, 0, len(rows))}
	for _, row := range rows {
		node := toBranchNode(row)
		if row.ActiveBranchID.Valid {
			graph.ActiveBranchID = row.ActiveBranchID.String()
			node.Active = row.ID == row.ActiveBranchID
		}
		if preview, ok := previewByBranch[row.ID.String()]; ok {
			node.Preview = preview
		}
		graph.Branches = append(graph.Branches, node)
	}
	graph.Turns = buildBranchTurns(graph.Branches, turnRows)
	return graph, nil
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
		Title:           row.Title,
		Metadata:        parseJSONMap(row.Metadata),
		ParentSessionID: parentID,
		CreatedByUserID: createdByUserID,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toBranchNode(row sqlc.ListSessionBranchesRow) BranchNode {
	node := BranchNode{
		ID:        row.ID.String(),
		SessionID: row.SessionID.String(),
		Title:     dbpkg.TextToString(row.Title),
		CreatedAt: dbpkg.TimeFromPg(row.CreatedAt),
		UpdatedAt: dbpkg.TimeFromPg(row.UpdatedAt),
	}
	if row.ParentBranchID.Valid {
		node.ParentBranchID = row.ParentBranchID.String()
	}
	if row.ForkFromMessageID.Valid {
		node.ForkFromMessageID = row.ForkFromMessageID.String()
	}
	if row.ForkFromSeq.Valid {
		node.ForkFromSeq = row.ForkFromSeq.Int64
	}
	if row.ForkFromTurnID.Valid {
		node.ForkFromTurnID = row.ForkFromTurnID.String()
	}
	if row.ForkFromTurnSeq.Valid {
		node.ForkFromTurnSeq = row.ForkFromTurnSeq.Int64
	}
	return node
}

func buildBranchPreviewMap(rows []sqlc.ListSessionBranchPreviewMessagesRow) map[string]TurnPreview {
	out := make(map[string]TurnPreview)
	for _, row := range rows {
		if !row.BranchID.Valid {
			continue
		}
		key := row.BranchID.String()
		preview := out[key]
		switch row.Role {
		case "assistant":
			preview.AssistantText = firstNonEmpty(preview.AssistantText, previewTextFromMessage(row.Content, row.DisplayText))
			preview.MessageID = row.ID.String()
			preview.Timestamp = dbpkg.TimeFromPg(row.CreatedAt)
		case "user":
			preview.UserText = firstNonEmpty(preview.UserText, previewTextFromMessage(row.Content, row.DisplayText))
			if preview.Timestamp.IsZero() {
				preview.Timestamp = dbpkg.TimeFromPg(row.CreatedAt)
			}
		}
		out[key] = preview
	}
	return out
}

func buildBranchTurns(branches []BranchNode, rows []sqlc.ListSessionBranchTurnMessagesRow) []BranchTurn {
	branchByID := make(map[string]BranchNode, len(branches))
	depthByBranch := make(map[string]int, len(branches))
	activeBranchID := ""
	for _, branch := range branches {
		branchByID[branch.ID] = branch
		if branch.Active {
			activeBranchID = branch.ID
		}
	}
	var branchDepth func(string) int
	branchDepth = func(branchID string) int {
		if depth, ok := depthByBranch[branchID]; ok {
			return depth
		}
		branch := branchByID[branchID]
		depth := 0
		if strings.TrimSpace(branch.ParentBranchID) != "" {
			depth = branchDepth(branch.ParentBranchID) + 1
		}
		depthByBranch[branchID] = depth
		return depth
	}

	turns := make([]BranchTurn, 0, len(rows))
	lastTurnByBranch := make(map[string]string, len(branches))
	turnIDByBranchSeq := make(map[string]map[int64]string, len(branches))
	for _, row := range rows {
		if !row.BranchID.Valid {
			continue
		}
		branchID := row.BranchID.String()
		seq := row.TurnSeq
		branch := branchByID[branchID]
		parentTurnID := lastTurnByBranch[branchID]
		if parentTurnID == "" && strings.TrimSpace(branch.ParentBranchID) != "" {
			parentTurnID = branch.ForkFromTurnID
			if parentTurnID == "" && branch.ForkFromTurnSeq > 0 {
				parentTurnID = turnIDForBranchSeq(turnIDByBranchSeq, branch.ParentBranchID, branch.ForkFromTurnSeq)
			}
		}
		branchSeq := int64(0)
		if row.BranchSeq.Valid {
			branchSeq = row.BranchSeq.Int64
		}
		turn := BranchTurn{
			ID:           row.TurnID.String(),
			BranchID:     branchID,
			ParentTurnID: parentTurnID,
			AssistantID:  row.AssistantID.String(),
			BranchSeq:    branchSeq,
			TurnSeq:      seq,
			Depth:        branchDepth(branchID),
			Active:       branchID == activeBranchID,
			Title:        branchTurnTitle(row),
			Preview: TurnPreview{
				UserText:      previewTextFromMessage(row.UserContent, row.UserDisplayText),
				AssistantText: previewTextFromMessage(row.AssistantContent, row.AssistantDisplayText),
				MessageID:     row.AssistantID.String(),
				Timestamp:     dbpkg.TimeFromPg(row.AssistantCreatedAt),
			},
			ForkFromSeq: branch.ForkFromTurnSeq,
			CreatedAt:   dbpkg.TimeFromPg(row.AssistantCreatedAt),
		}
		if row.UserID.Valid {
			turn.UserID = row.UserID.String()
		}
		turns = append(turns, turn)
		lastTurnByBranch[branchID] = turn.ID
		if _, ok := turnIDByBranchSeq[branchID]; !ok {
			turnIDByBranchSeq[branchID] = make(map[int64]string)
		}
		if seq > 0 {
			turnIDByBranchSeq[branchID][seq] = turn.ID
		}
	}
	return turns
}

func turnIDForBranchSeq(index map[string]map[int64]string, branchID string, seq int64) string {
	if seq <= 0 {
		return ""
	}
	return index[branchID][seq]
}

func firstNonEmpty(existing, next string) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	return strings.TrimSpace(next)
}

func branchTurnTitle(row sqlc.ListSessionBranchTurnMessagesRow) string {
	if text := strings.TrimSpace(dbpkg.TextToString(row.Title)); text != "" {
		return summarizeTitleText(text)
	}
	userText := previewTextFromMessage(row.UserContent, row.UserDisplayText)
	if userText != "" {
		return summarizeTitleText(userText)
	}
	return summarizeTitleText(previewTextFromMessage(row.AssistantContent, row.AssistantDisplayText))
}

func previewTextFromMessage(content []byte, displayText pgtype.Text) string {
	if text := strings.TrimSpace(dbpkg.TextToString(displayText)); text != "" {
		return summarizePreviewText(text)
	}
	texts := extractTextFragments(content)
	return summarizePreviewText(strings.Join(texts, "\n\n"))
}

func summarizeTitleText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxRunes = 48
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func summarizePreviewText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxRunes = 180
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func extractTextFragments(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return nil
	}
	return collectTextFragments(value)
}

func collectTextFragments(value any) []string {
	switch v := value.(type) {
	case string:
		if text := strings.TrimSpace(v); text != "" {
			return []string{text}
		}
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, collectTextFragments(item)...)
		}
		return out
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			typ, _ := v["type"].(string)
			if typ == "" || typ == "text" || typ == "input_text" || typ == "output_text" {
				if text = strings.TrimSpace(text); text != "" {
					return []string{text}
				}
			}
		}
		if content, ok := v["content"]; ok {
			return collectTextFragments(content)
		}
		if text, ok := v["display_text"].(string); ok && strings.TrimSpace(text) != "" {
			return []string{strings.TrimSpace(text)}
		}
	}
	return nil
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
