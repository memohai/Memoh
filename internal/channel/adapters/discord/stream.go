package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/memohai/memoh/internal/channel"
)

type discordOutboundStream struct {
    adapter     *DiscordAdapter
    cfg         channel.ChannelConfig
    target      string
    reply       *channel.ReplyRef
    session     *discordgo.Session
    closed      atomic.Bool
    mu          sync.Mutex
    msgID       string
    buffer      strings.Builder
    lastUpdate  time.Time
}

func (s *discordOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
    if s == nil || s.adapter == nil {
        return fmt.Errorf("discord stream not configured")
    }
    if s.closed.Load() {
        return fmt.Errorf("discord stream is closed")
    }

    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    switch event.Type {
    case channel.StreamEventStatus:
        if event.Status == channel.StreamStatusStarted {
            return s.ensureMessage(ctx, "Thinking...")
        }
        return nil

    case channel.StreamEventDelta:
        if event.Delta == "" {
            return nil
        }
        s.mu.Lock()
        s.buffer.WriteString(event.Delta)
        s.mu.Unlock()

        // Discord has strict rate limits, only update periodically
        if time.Since(s.lastUpdate) > 2*time.Second {
            return s.updateMessage(ctx)
        }
        return nil

    case channel.StreamEventFinal:
        if event.Final != nil && !event.Final.Message.IsEmpty() {
            finalText := strings.TrimSpace(event.Final.Message.PlainText())
            if finalText != "" {
                return s.finalizeMessage(ctx, finalText)
            }
        }
        s.mu.Lock()
        finalText := strings.TrimSpace(s.buffer.String())
        s.mu.Unlock()
        if finalText != "" {
            return s.finalizeMessage(ctx, finalText)
        }
        return nil

    case channel.StreamEventError:
        errText := strings.TrimSpace(event.Error)
        if errText == "" {
            return nil
        }
        return s.finalizeMessage(ctx, "Error: "+errText)

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

func (s *discordOutboundStream) ensureMessage(ctx context.Context, text string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.msgID != "" {
        return nil
    }

    // Discord limit: 2000 characters
    content := text
    if len(content) > 2000 {
        content = content[:1997] + "..."
    }

    msg, err := s.session.ChannelMessageSend(s.target, content)
    if err != nil {
        return err
    }

    s.msgID = msg.ID
    s.lastUpdate = time.Now()
    return nil
}

func (s *discordOutboundStream) updateMessage(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.msgID == "" {
        return nil
    }

    content := s.buffer.String()
    if content == "" {
        return nil
    }

    // Discord limit
    if len(content) > 2000 {
        content = content[:1997] + "..."
    }

    _, err := s.session.ChannelMessageEdit(s.target, s.msgID, content)
    if err != nil {
        return err
    }

    s.lastUpdate = time.Now()
    return nil
}

func (s *discordOutboundStream) finalizeMessage(ctx context.Context, text string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Discord limit
    if len(text) > 2000 {
        text = text[:1997] + "..."
    }

    if s.msgID == "" {
        _, err := s.session.ChannelMessageSend(s.target, text)
        return err
    }

    _, err := s.session.ChannelMessageEdit(s.target, s.msgID, text)
    return err
}