package inbound

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messagesource"
)

func buildInboundUserReceipt(
	identity InboundIdentity,
	msg channel.InboundMessage,
	text string,
	attachments []conversation.ChatAttachment,
	routeID string,
	eventID string,
) (conversation.UserMessageReceipt, error) {
	origin, err := messagesource.NewEnvelope(messagesource.EnvelopeInput{
		SenderChannelIdentityID: identity.ChannelIdentityID,
		SenderUserID:            identity.UserID,
		ExternalMessageID:       msg.Message.ID,
		SourceReplyToMessageID:  inboundReplyMessageID(msg.Message.Reply),
		EventID:                 eventID,
		Source: messagesource.V1Candidate{
			SenderDisplayName: identity.DisplayName,
			Platform:          msg.Channel.String(),
			ConversationType:  channel.NormalizeConversationType(msg.Conversation.Type),
			ConversationName:  msg.Conversation.Name,
		},
	})
	if err != nil {
		return conversation.UserMessageReceipt{}, fmt.Errorf("build inbound source envelope: %w", err)
	}
	metadata := map[string]any{}
	if routeID := strings.TrimSpace(routeID); routeID != "" {
		metadata["route_id"] = routeID
	}
	if platform := strings.TrimSpace(msg.Channel.String()); platform != "" {
		metadata["platform"] = platform
	}
	if reply := messageReplyMetadata(msg.Message.Reply); reply != nil {
		metadata["reply"] = reply
	}
	if forward := messageForwardMetadata(msg.Message.Forward); forward != nil {
		metadata["forward"] = forward
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return conversation.UserMessageReceipt{
		ID:          uuid.NewString(),
		DisplayText: strings.TrimSpace(text),
		Origin:      origin,
		Metadata:    metadata,
		Attachments: append([]conversation.ChatAttachment(nil), attachments...),
	}, nil
}
