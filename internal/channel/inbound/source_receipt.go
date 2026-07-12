package inbound

import (
	"encoding/json"
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
	origin, err := buildInboundSourceEnvelope(identity, msg, eventID)
	if err != nil {
		return conversation.UserMessageReceipt{}, err
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
	attachmentSnapshot, err := snapshotInboundAttachments(attachments)
	if err != nil {
		return conversation.UserMessageReceipt{}, err
	}
	return conversation.UserMessageReceipt{
		ID:          uuid.NewString(),
		DisplayText: strings.TrimSpace(text),
		Origin:      origin,
		Metadata:    metadata,
		Attachments: attachmentSnapshot,
	}, nil
}

func buildInboundSourceEnvelope(
	identity InboundIdentity,
	msg channel.InboundMessage,
	eventID string,
) (messagesource.Envelope, error) {
	origin, err := messagesource.NewEnvelope(messagesource.EnvelopeInput{
		SenderChannelIdentityID: identity.ChannelIdentityID,
		SenderUserID:            identity.UserID,
		ExternalMessageID:       msg.Message.ID,
		SourceReplyToMessageID:  inboundReplyMessageID(msg.Message.Reply),
		EventID:                 eventID,
		Source: messagesource.V1Candidate{
			SenderDisplayName: identity.DisplayName,
			Platform:          msg.Channel.String(),
			ConversationType:  canonicalSourceConversationType(msg.Conversation.Type),
			ConversationName:  msg.Conversation.Name,
		},
	})
	if err != nil {
		return messagesource.Envelope{}, fmt.Errorf("build inbound source envelope: %w", err)
	}
	return origin, nil
}

func bindInboundReceiptEventID(
	receipt conversation.UserMessageReceipt,
	eventID string,
) (conversation.UserMessageReceipt, error) {
	if strings.TrimSpace(eventID) == "" {
		return receipt, nil
	}
	values := receipt.Origin.Values()
	origin, err := messagesource.NewEnvelope(messagesource.EnvelopeInput{
		SenderChannelIdentityID: values.SenderChannelIdentityID,
		SenderUserID:            values.SenderUserID,
		ExternalMessageID:       values.ExternalMessageID,
		SourceReplyToMessageID:  values.SourceReplyToMessageID,
		EventID:                 eventID,
		Source: messagesource.V1Candidate{
			SenderDisplayName: values.Context.SenderDisplayName,
			Platform:          values.Context.Platform,
			ConversationType:  values.Context.ConversationType,
			ConversationName:  values.Context.ConversationName,
		},
	})
	if err != nil {
		return receipt, fmt.Errorf("bind inbound receipt event: %w", err)
	}
	receipt.Origin = origin
	return receipt, nil
}

func snapshotInboundAttachments(attachments []conversation.ChatAttachment) ([]conversation.ChatAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	snapshot := make([]conversation.ChatAttachment, len(attachments))
	for i, attachment := range attachments {
		snapshot[i] = attachment
		if len(attachment.Metadata) == 0 {
			snapshot[i].Metadata = nil
			continue
		}
		raw, err := json.Marshal(attachment.Metadata)
		if err != nil {
			return nil, fmt.Errorf("snapshot inbound attachment %d metadata: %w", i, err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("snapshot inbound attachment %d metadata: %w", i, err)
		}
		snapshot[i].Metadata = metadata
	}
	return snapshot, nil
}

func canonicalSourceConversationType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "p2p", "direct", channel.ConversationTypePrivate:
		return channel.ConversationTypePrivate
	case channel.ConversationTypeGroup:
		return channel.ConversationTypeGroup
	case channel.ConversationTypeThread:
		return channel.ConversationTypeThread
	default:
		return ""
	}
}
