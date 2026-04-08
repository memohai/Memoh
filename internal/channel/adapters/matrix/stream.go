package matrix

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type matrixOutboundStream struct {
	adapter *MatrixAdapter
	cfg     Config
	target  string
	reply   *channel.ReplyRef

	closed atomic.Bool
	mu     sync.Mutex

	roomID          string
	originalEventID string
	rawBuffer       strings.Builder
	lastText        string
	lastFormat      channel.MessageFormat
	lastEditedAt    time.Time
}

func (s *matrixOutboundStream) Push(ctx context.Context, event channel.PreparedStreamEvent) error {
	if s == nil || s.adapter == nil {
		return errors.New("matrix stream not configured")
	}
	if s.closed.Load() {
		return errors.New("matrix stream is closed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch event.Type {
	case channel.StreamEventStatus,
		channel.StreamEventPhaseStart,
		channel.StreamEventToolCallEnd,
		channel.StreamEventAgentStart,
		channel.StreamEventAgentEnd,
		channel.StreamEventProcessingStarted,
		channel.StreamEventProcessingCompleted,
		channel.StreamEventProcessingFailed:
		return nil
	case channel.StreamEventPhaseEnd:
		if event.Phase != channel.StreamPhaseText {
			return nil
		}
		s.mu.Lock()
		text := strings.TrimSpace(s.rawBuffer.String())
		s.mu.Unlock()
		return s.upsertText(ctx, text, channel.MessageFormatPlain, true)
	case channel.StreamEventToolCallStart:
		s.resetMessageState()
		return nil
	case channel.StreamEventDelta:
		if event.Phase == channel.StreamPhaseReasoning || event.Delta == "" {
			return nil
		}
		s.mu.Lock()
		s.rawBuffer.WriteString(event.Delta)
		s.mu.Unlock()
		return nil
	case channel.StreamEventError:
		errText := strings.TrimSpace(event.Error)
		if errText == "" {
			return nil
		}
		return s.upsertText(ctx, "Error: "+errText, channel.MessageFormatPlain, true)
	case channel.StreamEventAttachment:
		return s.pushAttachments(ctx, event.Attachments)
	case channel.StreamEventFinal:
		if event.Final == nil {
			return errors.New("matrix stream final payload is required")
		}
		text := strings.TrimSpace(event.Final.Message.Message.PlainText())
		format := event.Final.Message.Message.Format
		if format == "" {
			format = channel.MessageFormatPlain
		}
		if text == "" {
			s.mu.Lock()
			text = strings.TrimSpace(s.rawBuffer.String())
			s.mu.Unlock()
		}
		if err := s.upsertText(ctx, text, format, true); err != nil {
			return err
		}
		if err := s.pushAttachments(ctx, event.Final.Message.Attachments); err != nil {
			return err
		}
		s.resetMessageState()
		return nil
	default:
		return nil
	}
}

func (s *matrixOutboundStream) Close(ctx context.Context) error {
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

func (s *matrixOutboundStream) upsertText(ctx context.Context, text string, format channel.MessageFormat, force bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if format == "" {
		format = channel.MessageFormatPlain
	}

	s.mu.Lock()
	roomID := s.roomID
	originalEventID := s.originalEventID
	lastText := s.lastText
	lastFormat := s.lastFormat
	lastEditedAt := s.lastEditedAt
	reply := s.reply
	s.mu.Unlock()

	if roomID == "" {
		resolvedRoomID, err := s.adapter.resolveRoomTarget(ctx, s.cfg, s.target)
		if err != nil {
			return err
		}
		roomID = resolvedRoomID
		s.mu.Lock()
		s.roomID = resolvedRoomID
		s.mu.Unlock()
	}

	if originalEventID == "" {
		eventID, err := s.adapter.sendTextEvent(ctx, s.cfg, roomID, buildMatrixMessageContent(channel.Message{
			Text:   text,
			Format: format,
			Reply:  reply,
		}, false, ""))
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.originalEventID = eventID
		s.lastText = text
		s.lastFormat = format
		s.lastEditedAt = time.Now()
		s.mu.Unlock()
		return nil
	}

	if text == lastText && format == lastFormat {
		return nil
	}
	if !force && time.Since(lastEditedAt) < matrixEditThrottle {
		return nil
	}
	_, err := s.adapter.sendTextEvent(ctx, s.cfg, roomID, buildMatrixMessageContent(channel.Message{
		Text:   text,
		Format: format,
	}, true, originalEventID))
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.lastText = text
	s.lastFormat = format
	s.lastEditedAt = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *matrixOutboundStream) resetMessageState() {
	s.mu.Lock()
	s.originalEventID = ""
	s.rawBuffer.Reset()
	s.lastText = ""
	s.lastFormat = ""
	s.lastEditedAt = time.Time{}
	s.mu.Unlock()
}

func (s *matrixOutboundStream) pushAttachments(ctx context.Context, attachments []channel.PreparedAttachment) error {
	if len(attachments) == 0 {
		return nil
	}

	s.mu.Lock()
	roomID := s.roomID
	originalEventID := s.originalEventID
	reply := s.reply
	s.mu.Unlock()

	if roomID == "" {
		resolvedRoomID, err := s.adapter.resolveRoomTarget(ctx, s.cfg, s.target)
		if err != nil {
			return err
		}
		roomID = resolvedRoomID
		s.mu.Lock()
		s.roomID = resolvedRoomID
		s.mu.Unlock()
	}

	for idx, att := range attachments {
		mediaMsg := channel.Message{}
		if idx == 0 && originalEventID == "" {
			mediaMsg.Reply = reply
		}
		if err := s.adapter.sendMediaAttachment(ctx, s.cfg, roomID, mediaMsg, att); err != nil {
			return err
		}
	}
	return nil
}
