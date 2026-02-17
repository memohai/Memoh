package channel

import (
	"context"
	"errors"
	"log/slog"
)

type inboundTask struct {
	ctx context.Context
	cfg Config
	msg InboundMessage
}

// HandleInbound enqueues an inbound message for asynchronous processing by the worker pool.
func (m *Manager) HandleInbound(ctx context.Context, cfg Config, msg InboundMessage) error {
	if m.processor == nil {
		return errors.New("inbound processor not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.startInboundWorkers(ctx)
	if m.inboundCtx != nil && m.inboundCtx.Err() != nil {
		return errors.New("inbound dispatcher stopped")
	}
	task := inboundTask{
		ctx: context.WithoutCancel(ctx),
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

func (m *Manager) handleInbound(ctx context.Context, cfg Config, msg InboundMessage) error {
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
		workerCtx := ctx
		if workerCtx == nil {
			workerCtx = context.Background()
		}
		m.inboundCtx, m.inboundCancel = context.WithCancel(workerCtx)
		for i := 0; i < m.inboundWorkers; i++ {
			go m.runInboundWorker(m.inboundCtx)
		}
	})
}

func (m *Manager) runInboundWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-m.inboundQueue:
			if err := m.handleInbound(task.ctx, task.cfg, task.msg); err != nil {
				if m.logger != nil {
					m.logger.Error("inbound processing failed", slog.String("channel", task.msg.Channel.String()), slog.Any("error", err))
				}
			}
		}
	}
}
