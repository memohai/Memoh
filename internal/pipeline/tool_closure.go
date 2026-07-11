package pipeline

import (
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/messageconv"
)

const syntheticToolClosureReason = "tool execution interrupted before a response was recorded"

type sdkContextMessage struct {
	Message              sdk.Message
	CompactionArtifactID string
}

func repairSDKToolClosures(messages []sdk.Message) []sdk.Message {
	if len(messages) == 0 {
		return messages
	}
	repair := messageconv.RepairSDKToolOccurrences(messages, syntheticToolClosureReason)
	repaired := make([]sdk.Message, len(repair.Entries))
	for index, entry := range repair.Entries {
		repaired[index] = entry.Message
	}
	return repaired
}

func repairSDKContextToolClosures(entries []sdkContextMessage) []sdkContextMessage {
	if len(entries) == 0 {
		return entries
	}
	messages := make([]sdk.Message, len(entries))
	for index, entry := range entries {
		messages[index] = entry.Message
	}
	repair := messageconv.RepairSDKToolOccurrences(messages, syntheticToolClosureReason)
	repaired := make([]sdkContextMessage, 0, len(repair.Entries))
	for _, entry := range repair.Entries {
		contextMessage := sdkContextMessage{Message: entry.Message}
		if !entry.Synthetic && entry.SourceIndex >= 0 && entry.SourceIndex < len(entries) {
			contextMessage.CompactionArtifactID = entries[entry.SourceIndex].CompactionArtifactID
		}
		repaired = append(repaired, contextMessage)
	}
	return repaired
}
