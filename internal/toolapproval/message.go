package toolapproval

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

const (
	ActionTypeToolApproval = "tool_approval"
	ActionApprove          = "approve"
	ActionReject           = "reject"
)

func BuildPrompt(req Request) channel.Message {
	summary := summarizeInput(req)
	text := fmt.Sprintf(
		"Tool approval required #%d\nTool: %s\nInput: %s\n\nApprove with /approve %d or reply to this message with /approve.\nReject with /reject %d [reason] or reply with /reject [reason].",
		req.ShortID,
		req.ToolName,
		summary,
		req.ShortID,
		req.ShortID,
	)
	return channel.Message{
		Format: channel.MessageFormatPlain,
		Text:   text,
		Actions: []channel.Action{
			{Type: ActionTypeToolApproval, Label: "Approve", Value: ActionApprove + ":" + req.ID},
			{Type: ActionTypeToolApproval, Label: "Reject", Value: ActionReject + ":" + req.ID},
		},
		Metadata: map[string]any{
			"tool_approval_id":       req.ID,
			"tool_approval_short_id": req.ShortID,
			"tool_call_id":           req.ToolCallID,
		},
	}
}

func summarizeInput(req Request) string {
	switch req.ToolName {
	case "write", "edit":
		if path, ok := req.ToolInput["path"].(string); ok && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path)
		}
	case "exec":
		if command, ok := req.ToolInput["command"].(string); ok && strings.TrimSpace(command) != "" {
			return strings.TrimSpace(command)
		}
	}
	data, err := json.Marshal(req.ToolInput)
	if err != nil {
		return "{}"
	}
	text := string(data)
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}

func ParseActionValue(value string) (decision, approvalID string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	switch parts[0] {
	case ActionApprove, ActionReject:
		return parts[0], strings.TrimSpace(parts[1]), strings.TrimSpace(parts[1]) != ""
	default:
		return "", "", false
	}
}
