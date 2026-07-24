package turn

import "strings"

const (
	ConversationTypePrivate = "private"
	ConversationTypeGroup   = "group"
	ConversationTypeThread  = "thread"
)

// NormalizeConversationType normalizes delivery-specific aliases into the
// application-level conversation vocabulary carried by turn commands.
func NormalizeConversationType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "p2p", "direct", ConversationTypePrivate:
		return ConversationTypePrivate
	case ConversationTypeThread:
		return ConversationTypeThread
	case ConversationTypeGroup:
		return ConversationTypeGroup
	default:
		return ConversationTypeGroup
	}
}

// IsPrivateConversationType reports whether raw describes a direct
// conversation.
func IsPrivateConversationType(raw string) bool {
	return NormalizeConversationType(raw) == ConversationTypePrivate
}
