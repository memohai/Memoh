package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/mcp"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
)

const pluginMCPProbeTimeout = 30 * time.Second

type pluginMCPConnectionGetter interface {
	Get(ctx context.Context, botID, id string) (mcp.Connection, error)
}

type pluginMCPProbeRecorder interface {
	RecordMCPResourceProbeResult(ctx context.Context, botID, installationID, resourceKey, resourceID, status string, tools []mcp.ToolDescriptor, message string) error
}

type pluginMCPToolLister interface {
	ListHTTPConnectionTools(ctx context.Context, connection mcp.Connection) ([]mcp.ToolDescriptor, error)
	ListSSEConnectionTools(ctx context.Context, connection mcp.Connection) ([]mcp.ToolDescriptor, error)
	ListStdioConnectionTools(ctx context.Context, botID string, connection mcp.Connection) ([]mcp.ToolDescriptor, error)
}

type pluginMCPProbeDeps struct {
	connections pluginMCPConnectionGetter
	gateway     pluginMCPToolLister
	recorder    pluginMCPProbeRecorder
}

func (h *SupermarketHandler) probeReadyPluginMCPs(ctx context.Context, botID string, installation pluginspkg.Installation) error {
	return probePluginMCPs(ctx, botID, installation, pluginMCPProbeOptions{requireEnabled: false, requireActive: false}, pluginMCPProbeDeps{
		connections: h.mcpService,
		gateway:     h.fedGateway,
		recorder:    h.pluginService,
	})
}

func (h *PluginsHandler) probeReadyPluginMCPs(ctx context.Context, botID string, installation pluginspkg.Installation) error {
	return probePluginMCPs(ctx, botID, installation, pluginMCPProbeOptions{requireEnabled: false, requireActive: false}, pluginMCPProbeDeps{
		connections: h.mcpService,
		gateway:     h.fedGateway,
		recorder:    h.service,
	})
}

type pluginMCPProbeOptions struct {
	requireEnabled bool
	requireActive  bool
}

func probeEnabledPluginMCPs(ctx context.Context, botID string, installation pluginspkg.Installation, deps pluginMCPProbeDeps) error {
	return probePluginMCPs(ctx, botID, installation, pluginMCPProbeOptions{requireEnabled: true, requireActive: true}, deps)
}

func probePluginMCPs(ctx context.Context, botID string, installation pluginspkg.Installation, options pluginMCPProbeOptions, deps pluginMCPProbeDeps) error {
	if options.requireEnabled && !installation.Enabled {
		return nil
	}
	if installation.Status != pluginspkg.StatusReady {
		return nil
	}

	hasDeclaredMCPResource := len(installation.Manifest.MCPs) > 0
	hasMCPResource := false
	var firstErr error
	for _, resource := range installation.Resources {
		if strings.TrimSpace(resource.Type) != "mcp" {
			continue
		}
		hasMCPResource = true
		connID := strings.TrimSpace(resource.ResourceID)
		if connID == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin MCP resource %q is missing a connection", pluginResourceLabel(installation, resource))
			}
			continue
		}
		if deps.connections == nil {
			return errors.New("plugin MCP probe is not configured")
		}
		conn, err := deps.connections.Get(ctx, botID, connID)
		if err != nil {
			return fmt.Errorf("plugin MCP resource %q connection lookup failed: %w", pluginResourceLabel(installation, resource), err)
		}
		if options.requireActive && !conn.Active {
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin MCP resource %q is not active", pluginResourceLabel(installation, resource))
			}
			continue
		}
		if deps.gateway == nil {
			return errors.New("plugin MCP probe is not configured")
		}

		probeCtx, cancel := context.WithTimeout(ctx, pluginMCPProbeTimeout)
		tools, probeErr := listPluginMCPTools(probeCtx, deps.gateway, botID, conn)
		cancel()
		if probeErr != nil {
			message := sanitizePluginProbeMessage(probeErr.Error())
			recordPluginMCPProbeResult(ctx, deps.recorder, botID, installation.ID, resource, "error", []mcp.ToolDescriptor{}, message)
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin MCP resource %q connection failed: %s", pluginResourceLabel(installation, resource), message)
			}
			continue
		}
		if tools == nil {
			tools = []mcp.ToolDescriptor{}
		}
		tools = mcp.FilterToolsByMetadata(tools, conn.Metadata)
		recordPluginMCPProbeResult(ctx, deps.recorder, botID, installation.ID, resource, "connected", tools, "")
	}
	if firstErr != nil {
		return firstErr
	}
	if hasDeclaredMCPResource && !hasMCPResource {
		return errors.New("plugin declares MCP resources but none are installed")
	}
	return nil
}

func listPluginMCPTools(ctx context.Context, gateway pluginMCPToolLister, botID string, conn mcp.Connection) ([]mcp.ToolDescriptor, error) {
	switch strings.ToLower(strings.TrimSpace(conn.Type)) {
	case "http":
		return gateway.ListHTTPConnectionTools(ctx, conn)
	case "sse":
		return gateway.ListSSEConnectionTools(ctx, conn)
	case "stdio":
		return gateway.ListStdioConnectionTools(ctx, botID, conn)
	default:
		return nil, fmt.Errorf("unsupported connection type: %s", conn.Type)
	}
}

func recordPluginMCPProbeResult(ctx context.Context, recorder pluginMCPProbeRecorder, botID, installationID string, resource pluginspkg.Resource, status string, tools []mcp.ToolDescriptor, message string) {
	if recorder == nil {
		return
	}
	_ = recorder.RecordMCPResourceProbeResult(ctx, botID, installationID, resource.Key, resource.ResourceID, status, tools, message)
}

func pluginResourceLabel(installation pluginspkg.Installation, resource pluginspkg.Resource) string {
	if resource.Metadata != nil {
		for _, key := range []string{"display_name", "name", "resource_key"} {
			if value, ok := resource.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	for _, item := range installation.Manifest.MCPs {
		if strings.TrimSpace(item.Key) != strings.TrimSpace(resource.Key) {
			continue
		}
		for _, value := range []string{item.DisplayName, item.Name, item.Key} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if strings.TrimSpace(resource.Key) != "" {
		return strings.TrimSpace(resource.Key)
	}
	return strings.TrimSpace(resource.ResourceID)
}

func sanitizePluginProbeMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "connection failed"
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "unauthenticated"),
		strings.Contains(lower, "401"),
		strings.Contains(lower, "invalid or expired token"),
		strings.Contains(lower, "invalid token"),
		strings.Contains(lower, "expired token"),
		strings.Contains(lower, "invalid api key"),
		strings.Contains(lower, "invalid_api_key"):
		return "authorization failed"
	case strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "403"):
		return "permission denied"
	case strings.Contains(lower, "connection timed out"),
		strings.Contains(lower, "timed out"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "deadline exceeded"):
		return "connection timed out"
	}
	if len(message) > 2048 {
		message = strings.TrimSpace(message[:2048]) + "..."
	}
	return message
}
