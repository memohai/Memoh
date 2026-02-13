package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/bots"
)

const mcpCheckTimeout = 8 * time.Second

// ConnectionChecker implements bots.RuntimeChecker for MCP connections.
type ConnectionChecker struct {
	logger      *slog.Logger
	connections *ConnectionService
	gateway     *ToolGatewayService
}

// NewConnectionChecker creates an MCP runtime checker.
func NewConnectionChecker(log *slog.Logger, connections *ConnectionService, gateway *ToolGatewayService) *ConnectionChecker {
	if log == nil {
		log = slog.Default()
	}
	return &ConnectionChecker{
		logger:      log.With(slog.String("checker", "mcp")),
		connections: connections,
		gateway:     gateway,
	}
}

// CheckKeys returns check keys for each active MCP connection of a bot.
func (c *ConnectionChecker) CheckKeys(ctx context.Context, botID string) []string {
	if c.connections == nil {
		return nil
	}
	items, err := c.connections.ListActiveByBot(ctx, botID)
	if err != nil {
		c.logger.Warn("mcp checker: list connections failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return nil
	}
	keys := make([]string, 0, len(items))
	for _, conn := range items {
		keys = append(keys, "mcp."+sanitizeCheckKey(conn.Name))
	}
	return keys
}

// RunCheck probes a single MCP connection identified by check key.
func (c *ConnectionChecker) RunCheck(ctx context.Context, botID, key string) bots.BotCheck {
	connName := strings.TrimPrefix(key, "mcp.")
	check := bots.BotCheck{
		CheckKey: key,
		Status:   bots.BotCheckStatusUnknown,
		Summary:  fmt.Sprintf("MCP server %q is being checked.", connName),
	}

	if c.connections == nil || c.gateway == nil {
		check.Status = bots.BotCheckStatusWarn
		check.Summary = fmt.Sprintf("MCP server %q cannot be checked.", connName)
		check.Detail = "service not available"
		return check
	}

	conn, err := c.findConnectionByKey(ctx, botID, connName)
	if err != nil {
		check.Status = bots.BotCheckStatusError
		check.Summary = fmt.Sprintf("MCP server %q not found.", connName)
		check.Detail = err.Error()
		return check
	}
	check.Metadata = map[string]any{
		"connection_id": conn.ID,
		"name":          conn.Name,
		"type":          conn.Type,
	}

	probeCtx, cancel := context.WithTimeout(ctx, mcpCheckTimeout)
	defer cancel()

	session := ToolSessionContext{BotID: botID}
	tools, err := c.gateway.ListTools(probeCtx, session)
	if err != nil {
		check.Status = bots.BotCheckStatusError
		check.Summary = fmt.Sprintf("MCP server %q is not reachable.", connName)
		check.Detail = err.Error()
		return check
	}

	prefix := sanitizeCheckKey(conn.Name) + "."
	toolCount := 0
	for _, t := range tools {
		if strings.HasPrefix(t.Name, prefix) {
			toolCount++
		}
	}

	if toolCount > 0 {
		check.Status = bots.BotCheckStatusOK
		check.Summary = fmt.Sprintf("MCP server %q is healthy (%d tools).", connName, toolCount)
		check.Metadata["tool_count"] = toolCount
	} else {
		check.Status = bots.BotCheckStatusWarn
		check.Summary = fmt.Sprintf("MCP server %q is reachable but no tools found.", connName)
		check.Detail = "The server responded but exposed no tools for this connection."
	}
	return check
}

func (c *ConnectionChecker) findConnectionByKey(ctx context.Context, botID, sanitizedName string) (Connection, error) {
	items, err := c.connections.ListActiveByBot(ctx, botID)
	if err != nil {
		return Connection{}, err
	}
	for _, conn := range items {
		if sanitizeCheckKey(conn.Name) == sanitizedName {
			return conn, nil
		}
	}
	return Connection{}, fmt.Errorf("connection %q not found", sanitizedName)
}

func sanitizeCheckKey(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "unknown"
	}
	b := strings.Builder{}
	for _, ch := range raw {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_-")
}
