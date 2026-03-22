package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"
)

// SkillDetail holds the description and content of a loadable skill.
type SkillDetail struct {
	Description string
	Content     string
}

// SessionContext carries request-scoped identity for tool execution.
type SessionContext struct {
	BotID              string
	ChatID             string
	SessionID          string
	ChannelIdentityID  string
	SessionToken       string //nolint:gosec // carries session credential material at runtime
	CurrentPlatform    string
	ReplyTarget        string
	SupportsImageInput bool
	IsSubagent         bool
	Skills             map[string]SkillDetail
}

// ToolProvider supplies a set of tools for the agent.
// Tools() is called per-request; implementations may return different
// tool sets based on session context (e.g. subagent restrictions, bot settings).
type ToolProvider interface {
	Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error)
}

// ---- argument parsing helpers ----

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

func FirstStringArg(arguments map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := StringArg(arguments, key); value != "" {
			return value
		}
	}
	return ""
}

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
	case int64:
		if value < int64(math.MinInt) || value > int64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(value), true, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, true, fmt.Errorf("%s must be a valid number", key)
		}
		if value < float64(math.MinInt) || value > float64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(value), true, nil
	case json.Number:
		i, err := value.Int64()
		if err != nil {
			return 0, true, fmt.Errorf("%s must be an integer", key)
		}
		if i < int64(math.MinInt) || i > int64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(i), true, nil
	default:
		return 0, true, fmt.Errorf("%s must be a number", key)
	}
}

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

func inputAsMap(input any) map[string]any {
	args, ok := input.(map[string]any)
	if ok {
		return args
	}
	if input == nil {
		return map[string]any{}
	}
	raw, _ := json.Marshal(input)
	_ = json.Unmarshal(raw, &args)
	if args == nil {
		args = map[string]any{}
	}
	return args
}
