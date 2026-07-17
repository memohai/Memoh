package pipeline

import (
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"
)

const syntheticToolClosureReason = "tool execution interrupted before a response was recorded"

type sdkContextMessage struct {
	Message                   sdk.Message
	CompactionArtifactID      string
	RenderedMessageIDs        []string
	ExternalEventCursors      []int64
	LatestExternalEventCursor int64
}

func repairSDKToolClosures(messages []sdk.Message) []sdk.Message {
	entries := make([]sdkContextMessage, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, sdkContextMessage{Message: message})
	}
	return sdkMessagesFromContextEntries(repairSDKContextToolClosures(entries))
}

func repairSDKContextToolClosures(entries []sdkContextMessage) []sdkContextMessage {
	if len(entries) == 0 {
		return entries
	}
	repaired := make([]sdkContextMessage, 0, len(entries))
	pending := make(map[string]string)
	pendingOrder := make([]string, 0)
	flushPending := func() {
		for _, callID := range pendingOrder {
			toolName, ok := pending[callID]
			if !ok {
				continue
			}
			repaired = append(repaired, sdkContextMessage{
				Message: sdk.ToolMessage(sdk.ToolResultPart{
					ToolCallID: callID,
					ToolName:   toolName,
					Result:     syntheticToolClosureReason,
					IsError:    true,
				}),
			})
			delete(pending, callID)
		}
		pendingOrder = pendingOrder[:0]
	}

	for _, entry := range entries {
		message := entry.Message
		switch message.Role {
		case sdk.MessageRoleAssistant:
			flushPending()
			repaired = append(repaired, entry)
			for _, part := range message.Content {
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
			kept := make([]sdk.MessagePart, 0, len(message.Content))
			for _, part := range message.Content {
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
			entry.Message.Content = kept
			repaired = append(repaired, entry)
		default:
			flushPending()
			repaired = append(repaired, entry)
		}
	}
	flushPending()
	return repaired
}
