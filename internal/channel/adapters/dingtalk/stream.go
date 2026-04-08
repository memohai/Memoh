package dingtalk

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/memohai/memoh/internal/channel"
)

// dingtalkOutboundStream accumulates streaming events and flushes the final message
// to DingTalk when closed. DingTalk has no native streaming API, so the stream
// is buffered and sent as a single message on Close.
type dingtalkOutboundStream struct {
	adapter *DingTalkAdapter
	cfg     channel.ChannelConfig
	target  string
	reply   *channel.ReplyRef

	mu          sync.Mutex
	closed      atomic.Bool
	finalSent   atomic.Bool
	textBuilder strings.Builder
	attachments []channel.Attachment
	final       *channel.Message
}

func (s *dingtalkOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if s.closed.Load() {
		return errors.New("dingtalk stream is closed")
	}
	if s.finalSent.Load() {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch event.Type {
	case channel.StreamEventStatus,
		channel.StreamEventPhaseStart,
		channel.StreamEventPhaseEnd,
		channel.StreamEventToolCallStart,
		channel.StreamEventToolCallEnd,
		channel.StreamEventAgentStart,
		channel.StreamEventAgentEnd,
		channel.StreamEventProcessingStarted,
		channel.StreamEventProcessingCompleted,
		channel.StreamEventProcessingFailed:
		// Non-content events: no-op.
		return nil

	case channel.StreamEventDelta:
		if strings.TrimSpace(event.Delta) == "" || event.Phase == channel.StreamPhaseReasoning {
			return nil
		}
		s.mu.Lock()
		s.textBuilder.WriteString(event.Delta)
		s.mu.Unlock()
		return nil

	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return nil
		}
		s.mu.Lock()
		s.attachments = append(s.attachments, event.Attachments...)
		s.mu.Unlock()
		return nil

	case channel.StreamEventFinal:
		if event.Final == nil {
			return nil
		}
		s.mu.Lock()
		final := event.Final.Message
		s.final = &final
		s.mu.Unlock()
		return s.flush(ctx)

	case channel.StreamEventError:
		text := strings.TrimSpace(event.Error)
		if text == "" {
			return nil
		}
		s.mu.Lock()
		s.final = &channel.Message{Format: channel.MessageFormatPlain, Text: "Error: " + text}
		s.mu.Unlock()
		return s.flush(ctx)
	}
	return nil
}

func (s *dingtalkOutboundStream) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.closed.Store(true)
	if s.finalSent.Load() {
		return nil
	}
	return s.flush(ctx)
}

func (s *dingtalkOutboundStream) flush(ctx context.Context) error {
	if s.finalSent.Load() {
		return nil
	}
	msg := s.snapshotMessage()
	if msg.IsEmpty() {
		return nil
	}
	if err := s.adapter.Send(ctx, s.cfg, channel.OutboundMessage{
		Target:  s.target,
		Message: msg,
	}); err != nil {
		return err
	}
	s.finalSent.Store(true)
	return nil
}

func (s *dingtalkOutboundStream) snapshotMessage() channel.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := channel.Message{}
	if s.final != nil {
		msg = *s.final
	}
	if strings.TrimSpace(msg.Text) == "" {
		msg.Text = strings.TrimSpace(s.textBuilder.String())
	}
	if len(msg.Attachments) == 0 && len(s.attachments) > 0 {
		msg.Attachments = append(msg.Attachments, s.attachments...)
	}
	if msg.Reply == nil && s.reply != nil {
		msg.Reply = s.reply
	}
	return msg
}
