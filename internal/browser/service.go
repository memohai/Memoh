package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

const (
	StatusActive  = "active"
	StatusClosed  = "closed"
	StatusExpired = "expired"
	StatusError   = "error"
)

type Service struct {
	log     *slog.Logger
	queries *dbsqlc.Queries
	cfg     config.BrowserConfig
	remote  *remoteClient
}

type CreateSessionRequest struct {
	IdleTTLSeconds int32 `json:"idle_ttl_seconds,omitempty"`
}

type ActionRequest struct {
	Name   string         `json:"name"`
	URL    string         `json:"url,omitempty"`
	Target string         `json:"target,omitempty"`
	Value  string         `json:"value,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

type Session struct {
	ID              string         `json:"id"`
	BotID           string         `json:"bot_id"`
	SessionID       string         `json:"session_id"`
	Provider        string         `json:"provider"`
	RemoteSessionID string         `json:"remote_session_id"`
	WorkerID        string         `json:"worker_id"`
	Status          string         `json:"status"`
	CurrentURL      string         `json:"current_url"`
	ContextDir      string         `json:"context_dir"`
	IdleTTLSeconds  int32          `json:"idle_ttl_seconds"`
	ActionCount     int32          `json:"action_count"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LastUsedAt      time.Time      `json:"last_used_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

func NewService(log *slog.Logger, queries *dbsqlc.Queries, cfg config.BrowserConfig) *Service {
	if log == nil {
		log = slog.Default()
	}
	cfg.ApplyDefaults()
	return &Service{
		log:     log.With(slog.String("service", "browser")),
		queries: queries,
		cfg:     cfg,
		remote:  newRemoteClient(cfg.ServerBaseURL, cfg.ServerAPIKey, cfg.ActionTimeoutSeconds),
	}
}

func (s *Service) CreateSession(ctx context.Context, botID string, req CreateSessionRequest) (Session, error) {
	if s.queries == nil {
		return Session{}, fmt.Errorf("browser queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Session{}, err
	}
	activeCount, err := s.queries.CountActiveBrowserSessionsByBot(ctx, pgBotID)
	if err != nil {
		return Session{}, err
	}
	if activeCount >= int64(s.cfg.MaxSessionsPerBot) {
		return Session{}, fmt.Errorf("active browser sessions limit reached for bot")
	}
	ttl := s.normalizeTTL(req.IdleTTLSeconds)
	sessionID := "bs_" + uuid.NewString()
	contextDir := filepath.Join("bots", botID, ".browser", sessionID)
	remoteCreated, err := s.remote.CreateSession(ctx, remoteSessionCreateRequest{
		BotID:          botID,
		SessionID:      sessionID,
		ContextDir:     contextDir,
		IdleTTLSeconds: ttl,
	})
	if err != nil {
		return Session{}, err
	}
	metaPayload, err := json.Marshal(map[string]any{
		"last_action": "new_session",
	})
	if err != nil {
		return Session{}, err
	}
	row, err := s.queries.CreateBrowserSession(ctx, dbsqlc.CreateBrowserSessionParams{
		BotID:           pgBotID,
		SessionID:       sessionID,
		Provider:        "remote",
		RemoteSessionID: strings.TrimSpace(remoteCreated.RemoteSessionID),
		WorkerID:        strings.TrimSpace(remoteCreated.WorkerID),
		Status:          StatusActive,
		CurrentUrl:      "",
		ContextDir:      contextDir,
		IdleTtlSeconds:  ttl,
		ActionCount:     0,
		Metadata:        metaPayload,
		ExpiresAt:       toPgTimestamp(time.Now().Add(time.Duration(ttl) * time.Second)),
	})
	if err != nil {
		return Session{}, err
	}
	return toSession(row)
}

func (s *Service) ListSessions(ctx context.Context, botID string) ([]Session, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("browser queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBrowserSessionsByBot(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Session, 0, len(rows))
	for _, row := range rows {
		item, convErr := toSession(row)
		if convErr != nil {
			return nil, convErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) CloseSession(ctx context.Context, botID, sessionID string) (Session, error) {
	item, err := s.GetSession(ctx, botID, sessionID)
	if err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(item.RemoteSessionID) != "" {
		if err := s.remote.CloseSession(ctx, item.RemoteSessionID); err != nil {
			s.log.Warn("remote close browser session failed",
				slog.String("bot_id", botID),
				slog.String("session_id", sessionID),
				slog.String("remote_session_id", item.RemoteSessionID),
				slog.Any("error", err),
			)
		}
	}
	metaPayload, err := json.Marshal(mergeMeta(item.Metadata, map[string]any{
		"last_action": "close_session",
	}))
	if err != nil {
		return Session{}, err
	}
	row, err := s.queries.CloseBrowserSession(ctx, dbsqlc.CloseBrowserSessionParams{
		SessionID: sessionID,
		Metadata:  metaPayload,
	})
	if err != nil {
		return Session{}, err
	}
	return toSession(row)
}

func (s *Service) CloseSessionsByBot(ctx context.Context, botID string) error {
	if s.queries == nil {
		return fmt.Errorf("browser queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	rows, err := s.queries.ListBrowserSessionsByBot(ctx, pgBotID)
	if err == nil {
		for _, row := range rows {
			if strings.TrimSpace(row.Status) != StatusActive {
				continue
			}
			if strings.TrimSpace(row.RemoteSessionID) == "" {
				continue
			}
			if closeErr := s.remote.CloseSession(ctx, strings.TrimSpace(row.RemoteSessionID)); closeErr != nil {
				s.log.Warn("remote close browser session by bot failed",
					slog.String("bot_id", botID),
					slog.String("remote_session_id", strings.TrimSpace(row.RemoteSessionID)),
					slog.Any("error", closeErr),
				)
			}
		}
	}
	return s.queries.CloseBrowserSessionsByBot(ctx, pgBotID)
}

func (s *Service) CleanupExpiredSessions(ctx context.Context) (int, error) {
	if s.queries == nil {
		return 0, fmt.Errorf("browser queries not configured")
	}
	rows, err := s.queries.ExpireBrowserSessionsBefore(ctx, toPgTimestamp(time.Now()))
	if err != nil {
		return 0, err
	}
	if len(rows) > 0 {
		s.log.Info("expired browser sessions cleaned", slog.Int("count", len(rows)))
		for _, row := range rows {
			if strings.TrimSpace(row.RemoteSessionID) == "" {
				continue
			}
			if closeErr := s.remote.CloseSession(ctx, strings.TrimSpace(row.RemoteSessionID)); closeErr != nil {
				s.log.Warn("remote close expired browser session failed",
					slog.String("remote_session_id", strings.TrimSpace(row.RemoteSessionID)),
					slog.Any("error", closeErr),
				)
			}
		}
	}
	return len(rows), nil
}

func (s *Service) ExecuteAction(ctx context.Context, botID, sessionID string, req ActionRequest) (map[string]any, error) {
	item, err := s.GetSession(ctx, botID, sessionID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusActive {
		return nil, fmt.Errorf("session is not active")
	}
	if item.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("session expired")
	}
	if item.ActionCount >= s.cfg.MaxActionsPerSession {
		return nil, fmt.Errorf("session action limit reached")
	}
	actionName := strings.ToLower(strings.TrimSpace(req.Name))
	metadata := mergeMeta(item.Metadata, map[string]any{
		"last_action": actionName,
	})
	start := time.Now()
	switch actionName {
	case "goto", "extract_text":
		targetURL := strings.TrimSpace(req.URL)
		if targetURL != "" {
			if err := validateURL(targetURL); err != nil {
				return nil, err
			}
		}
	case "click", "type", "screenshot":
	default:
		return nil, fmt.Errorf("unsupported browser action: %s", actionName)
	}
	result, err := s.remote.ExecuteAction(ctx, item.RemoteSessionID, remoteActionRequest{
		Name:   actionName,
		URL:    strings.TrimSpace(req.URL),
		Target: strings.TrimSpace(req.Target),
		Value:  strings.TrimSpace(req.Value),
		Params: req.Params,
	})
	if err != nil {
		return nil, err
	}
	nextURL := item.CurrentURL
	if raw, ok := result["current_url"].(string); ok && strings.TrimSpace(raw) != "" {
		nextURL = strings.TrimSpace(raw)
	} else if raw, ok := result["url"].(string); ok && strings.TrimSpace(raw) != "" {
		nextURL = strings.TrimSpace(raw)
	}
	metadata["last_duration_ms"] = time.Since(start).Milliseconds()
	metadata["last_result"] = result
	metaPayload, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	updated, err := s.queries.TouchBrowserSession(ctx, dbsqlc.TouchBrowserSessionParams{
		SessionID:   sessionID,
		CurrentUrl:  nextURL,
		ActionCount: item.ActionCount + 1,
		Metadata:    metaPayload,
		ExpiresAt:   toPgTimestamp(time.Now().Add(time.Duration(item.IdleTTLSeconds) * time.Second)),
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("browser action executed",
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
		slog.String("action", actionName),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
	result["session"] = map[string]any{
		"status":       updated.Status,
		"current_url":  updated.CurrentUrl,
		"action_count": updated.ActionCount,
		"expires_at":   updated.ExpiresAt.Time,
	}
	return result, nil
}

func (s *Service) GetSession(ctx context.Context, botID, sessionID string) (Session, error) {
	row, err := s.queries.GetBrowserSessionBySessionID(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, fmt.Errorf("browser session not found")
		}
		return Session{}, err
	}
	item, err := toSession(row)
	if err != nil {
		return Session{}, err
	}
	if item.BotID != strings.TrimSpace(botID) {
		return Session{}, fmt.Errorf("browser session does not belong to bot")
	}
	return item, nil
}

func (s *Service) normalizeTTL(raw int32) int32 {
	if raw <= 0 {
		return s.cfg.IdleTTLSeconds
	}
	if raw > s.cfg.MaxIdleTTLSeconds {
		return s.cfg.MaxIdleTTLSeconds
	}
	return raw
}

func toSession(row dbsqlc.BrowserSession) (Session, error) {
	meta := map[string]any{}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return Session{}, err
		}
	}
	item := Session{
		ID:              row.ID.String(),
		BotID:           row.BotID.String(),
		SessionID:       strings.TrimSpace(row.SessionID),
		Provider:        strings.TrimSpace(row.Provider),
		RemoteSessionID: strings.TrimSpace(row.RemoteSessionID),
		WorkerID:        strings.TrimSpace(row.WorkerID),
		Status:          strings.TrimSpace(row.Status),
		CurrentURL:      strings.TrimSpace(row.CurrentUrl),
		ContextDir:      strings.TrimSpace(row.ContextDir),
		IdleTTLSeconds:  row.IdleTtlSeconds,
		ActionCount:     row.ActionCount,
		Metadata:        meta,
	}
	if row.CreatedAt.Valid {
		item.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		item.UpdatedAt = row.UpdatedAt.Time
	}
	if row.LastUsedAt.Valid {
		item.LastUsedAt = row.LastUsedAt.Time
	}
	if row.ExpiresAt.Valid {
		item.ExpiresAt = row.ExpiresAt.Time
	}
	return item, nil
}

func toPgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func mergeMeta(base map[string]any, patch map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

func validateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https urls are allowed")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if host == "localhost" {
		return fmt.Errorf("localhost is blocked")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("private or loopback ip is blocked")
		}
		return nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("unable to resolve host")
	}
	for _, ip := range addrs {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if isPrivateIP(addr.Unmap()) {
			return fmt.Errorf("resolved private or loopback ip is blocked")
		}
	}
	return nil
}

func isPrivateIP(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() || addr.IsUnspecified()
}
