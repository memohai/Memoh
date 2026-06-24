package flow

import (
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messageconv"
)

// sdkMessagesToModelMessages converts SDK messages to the persistence/API format
// at the resolver boundary. This is the only place where this conversion should happen.
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

// modelMessagesToSDKMessages converts a slice of persistence messages to SDK messages.
func modelMessagesToSDKMessages(msgs []conversation.ModelMessage) []sdk.Message {
	return messageconv.ModelMessagesToSDKMessages(msgs)
}
