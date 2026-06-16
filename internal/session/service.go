package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
