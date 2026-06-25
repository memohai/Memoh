package compaction

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/memohai/memoh/internal/conversation"
)

// toolOutputMaxBytes bounds the tool-result outcome rendered into the summarizer
// input. Stored tool payloads are already gateway-pruned; this keeps the summary
// input compact while preserving the outcome gist.
const toolOutputMaxBytes = 2048

var dataURIRe = regexp.MustCompile(`data:[a-zA-Z0-9.+/-]+;base64,[A-Za-z0-9+/=\s]+`)

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
			segs = append(segs, renderToolResult(p.Output, p.Result))
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

// renderToolResult surfaces a tool result's outcome for the summarizer: a clean
// string when one can be extracted, otherwise a bounded, media-scrubbed
// serialization of the raw outcome so real output (stdout, search results,
// structured data) survives. Falls back to a marker only when there is nothing.
func renderToolResult(candidates ...json.RawMessage) string {
	if s := firstOutputText(candidates...); s != "" {
		return s
	}
	if s := compactToolOutput(candidates...); s != "" {
		return s
	}
	return "[tool result]"
}

// firstOutputText returns the first cleanly extractable string from a tool
// result's output or result field. It only returns recognized string shapes so
// binary or structured payloads never leak through this path.
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
		Value   string `json:"value"`
		Text    string `json:"text"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if v := strings.TrimSpace(obj.Value); v != "" {
			return v
		}
		if v := strings.TrimSpace(obj.Text); v != "" {
			return v
		}
		var texts []string
		for _, c := range obj.Content {
			if t := strings.TrimSpace(c.Text); t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// compactToolOutput renders the raw outcome as bounded, media-scrubbed text so
// structured tool results (maps, arrays) still reach the summarizer instead of
// being dropped, without leaking base64/data-URI payloads.
func compactToolOutput(candidates ...json.RawMessage) string {
	for _, raw := range candidates {
		s := strings.TrimSpace(string(raw))
		if s == "" || s == "null" {
			continue
		}
		s = dataURIRe.ReplaceAllString(s, "[media]")
		return truncateBytes(s, toolOutputMaxBytes)
	}
	return ""
}

func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut]) + " …[truncated]"
}
