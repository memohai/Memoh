package session

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
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	RouteMetadata         map[string]any `json:"route_metadata,omitempty"`
	RouteConversationType string         `json:"route_conversation_type,omitempty"`
}

const (
	TypeChat      = "chat"
	TypeHeartbeat = "heartbeat"
	TypeSchedule  = "schedule"
	TypeSubagent  = "subagent"
)

// CreateInput holds input for creating a new session.
type CreateInput struct {
	BotID           string
	RouteID         string
	ChannelType     string
	Type            string
	Title           string
	Metadata        map[string]any
	ParentSessionID string
}

// Service manages bot chat sessions.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a session service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "session")),
	}
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

	meta := input.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return Session{}, fmt.Errorf("marshal metadata: %w", err)
	}

	channelType := pgtype.Text{}
	if ct := strings.TrimSpace(input.ChannelType); ct != "" {
		channelType = pgtype.Text{String: ct, Valid: true}
	}

	sessionType := strings.TrimSpace(input.Type)
	if sessionType == "" {
		sessionType = TypeChat
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
		Title:           input.Title,
		Metadata:        metaBytes,
		ParentSessionID: pgParentSessionID,
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
func (s *Service) CreateNewSession(ctx context.Context, botID, routeID, channelType string) (Session, error) {
	sess, err := s.Create(ctx, CreateInput{
		BotID:       botID,
		RouteID:     routeID,
		ChannelType: channelType,
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
	return Session{
		ID:              row.ID.String(),
		BotID:           row.BotID.String(),
		RouteID:         row.RouteID.String(),
		ChannelType:     dbpkg.TextToString(row.ChannelType),
		Type:            row.Type,
		Title:           row.Title,
		Metadata:        parseJSONMap(row.Metadata),
		ParentSessionID: parentID,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
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
	return Session{
		ID:                    row.ID.String(),
		BotID:                 row.BotID.String(),
		RouteID:               row.RouteID.String(),
		ChannelType:           dbpkg.TextToString(row.ChannelType),
		Type:                  row.Type,
		Title:                 row.Title,
		Metadata:              parseJSONMap(row.Metadata),
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
		RouteMetadata:         parseJSONMap(row.RouteMetadata),
		RouteConversationType: dbpkg.TextToString(row.RouteConversationType),
	}
}
