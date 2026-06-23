package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	textprune "github.com/memohai/memoh/internal/prune"
	sdk "github.com/memohai/twilight-ai/sdk"
)

type ToolOutputLimit struct {
	MaxBytes int
	MaxLines int
}

func WrapToolOutputLimits(sdkTools []sdk.Tool, limit ToolOutputLimit) []sdk.Tool {
	if len(sdkTools) == 0 {
		return sdkTools
	}
	wrapped := make([]sdk.Tool, len(sdkTools))
	copy(wrapped, sdkTools)
	for i := range wrapped {
		execute := wrapped[i].Execute
		if execute == nil {
			continue
		}
		toolName := strings.TrimSpace(wrapped[i].Name)
		label := "tool result"
		if toolName != "" {
			label = "tool result (" + toolName + ")"
		}
		wrapped[i].Execute = func(ctx *sdk.ToolExecContext, input any) (any, error) {
			output, err := execute(ctx, input)
			if err != nil {
				return output, err
			}
			return LimitToolOutput(output, label, limit), nil
		}
	}
	return wrapped
}

func LimitToolOutput(output any, label string, limit ToolOutputLimit) any {
	cfg := toolOutputPruneConfig(limit)
	limited := limitToolOutputValue(output, label, cfg)
	if !toolOutputExceeds(limited, cfg) {
		return limited
	}
	if fitted, ok := fitToolOutputValue(output, label, cfg); ok {
		return fitted
	}
	return fallbackToolOutput(limited, label, cfg)
}

func limitToolOutputValue(value any, label string, cfg textprune.Config) any {
	switch v := value.(type) {
	case string:
		return textprune.PruneWithEdges(v, label, cfg)
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = textprune.PruneWithEdges(item, fmt.Sprintf("%s[%d]", label, i), cfg)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = limitToolOutputValue(item, fmt.Sprintf("%s[%d]", label, i), cfg)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, item := range v {
			out[i], _ = limitToolOutputValue(item, fmt.Sprintf("%s[%d]", label, i), cfg).(map[string]any)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, item := range v {
			out[key] = textprune.PruneWithEdges(item, toolOutputChildLabel(label, key), cfg)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = limitToolOutputValue(item, toolOutputChildLabel(label, key), cfg)
		}
		return out
	default:
		return value
	}
}

func toolOutputChildLabel(label, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return label
	}
	return label + "." + key
}

func toolOutputExceeds(value any, cfg textprune.Config) bool {
	return textprune.Exceeds(marshalToolOutputString(value), cfg.MaxBytes, cfg.MaxLines)
}

func fitToolOutputValue(value any, label string, cfg textprune.Config) (any, bool) {
	stringLeaves := countStringLeaves(value)
	if stringLeaves == 0 {
		return nil, false
	}
	divisors := []int{stringLeaves + 1, stringLeaves*2 + 2, stringLeaves*4 + 4}
	for _, divisor := range divisors {
		if divisor <= 0 {
			continue
		}
		budget := cfg.MaxBytes / divisor
		if budget < len(textprune.DefaultMarker) {
			continue
		}
		tightCfg := toolOutputPruneConfig(ToolOutputLimit{
			MaxBytes: budget,
			MaxLines: cfg.MaxLines,
		})
		fitted := limitToolOutputValue(value, label, tightCfg)
		if !toolOutputExceeds(fitted, cfg) {
			return fitted, true
		}
	}
	return nil, false
}

func countStringLeaves(value any) int {
	switch v := value.(type) {
	case string:
		return 1
	case []string:
		return len(v)
	case []any:
		count := 0
		for _, item := range v {
			count += countStringLeaves(item)
		}
		return count
	case []map[string]any:
		count := 0
		for _, item := range v {
			count += countStringLeaves(item)
		}
		return count
	case map[string]string:
		return len(v)
	case map[string]any:
		count := 0
		for _, item := range v {
			count += countStringLeaves(item)
		}
		return count
	default:
		return 0
	}
}

func fallbackToolOutput(value any, label string, cfg textprune.Config) any {
	raw := marshalToolOutputString(value)
	contentBudget := cfg.MaxBytes - len(`{"_memoh_truncated":true,"content":""}`)
	if contentBudget <= 0 {
		return map[string]any{"_memoh_truncated": true}
	}
	for contentBudget > 0 {
		contentCfg := toolOutputPruneConfig(ToolOutputLimit{
			MaxBytes: contentBudget,
			MaxLines: cfg.MaxLines,
		})
		result := map[string]any{
			"_memoh_truncated": true,
			"content":          textprune.PruneWithEdges(raw, label, contentCfg),
		}
		if !toolOutputExceeds(result, cfg) {
			return result
		}
		contentBudget = contentBudget * 3 / 4
	}
	return map[string]any{"_memoh_truncated": true}
}

func marshalToolOutputString(value any) string {
	raw, err := json.Marshal(value)
	if err == nil {
		return string(raw)
	}
	return fmt.Sprint(value)
}

func toolOutputPruneConfig(limit ToolOutputLimit) textprune.Config {
	maxBytes := limit.MaxBytes
	if maxBytes <= 0 {
		maxBytes = textprune.DefaultMaxBytes
	}
	maxLines := limit.MaxLines
	if maxLines <= 0 {
		maxLines = textprune.DefaultMaxLines
	}
	headBytes, tailBytes := fitHeadTail(maxBytes, toolOutputHeadBytes, toolOutputTailBytes)
	headLines, tailLines := fitHeadTail(maxLines, toolOutputHeadLines, toolOutputTailLines)
	return textprune.Config{
		MaxBytes:  maxBytes,
		MaxLines:  maxLines,
		HeadBytes: headBytes,
		TailBytes: tailBytes,
		HeadLines: headLines,
		TailLines: tailLines,
		Marker:    textprune.DefaultMarker,
	}
}

func fitHeadTail(max, head, tail int) (int, int) {
	if max <= 0 {
		return head, tail
	}
	if head+tail <= max {
		return head, tail
	}
	head = max * 3 / 4
	tail = max - head
	if head <= 0 {
		return max, 0
	}
	return head, tail
}
