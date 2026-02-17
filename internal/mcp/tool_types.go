package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// ToolSessionContext carries request-scoped identity for tool execution.
type ToolSessionContext struct {
	BotID             string
	ChatID            string
	ChannelIdentityID string
	SessionToken      string
	CurrentPlatform   string
	ReplyTarget       string
}

// ToolDescriptor is the MCP tools/list item shape used by the gateway.
type ToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolExecutor represents business-facing tools (message/schedule/memory).
type ToolExecutor interface {
	ListTools(ctx context.Context, session ToolSessionContext) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, session ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error)
}

// ToolSource represents infrastructure-level tool sources (federation/connectors).
// A source is not a business tool itself; it supplies and routes downstream tools.
type ToolSource interface {
	ListTools(ctx context.Context, session ToolSessionContext) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, session ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error)
}

// ToolCallPayload is the MCP tools/call params payload.
type ToolCallPayload struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ErrToolNotFound indicates the provider does not own the requested tool.
var ErrToolNotFound = errors.New("tool not found")

// BuildToolSuccessResult builds a standard MCP tool success result object.
func BuildToolSuccessResult(structured any) map[string]any {
	result := map[string]any{}
	if structured != nil {
		result["structuredContent"] = structured
		if text := stringifyStructuredContent(structured); text != "" {
			result["content"] = []map[string]any{
				{
					"type": "text",
					"text": text,
				},
			}
		}
	}
	if len(result) == 0 {
		result["content"] = []map[string]any{
			{
				"type": "text",
				"text": "ok",
			},
		}
	}
	return result
}

// BuildToolErrorResult builds a standard MCP tool error result object.
func BuildToolErrorResult(message string) map[string]any {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "tool execution failed"
	}
	return map[string]any{
		"isError": true,
		"content": []map[string]any{
			{
				"type": "text",
				"text": msg,
			},
		},
	}
}

func stringifyStructuredContent(v any) string {
	if v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(payload)
	}
}

// StringArg returns the string value for key in arguments, or empty string if missing/invalid.
func StringArg(arguments map[string]any, key string) string {
	if arguments == nil {
		return ""
	}
	raw, ok := arguments[key]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", raw))
	}
}

// FirstStringArg returns the first non-empty string value for the given keys in arguments.
func FirstStringArg(arguments map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := StringArg(arguments, key); value != "" {
			return value
		}
	}
	return ""
}

// IntArg parses the key in arguments as an integer; returns value, true if present, or error if wrong type.
func IntArg(arguments map[string]any, key string) (int, bool, error) {
	if arguments == nil {
		return 0, false, nil
	}
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case int:
		return value, true, nil
	case int8:
		return int(value), true, nil
	case int16:
		return int(value), true, nil
	case int32:
		return int(value), true, nil
	case int64:
		return int(value), true, nil
	case uint:
		return int(value), true, nil
	case uint8:
		return int(value), true, nil
	case uint16:
		return int(value), true, nil
	case uint32:
		return int(value), true, nil
	case uint64:
		return int(value), true, nil
	case float32:
		f := float64(value)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, true, fmt.Errorf("%s must be a valid number", key)
		}
		return int(f), true, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, true, fmt.Errorf("%s must be a valid number", key)
		}
		return int(value), true, nil
	case json.Number:
		i, err := value.Int64()
		if err != nil {
			return 0, true, fmt.Errorf("%s must be an integer", key)
		}
		return int(i), true, nil
	default:
		return 0, true, fmt.Errorf("%s must be a number", key)
	}
}

// BoolArg returns the boolean value for key in arguments; second return is true if key present, error if wrong type.
func BoolArg(arguments map[string]any, key string) (bool, bool, error) {
	if arguments == nil {
		return false, false, nil
	}
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, true, fmt.Errorf("%s must be a boolean", key)
	}
	return value, true, nil
}
