package flow

import (
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
)

const syntheticToolClosureError = "tool execution interrupted before a response was recorded"

type repairedModelMessageStream struct {
	messages      []conversation.ModelMessage
	sourceIndexes []int
}

func repairToolCallClosures(messages []conversation.ModelMessage, reason string) []conversation.ModelMessage {
	return repairToolCallClosuresWithSources(messages, reason).messages
}

func repairToolCallClosuresWithSources(messages []conversation.ModelMessage, reason string) repairedModelMessageStream {
	if len(messages) == 0 {
		return repairedModelMessageStream{messages: messages}
	}
	repair := messageconv.RepairSDKToolOccurrences(messageconv.ModelMessagesToSDKMessages(messages), reason)
	repaired := repairedModelMessageStream{
		messages:      make([]conversation.ModelMessage, 0, len(repair.Entries)),
		sourceIndexes: make([]int, 0, len(repair.Entries)),
	}
	for _, entry := range repair.Entries {
		converted := messageconv.SDKMessagesToModelMessages([]sdk.Message{entry.Message})
		if len(converted) == 0 {
			continue
		}
		message := converted[0]
		if !entry.Synthetic && entry.SourceIndex >= 0 && entry.SourceIndex < len(messages) {
			message.Usage = messages[entry.SourceIndex].Usage
		}
		repaired.messages = append(repaired.messages, message)
		sourceIndex := -1
		if !entry.Synthetic {
			sourceIndex = entry.SourceIndex
		}
		repaired.sourceIndexes = append(repaired.sourceIndexes, sourceIndex)
	}
	return repaired
}

func extractAssistantToolCallParts(msg conversation.ModelMessage) []sdk.ToolCallPart {
	sdkMsg := modelMessageToSDKMessage(msg)
	if len(sdkMsg.Content) == 0 {
		return nil
	}
	calls := make([]sdk.ToolCallPart, 0, len(sdkMsg.Content))
	for _, part := range sdkMsg.Content {
		call, ok := part.(sdk.ToolCallPart)
		if !ok {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func extractToolResultParts(msg conversation.ModelMessage) []sdk.ToolResultPart {
	sdkMsg := modelMessageToSDKMessage(msg)
	if len(sdkMsg.Content) == 0 {
		return nil
	}
	results := make([]sdk.ToolResultPart, 0, len(sdkMsg.Content))
	for _, part := range sdkMsg.Content {
		result, ok := part.(sdk.ToolResultPart)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}
