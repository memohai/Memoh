package flow

import (
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
)

// sdkMessagesToModelMessages converts SDK messages to the persistence/API format
// for resolver call sites using the shared conversion helper.
func sdkMessagesToModelMessages(msgs []sdk.Message) []conversation.ModelMessage {
	return messageconv.SDKMessagesToModelMessages(msgs)
}

// modelMessageToSDKMessage converts a persistence format message to SDK message
// at the resolver boundary using sdk.Message's native JSON deserialization.
func modelMessageToSDKMessage(mm conversation.ModelMessage) sdk.Message {
	return messageconv.ModelMessageToSDKMessage(mm)
}

// prependUserMessage prepends the user query as a ModelMessage to the output
// messages from the agent. The SDK only returns output messages (assistant + tool);
// user messages must be added back at the resolver boundary for persistence.
func prependUserMessage(query string, output []conversation.ModelMessage) []conversation.ModelMessage {
	return messageconv.PrependUserMessage(query, output)
}

func prependTurnUserMessage(req conversation.ChatRequest, output []conversation.ModelMessage) []conversation.ModelMessage {
	if strings.TrimSpace(req.Query) == "" && req.UserMessageKind != conversation.UserMessageKindSkillActivation {
		return output
	}
	round := make([]conversation.ModelMessage, 0, 1+len(output))
	user := conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(req.Query),
	}
	if req.RuntimeTurn != nil {
		row := req.RuntimeTurn.Request
		user.RuntimeRow = &row
	}
	round = append(round, user)
	return append(round, output...)
}

func modelQueryText(req conversation.ChatRequest) string {
	if strings.TrimSpace(req.ModelQuery) != "" {
		return req.ModelQuery
	}
	return req.Query
}

// modelMessagesToSDKMessages converts a slice of persistence messages to SDK messages.
func modelMessagesToSDKMessages(msgs []conversation.ModelMessage) []sdk.Message {
	return messageconv.ModelMessagesToSDKMessages(msgs)
}
