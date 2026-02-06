package channel

import "context"

// InboundProcessor 负责处理入站消息并通过 sender 回传响应。
type InboundProcessor interface {
	HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage, sender ReplySender) error
}
