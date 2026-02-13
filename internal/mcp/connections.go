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
	Active    bool           `json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// UpsertRequest accepts standard mcpServers item format.
// Type is auto-inferred: command present -> stdio, url present -> http (default) or sse (if transport:"sse").
type UpsertRequest struct {
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Active    *bool             `json:"is_active,omitempty"`
}

// ImportRequest accepts a standard mcpServers dict for batch import.
type ImportRequest struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// MCPServerEntry is one entry in the standard mcpServers dict.
type MCPServerEntry struct {
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Transport string            `json:"transport,omitempty"`
}

// ListResponse wraps MCP connection list responses.
type ListResponse struct {
	Items []Connection `json:"items"`
}

// ExportResponse returns connections in standard mcpServers format.
type ExportResponse struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
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
	mcpType, config, err := inferTypeAndConfig(req)
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
	mcpType, config, err := inferTypeAndConfig(req)
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

// Import performs a declarative sync from a standard mcpServers dict.
// Existing connections (matched by name) get config updated but is_active preserved.
// New connections are created with is_active=true.
// Connections not in the input are left untouched.
func (s *ConnectionService) Import(ctx context.Context, botID string, req ImportRequest) ([]Connection, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("mcp queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	if len(req.MCPServers) == 0 {
		return []Connection{}, nil
	}
	results := make([]Connection, 0, len(req.MCPServers))
	for name, entry := range req.MCPServers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upsert := entryToUpsertRequest(name, entry)
		mcpType, config, err := inferTypeAndConfig(upsert)
		if err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
		configPayload, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		row, err := s.queries.UpsertMCPConnectionByName(ctx, sqlc.UpsertMCPConnectionByNameParams{
			BotID:  botUUID,
			Name:   name,
			Type:   mcpType,
			Config: configPayload,
		})
		if err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
		conn, err := normalizeMCPConnection(row)
		if err != nil {
			return nil, err
		}
		results = append(results, conn)
	}
	return results, nil
}

// ExportByBot returns all connections for a bot in standard mcpServers format.
func (s *ConnectionService) ExportByBot(ctx context.Context, botID string) (ExportResponse, error) {
	items, err := s.ListByBot(ctx, botID)
	if err != nil {
		return ExportResponse{}, err
	}
	servers := make(map[string]MCPServerEntry, len(items))
	for _, conn := range items {
		servers[conn.Name] = connectionToExportEntry(conn)
	}
	return ExportResponse{MCPServers: servers}, nil
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

// BatchDelete removes multiple MCP connections by IDs. Invalid IDs are skipped; at least one must succeed for no error.
func (s *ConnectionService) BatchDelete(ctx context.Context, botID string, ids []string) error {
	if s.queries == nil {
		return fmt.Errorf("mcp queries not configured")
	}
	if len(ids) == 0 {
		return nil
	}
	var lastErr error
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := s.Delete(ctx, botID, id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func normalizeMCPConnection(row sqlc.McpConnection) (Connection, error) {
	config, err := decodeMCPConfig(row.Config)
	if err != nil {
		return Connection{}, err
	}
	return Connection{
		ID:        row.ID.String(),
		BotID:     row.BotID.String(),
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

// inferTypeAndConfig builds internal type + config from a standard mcpServers item.
func inferTypeAndConfig(req UpsertRequest) (string, map[string]any, error) {
	hasCommand := strings.TrimSpace(req.Command) != ""
	hasURL := strings.TrimSpace(req.URL) != ""

	if !hasCommand && !hasURL {
		return "", nil, fmt.Errorf("command or url is required")
	}
	if hasCommand && hasURL {
		return "", nil, fmt.Errorf("command and url are mutually exclusive")
	}

	config := map[string]any{}

	if hasCommand {
		config["command"] = strings.TrimSpace(req.Command)
		if len(req.Args) > 0 {
			config["args"] = req.Args
		}
		if len(req.Env) > 0 {
			config["env"] = req.Env
		}
		if strings.TrimSpace(req.Cwd) != "" {
			config["cwd"] = strings.TrimSpace(req.Cwd)
		}
		return "stdio", config, nil
	}

	config["url"] = strings.TrimSpace(req.URL)
	if len(req.Headers) > 0 {
		config["headers"] = req.Headers
	}
	transport := strings.ToLower(strings.TrimSpace(req.Transport))
	if transport == "sse" {
		return "sse", config, nil
	}
	return "http", config, nil
}

// entryToUpsertRequest converts a named MCPServerEntry to an UpsertRequest.
func entryToUpsertRequest(name string, entry MCPServerEntry) UpsertRequest {
	return UpsertRequest{
		Name:      name,
		Command:   entry.Command,
		Args:      entry.Args,
		Env:       entry.Env,
		Cwd:       entry.Cwd,
		URL:       entry.URL,
		Headers:   entry.Headers,
		Transport: entry.Transport,
	}
}

// connectionToExportEntry converts a stored connection to standard mcpServers entry.
func connectionToExportEntry(conn Connection) MCPServerEntry {
	entry := MCPServerEntry{}
	switch conn.Type {
	case "stdio":
		entry.Command, _ = conn.Config["command"].(string)
		if rawArgs, ok := conn.Config["args"]; ok {
			switch v := rawArgs.(type) {
			case []any:
				for _, a := range v {
					if s, ok := a.(string); ok {
						entry.Args = append(entry.Args, s)
					}
				}
			case []string:
				entry.Args = v
			}
		}
		if rawEnv, ok := conn.Config["env"]; ok {
			if m, ok := rawEnv.(map[string]any); ok {
				entry.Env = make(map[string]string, len(m))
				for k, v := range m {
					if s, ok := v.(string); ok {
						entry.Env[k] = s
					}
				}
			}
		}
		if cwd, ok := conn.Config["cwd"].(string); ok && cwd != "" {
			entry.Cwd = cwd
		}
	case "http", "sse":
		entry.URL, _ = conn.Config["url"].(string)
		if rawHeaders, ok := conn.Config["headers"]; ok {
			if m, ok := rawHeaders.(map[string]any); ok {
				entry.Headers = make(map[string]string, len(m))
				for k, v := range m {
					if s, ok := v.(string); ok {
						entry.Headers[k] = s
					}
				}
			}
		}
		if conn.Type == "sse" {
			entry.Transport = "sse"
		}
	}
	return entry
}
