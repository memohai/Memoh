package qq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/memohai/memoh/internal/channel"
)

type qqOutboundStream struct {
	target string
	reply  *channel.ReplyRef
	send   func(context.Context, channel.OutboundMessage) error

	closed      atomic.Bool
	mu          sync.Mutex
	buffer      strings.Builder
	attachments []channel.Attachment
	sentText    bool
}

func (a *QQAdapter) OpenStream(_ context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, fmt.Errorf("qq open stream: %w", err)
	}
	channel.RegisterIMErrorSecrets(parsed.AppSecret)
	return &qqOutboundStream{
		target: target,
		reply:  opts.Reply,
		send: func(ctx context.Context, msg channel.OutboundMessage) error {
			if msg.Target == "" {
				msg.Target = target
			}
			if msg.Message.Reply == nil && opts.Reply != nil {
				msg.Message.Reply = opts.Reply
			}
			return a.Send(ctx, cfg, msg)
		},
	}, nil
}

func (s *qqOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if s == nil || s.send == nil {
		return errors.New("qq stream not configured")
	}
	if s.closed.Load() {
		return errors.New("qq stream is closed")
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
		return nil
	case channel.StreamEventDelta:
		if event.Phase == channel.StreamPhaseReasoning || event.Delta == "" {
			return nil
		}
		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
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
	case channel.StreamEventError:
		errText := channel.RedactIMErrorText(strings.TrimSpace(event.Error))
		if errText == "" {
			return nil
		}
		return s.flush(ctx, channel.Message{
			Text: "Error: " + errText,
		})
	case channel.StreamEventFinal:
		if event.Final == nil {
			return errors.New("qq stream final payload is required")
		}
		return s.flush(ctx, event.Final.Message)
	default:
		return nil
	}
}

func (s *qqOutboundStream) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.closed.Store(true)
	return nil
}

func (s *qqOutboundStream) flush(ctx context.Context, msg channel.Message) error {
	s.mu.Lock()
	bufferedText := strings.TrimSpace(s.buffer.String())
	bufferedAttachments := append([]channel.Attachment(nil), s.attachments...)
	alreadySentText := s.sentText
	s.buffer.Reset()
	s.attachments = nil
	s.mu.Unlock()

	if bufferedText != "" {
		msg.Text = bufferedText
		msg.Parts = nil
		if msg.Format == "" {
			msg.Format = channel.MessageFormatPlain
		}
	} else if alreadySentText && len(bufferedAttachments) == 0 && len(msg.Attachments) == 0 && strings.TrimSpace(msg.PlainText()) != "" {
		return nil
	}
	if len(bufferedAttachments) > 0 {
		msg.Attachments = append(bufferedAttachments, msg.Attachments...)
	}
	if msg.Reply == nil && s.reply != nil {
		msg.Reply = s.reply
	}
	if msg.IsEmpty() {
		return nil
	}
	if err := s.send(ctx, channel.OutboundMessage{
		Target:  s.target,
		Message: msg,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(msg.PlainText()) != "" {
		s.mu.Lock()
		s.sentText = true
		s.mu.Unlock()
	}
	return nil
}
