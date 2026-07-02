package pipeline

import (
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"
)

const syntheticToolClosureReason = "tool execution interrupted before a response was recorded"

// repairSDKToolClosures keeps assistant tool-call / tool-result pairing intact
// after coverage filtering: orphan results are dropped and unanswered calls get
// a synthetic error result, matching the chat path's repairToolCallClosures
// guarantees required by provider serializers.
func repairSDKToolClosures(messages []sdk.Message) []sdk.Message {
	if len(messages) == 0 {
		return messages
	}
	repaired := make([]sdk.Message, 0, len(messages))
	pending := make(map[string]string)
	pendingOrder := make([]string, 0)

	flushPending := func() {
		for _, callID := range pendingOrder {
			toolName, ok := pending[callID]
			if !ok {
				continue
			}
			repaired = append(repaired, sdk.ToolMessage(sdk.ToolResultPart{
				ToolCallID: callID,
				ToolName:   toolName,
				Result:     syntheticToolClosureReason,
				IsError:    true,
			}))
			delete(pending, callID)
		}
		pendingOrder = pendingOrder[:0]
	}

	for _, msg := range messages {
		switch msg.Role {
		case sdk.MessageRoleAssistant:
			flushPending()
			repaired = append(repaired, msg)
			for _, part := range msg.Content {
				call, ok := part.(sdk.ToolCallPart)
				if !ok {
					continue
				}
				callID := strings.TrimSpace(call.ToolCallID)
				if callID == "" {
					continue
				}
				if _, exists := pending[callID]; exists {
					continue
				}
				pending[callID] = strings.TrimSpace(call.ToolName)
				pendingOrder = append(pendingOrder, callID)
			}
		case sdk.MessageRoleTool:
			kept := make([]sdk.MessagePart, 0, len(msg.Content))
			for _, part := range msg.Content {
				result, ok := part.(sdk.ToolResultPart)
				if !ok {
					kept = append(kept, part)
					continue
				}
				callID := strings.TrimSpace(result.ToolCallID)
				if _, matches := pending[callID]; !matches {
					continue
				}
				kept = append(kept, part)
				delete(pending, callID)
			}
			if len(kept) == 0 {
				continue
			}
			msg.Content = kept
			repaired = append(repaired, msg)
		default:
			repaired = append(repaired, msg)
		}
	}
	flushPending()
	return repaired
}
