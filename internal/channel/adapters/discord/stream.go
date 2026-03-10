package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/memohai/memoh/internal/channel"
)

type discordOutboundStream struct {
	adapter    *DiscordAdapter
	cfg        channel.ChannelConfig
	target     string
	reply      *channel.ReplyRef
	session    *discordgo.Session
	closed     atomic.Bool
	mu         sync.Mutex
	msgID      string
	buffer     strings.Builder
	lastUpdate time.Time
}

func (s *discordOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if s == nil || s.adapter == nil {
		return errors.New("discord stream not configured")
	}
	if s.closed.Load() {
		return errors.New("discord stream is closed")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch event.Type {
	case channel.StreamEventStatus:
		if event.Status == channel.StreamStatusStarted {
			return s.ensureMessage("Thinking...")
		}
		return nil

	case channel.StreamEventDelta:
		if event.Delta == "" || event.Phase == channel.StreamPhaseReasoning {
			return nil
		}
		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
		s.mu.Unlock()

		// Discord has strict rate limits, only update periodically
		if time.Since(s.lastUpdate) > 2*time.Second {
			return s.updateMessage()
		}
		return nil

	case channel.StreamEventFinal:
		s.mu.Lock()
		bufText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		finalText := bufText
		if finalText == "" && event.Final != nil && !event.Final.Message.IsEmpty() {
			finalText = strings.TrimSpace(event.Final.Message.PlainText())
		}
		if finalText != "" {
			return s.finalizeMessage(finalText)
		}
		return nil

	case channel.StreamEventError:
		errText := strings.TrimSpace(event.Error)
		if errText == "" {
			return nil
		}
		return s.finalizeMessage("Error: " + errText)

	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return nil
		}
		// Finalize current text message before sending attachments
		s.mu.Lock()
		finalText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		if finalText != "" {
			if err := s.finalizeMessage(finalText); err != nil {
				return err
			}
		}
		// Send attachments
		for _, att := range event.Attachments {
			if err := s.sendAttachment(ctx, att); err != nil {
				return err
			}
		}
		return nil

	case channel.StreamEventAgentStart, channel.StreamEventAgentEnd, channel.StreamEventPhaseStart, channel.StreamEventPhaseEnd, channel.StreamEventProcessingStarted, channel.StreamEventProcessingCompleted, channel.StreamEventProcessingFailed, channel.StreamEventToolCallStart, channel.StreamEventToolCallEnd:
		// Status events - no action needed for Discord
		return nil

	default:
		return fmt.Errorf("unsupported stream event type: %s", event.Type)
	}
}

func (s *discordOutboundStream) Close(ctx context.Context) error {
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

func (s *discordOutboundStream) ensureMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.msgID != "" {
		return nil
	}

	content := truncateDiscordText(text)

	var msg *discordgo.Message
	var err error
	if s.reply != nil && s.reply.MessageID != "" {
		msg, err = s.session.ChannelMessageSendReply(s.target, content, &discordgo.MessageReference{
			ChannelID: s.target,
			MessageID: s.reply.MessageID,
		})
	} else {
		msg, err = s.session.ChannelMessageSend(s.target, content)
	}
	if err != nil {
		return err
	}

	s.msgID = msg.ID
	s.lastUpdate = time.Now()
	return nil
}

func (s *discordOutboundStream) updateMessage() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.msgID == "" {
		return nil
	}

	content := s.buffer.String()
	if content == "" {
		return nil
	}

	content = truncateDiscordText(content)

	_, err := s.session.ChannelMessageEdit(s.target, s.msgID, content)
	if err != nil {
		return err
	}

	s.lastUpdate = time.Now()
	return nil
}

func (s *discordOutboundStream) finalizeMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	text = truncateDiscordText(text)

	if s.msgID == "" {
		var msg *discordgo.Message
		var err error
		if s.reply != nil && s.reply.MessageID != "" {
			msg, err = s.session.ChannelMessageSendReply(s.target, text, &discordgo.MessageReference{
				ChannelID: s.target,
				MessageID: s.reply.MessageID,
			})
		} else {
			msg, err = s.session.ChannelMessageSend(s.target, text)
		}
		if err != nil {
			return err
		}
		s.msgID = msg.ID
		s.lastUpdate = time.Now()
		return nil
	}

	_, err := s.session.ChannelMessageEdit(s.target, s.msgID, text)
	return err
}

func (s *discordOutboundStream) sendAttachment(ctx context.Context, att channel.Attachment) error {
	file := discordAttachmentToFile(ctx, att, s.adapter.assets)
	if file == nil {
		return nil
	}

	messageSend := &discordgo.MessageSend{
		Files: []*discordgo.File{file},
	}

	// Add reply reference if this is the first message and we have a reply target
	if s.reply != nil && s.reply.MessageID != "" {
		messageSend.Reference = &discordgo.MessageReference{
			ChannelID: s.target,
			MessageID: s.reply.MessageID,
		}
	}

	_, err := s.session.ChannelMessageSendComplex(s.target, messageSend)
	return err
}
