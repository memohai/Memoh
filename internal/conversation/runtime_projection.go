package conversation

import (
	"encoding/json"
	"strings"

	messagepkg "github.com/memohai/memoh/internal/message"
)

const RuntimePersistedProjectionMetadataKey = "_memoh_runtime_persisted_projection"

// RuntimePersistedProjection is the ownership handoff payload from durable
// history to session runtime. It is built from the rows that actually
// committed, so terminal runtime state and the next REST read share identity.
type RuntimePersistedProjection struct {
	RequestUserTurn   *UITurn         `json:"request_user_turn,omitempty"`
	AssistantMessages []UIMessage     `json:"assistant_messages,omitempty"`
	RowLedger         []UIRowIdentity `json:"row_ledger"`
}

func NewRuntimePersistedProjection(messages []messagepkg.Message) RuntimePersistedProjection {
	projection := RuntimePersistedProjection{RowLedger: []UIRowIdentity{}}
	for _, message := range messages {
		if strings.TrimSpace(message.ID) == "" || message.TurnPosition <= 0 || message.TurnMessageSeq <= 0 {
			continue
		}
		projection.RowLedger = append(projection.RowLedger, UIRowIdentity{
			StableID:       message.ID,
			Role:           message.Role,
			TurnID:         message.TurnID,
			TurnPosition:   message.TurnPosition,
			TurnMessageSeq: message.TurnMessageSeq,
		})
	}
	for _, turn := range ConvertMessagesToUITurns(messages) {
		switch strings.ToLower(strings.TrimSpace(turn.Role)) {
		case "user":
			if projection.RequestUserTurn == nil {
				turnCopy := turn
				projection.RequestUserTurn = &turnCopy
			}
		case "assistant":
			projection.AssistantMessages = append([]UIMessage(nil), turn.Messages...)
		}
	}
	return projection
}

func RuntimePersistedProjectionFromMetadata(metadata map[string]any) (RuntimePersistedProjection, bool) {
	if len(metadata) == 0 {
		return RuntimePersistedProjection{}, false
	}
	raw, ok := metadata[RuntimePersistedProjectionMetadataKey]
	if !ok {
		return RuntimePersistedProjection{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return RuntimePersistedProjection{}, false
	}
	var projection RuntimePersistedProjection
	if err := json.Unmarshal(data, &projection); err != nil {
		return RuntimePersistedProjection{}, false
	}
	return projection, true
}
