package channel

import "context"

// InboundProcessor handles inbound messages and replies through the given sender.
type InboundProcessor interface {
	HandleInbound(ctx context.Context, cfg Config, msg InboundMessage, sender StreamReplySender) error
}
