package channel

import (
	"context"
	"errors"
	"log/slog"
)

type inboundTask struct {
	cfg ChannelConfig
	msg InboundMessage
}

// HandleInbound enqueues an inbound message for asynchronous processing by the worker pool.
func (m *Manager) HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return errors.New("inbound processor not configured")
	}
	m.startInboundWorkers(ctx)
	if m.inboundCtx != nil && m.inboundCtx.Err() != nil {
		return errors.New("inbound dispatcher stopped")
	}
	task := inboundTask{
		cfg: cfg,
		msg: msg,
	}
	select {
	case m.inboundQueue <- task:
		return nil
	default:
		return errors.New("inbound queue full")
	}
}

func (m *Manager) handleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage) error {
	if m.processor == nil {
		return errors.New("inbound processor not configured")
	}
	sender := m.newReplySender(cfg, msg.Channel)
	if err := m.processor.HandleInbound(ctx, cfg, msg, sender); err != nil {
		if m.logger != nil {
			m.logger.Error("inbound processing failed", slog.String("channel", msg.Channel.String()), slog.Any("error", err))
		}
		return err
	}
	return nil
}

func (m *Manager) startInboundWorkers(ctx context.Context) {
	m.inboundOnce.Do(func() {
		workerCtx := context.WithoutCancel(ctx)
		inboundCtx, inboundCancel := context.WithCancel(workerCtx)
		m.inboundCtx, m.inboundCancel = inboundCtx, inboundCancel
		for i := 0; i < m.inboundWorkers; i++ {
			go m.runInboundWorker(inboundCtx)
		}
	})
}

func (m *Manager) runInboundWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-m.inboundQueue:
			if err := m.handleInbound(ctx, task.cfg, task.msg); err != nil {
				if m.logger != nil {
					m.logger.Error("inbound processing failed", slog.String("channel", task.msg.Channel.String()), slog.Any("error", err))
				}
			}
		}
	}
}
