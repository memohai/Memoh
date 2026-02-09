package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Connection represents a stored MCP connection for a bot.
type Connection struct {
	ID        string         `json:"id"`
	BotID     string         `json:"bot_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
	Active    bool           `json:"active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// UpsertRequest is the payload for creating or updating MCP connections.
type UpsertRequest struct {
	Name   string         `json:"name"`
	Type   string         `json:"type,omitempty"`
	Config map[string]any `json:"config"`
	Active *bool          `json:"active,omitempty"`
}

// ListResponse wraps MCP connection list responses.
type ListResponse struct {
	Items []Connection `json:"items"`
}

// ConnectionService handles CRUD operations for MCP connections.
type ConnectionService struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewConnectionService creates a ConnectionService backed by sqlc queries.
func NewConnectionService(log *slog.Logger, queries *sqlc.Queries) *ConnectionService {
	if log == nil {
		log = slog.Default()
	}
	return &ConnectionService{
		queries: queries,
		logger:  log.With(slog.String("service", "mcp_connections")),
	}
}

// ListByBot returns all MCP connections for a bot.
func (s *ConnectionService) ListByBot(ctx context.Context, botID string) ([]Connection, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("mcp queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListMCPConnectionsByBotID(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	items := make([]Connection, 0, len(rows))
	for _, row := range rows {
		item, err := normalizeMCPConnection(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListActiveByBot returns active MCP connections for a bot.
func (s *ConnectionService) ListActiveByBot(ctx context.Context, botID string) ([]Connection, error) {
	items, err := s.ListByBot(ctx, botID)
	if err != nil {
		return nil, err
	}
	active := make([]Connection, 0, len(items))
	for _, item := range items {
		if item.Active {
			active = append(active, item)
		}
	}
	return active, nil
}

// Get returns a specific MCP connection for a bot.
func (s *ConnectionService) Get(ctx context.Context, botID, id string) (Connection, error) {
	if s.queries == nil {
		return Connection{}, fmt.Errorf("mcp queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return Connection{}, err
	}
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return Connection{}, err
	}
	row, err := s.queries.GetMCPConnectionByID(ctx, sqlc.GetMCPConnectionByIDParams{
		BotID: pgBotID,
		ID:    pgID,
	})
	if err != nil {
		return Connection{}, err
	}
	return normalizeMCPConnection(row)
}

// Create inserts a new MCP connection for a bot.
func (s *ConnectionService) Create(ctx context.Context, botID string, req UpsertRequest) (Connection, error) {
	if s.queries == nil {
		return Connection{}, fmt.Errorf("mcp queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Connection{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Connection{}, fmt.Errorf("name is required")
	}
	mcpType, config, err := normalizeMCPType(req)
	if err != nil {
		return Connection{}, err
	}
	configPayload, err := json.Marshal(config)
	if err != nil {
		return Connection{}, err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	row, err := s.queries.CreateMCPConnection(ctx, sqlc.CreateMCPConnectionParams{
		BotID:    botUUID,
		Name:     name,
		Type:     mcpType,
		Config:   configPayload,
		IsActive: active,
	})
	if err != nil {
		return Connection{}, err
	}
	return normalizeMCPConnection(row)
}

// Update modifies an existing MCP connection.
func (s *ConnectionService) Update(ctx context.Context, botID, id string, req UpsertRequest) (Connection, error) {
	if s.queries == nil {
		return Connection{}, fmt.Errorf("mcp queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Connection{}, err
	}
	connUUID, err := db.ParseUUID(id)
	if err != nil {
		return Connection{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Connection{}, fmt.Errorf("name is required")
	}
	mcpType, config, err := normalizeMCPType(req)
	if err != nil {
		return Connection{}, err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	configPayload, err := json.Marshal(config)
	if err != nil {
		return Connection{}, err
	}
	row, err := s.queries.UpdateMCPConnection(ctx, sqlc.UpdateMCPConnectionParams{
		BotID:    botUUID,
		ID:       connUUID,
		Name:     name,
		Type:     mcpType,
		Config:   configPayload,
		IsActive: active,
	})
	if err != nil {
		return Connection{}, err
	}
	return normalizeMCPConnection(row)
}

// Delete removes an MCP connection.
func (s *ConnectionService) Delete(ctx context.Context, botID, id string) error {
	if s.queries == nil {
		return fmt.Errorf("mcp queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	connUUID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteMCPConnection(ctx, sqlc.DeleteMCPConnectionParams{
		BotID: botUUID,
		ID:    connUUID,
	})
}

func normalizeMCPConnection(row sqlc.McpConnection) (Connection, error) {
	config, err := decodeMCPConfig(row.Config)
	if err != nil {
		return Connection{}, err
	}
	return Connection{
		ID:        db.UUIDToString(row.ID),
		BotID:     db.UUIDToString(row.BotID),
		Name:      strings.TrimSpace(row.Name),
		Type:      strings.TrimSpace(row.Type),
		Config:    config,
		Active:    row.IsActive,
		CreatedAt: db.TimeFromPg(row.CreatedAt),
		UpdatedAt: db.TimeFromPg(row.UpdatedAt),
	}, nil
}

func decodeMCPConfig(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func normalizeMCPType(req UpsertRequest) (string, map[string]any, error) {
	config := req.Config
	if config == nil {
		config = map[string]any{}
	}
	mcpType := strings.TrimSpace(req.Type)
	if mcpType == "" {
		if raw, ok := config["type"].(string); ok {
			mcpType = strings.TrimSpace(raw)
		}
	}
	mcpType = strings.ToLower(strings.TrimSpace(mcpType))
	if mcpType == "" {
		return "", nil, fmt.Errorf("type is required")
	}
	switch mcpType {
	case "stdio", "http", "sse":
	default:
		return "", nil, fmt.Errorf("unsupported mcp type: %s", mcpType)
	}
	config["type"] = mcpType
	return mcpType, config, nil
}
