package conversation

import "context"

// Reader defines conversation lookup behavior.
type Reader interface {
	Get(ctx context.Context, conversationID string) (Conversation, error)
}

// ParticipantChecker defines participant membership checks.
type ParticipantChecker interface {
	IsParticipant(ctx context.Context, conversationID, channelIdentityID string) (bool, error)
}

// Accessor defines read access checks for conversation-scoped operations.
type Accessor interface {
	Reader
	ParticipantChecker
	GetReadAccess(ctx context.Context, conversationID, channelIdentityID string) (ConversationReadAccess, error)
}
