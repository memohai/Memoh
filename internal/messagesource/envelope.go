package messagesource

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Envelope struct {
	values EnvelopeValues
}

type EnvelopeInput struct {
	SenderChannelIdentityID string
	SenderUserID            string
	ExternalMessageID       string
	SourceReplyToMessageID  string
	EventID                 string
	Source                  V1Candidate
}

type EnvelopeValues struct {
	SenderChannelIdentityID string
	SenderUserID            string
	ExternalMessageID       string
	SourceReplyToMessageID  string
	EventID                 string
	Context                 Context
}

func NewEnvelope(input EnvelopeInput) (Envelope, error) {
	senderChannelIdentityID, err := normalizeOptionalEnvelopeUUID("sender channel identity id", input.SenderChannelIdentityID)
	if err != nil {
		return Envelope{}, err
	}
	senderUserID, err := normalizeOptionalEnvelopeUUID("sender user id", input.SenderUserID)
	if err != nil {
		return Envelope{}, err
	}
	eventID, err := normalizeOptionalEnvelopeUUID("event id", input.EventID)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{values: EnvelopeValues{
		SenderChannelIdentityID: senderChannelIdentityID,
		SenderUserID:            senderUserID,
		ExternalMessageID:       strings.TrimSpace(input.ExternalMessageID),
		SourceReplyToMessageID:  strings.TrimSpace(input.SourceReplyToMessageID),
		EventID:                 eventID,
		Context:                 completeV1(input.Source),
	}}, nil
}

func (envelope Envelope) Values() EnvelopeValues {
	return envelope.values
}

type V1Candidate struct {
	SenderDisplayName string
	Platform          string
	ConversationType  string
	ConversationName  string
}

func completeV1(candidate V1Candidate) Context {
	context := NewV1(
		candidate.SenderDisplayName,
		candidate.Platform,
		candidate.ConversationType,
		candidate.ConversationName,
	)
	if context.SenderDisplayName == "" || context.Platform == "" || context.ConversationName == "" {
		return Context{}
	}
	switch context.ConversationType {
	case "private", "group", "thread":
		return context
	default:
		return Context{}
	}
}

func normalizeOptionalEnvelopeUUID(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed.String(), nil
}
