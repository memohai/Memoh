package channel

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrStopNotSupported = errors.New("channel connection stop not supported")

type InboundHandler func(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error

type ReplySender interface {
	Send(ctx context.Context, msg OutboundMessage) error
}

type Adapter interface {
	Type() ChannelType
}

type Sender interface {
	Send(ctx context.Context, cfg ChannelConfig, msg OutboundMessage) error
}

type Receiver interface {
	Connect(ctx context.Context, cfg ChannelConfig, handler InboundHandler) (Connection, error)
}

type Connection interface {
	ConfigID() string
	BotID() string
	ChannelType() ChannelType
	Stop(ctx context.Context) error
	Running() bool
}

type BaseConnection struct {
	configID    string
	botID       string
	channelType ChannelType
	stop        func(ctx context.Context) error
	running     atomic.Bool
}

func NewConnection(cfg ChannelConfig, stop func(ctx context.Context) error) *BaseConnection {
	conn := &BaseConnection{
		configID:    cfg.ID,
		botID:       cfg.BotID,
		channelType: cfg.ChannelType,
		stop:        stop,
	}
	conn.running.Store(true)
	return conn
}

func (c *BaseConnection) ConfigID() string {
	return c.configID
}

func (c *BaseConnection) BotID() string {
	return c.botID
}

func (c *BaseConnection) ChannelType() ChannelType {
	return c.channelType
}

func (c *BaseConnection) Stop(ctx context.Context) error {
	if c.stop == nil {
		return ErrStopNotSupported
	}
	err := c.stop(ctx)
	if err == nil {
		c.running.Store(false)
	}
	return err
}

func (c *BaseConnection) Running() bool {
	return c.running.Load()
}
