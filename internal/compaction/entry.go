package compaction

import (
	"encoding/json"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
)

// entryPart is a minimal view of one stored content part, enough to render a
// summarizer-friendly entry without leaking raw JSON, media payloads, or
// oversized tool blobs.
type entryPart struct {
	Type     string          `json:"type"`
	ToolName string          `json:"toolName"`
	Output   json.RawMessage `json:"output"`
	Result   json.RawMessage `json:"result"`
}

// renderEntryContent turns a stored history message into clean text for the
// summarizer prompt: plain text is surfaced directly, media is reduced to a
// marker, tool calls show their name, and tool results show their outcome text
// (falling back to a marker) instead of dumping the raw stored JSON.
func renderEntryContent(mm conversation.ModelMessage) string {
	var segs []string
	if text := strings.TrimSpace(mm.TextContent()); text != "" {
		segs = append(segs, text)
	}

	sawToolCallPart := false
	for _, p := range parseEntryParts(mm.Content) {
		switch {
		case p.Type == "image":
			segs = append(segs, "[image]")
		case p.Type == "file":
			segs = append(segs, "[file]")
		case strings.Contains(p.Type, "tool-call"), strings.Contains(p.Type, "tool_call"):
			sawToolCallPart = true
			segs = append(segs, toolCallMarker(p.ToolName))
		case strings.Contains(p.Type, "tool-result"), strings.Contains(p.Type, "tool_result"):
			if out := firstOutputText(p.Output, p.Result); out != "" {
				segs = append(segs, out)
			} else {
				segs = append(segs, "[tool result]")
			}
		}
	}

	if !sawToolCallPart {
		for _, tc := range mm.ToolCalls {
			segs = append(segs, toolCallMarker(tc.Function.Name))
		}
	}

	return strings.Join(segs, "\n")
}

func parseEntryParts(content json.RawMessage) []entryPart {
	if len(content) == 0 {
		return nil
	}
	var parts []entryPart
	if err := json.Unmarshal(content, &parts); err != nil {
		return nil
	}
	return parts
}

func toolCallMarker(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "[tool_call]"
	}
	return "[tool_call: " + name + "]"
}

// firstOutputText returns the first extractable string from a tool result's
// output or result field. It only returns recognized string shapes so binary or
// structured payloads never leak into the summarizer input.
func firstOutputText(candidates ...json.RawMessage) string {
	for _, raw := range candidates {
		if s := outputText(raw); s != "" {
			return s
		}
	}
	return ""
}

func outputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		Type  string `json:"type"`
		Value string `json:"value"`
		Text  string `json:"text"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if v := strings.TrimSpace(obj.Value); v != "" {
			return v
		}
		if v := strings.TrimSpace(obj.Text); v != "" {
			return v
		}
	}
	return ""
}
