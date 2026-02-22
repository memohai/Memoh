package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/common"
)

type DiscordAdapter struct {
    logger   *slog.Logger
    mu       sync.RWMutex
    sessions map[string]*discordgo.Session // keyed by bot token
}

func NewDiscordAdapter(log *slog.Logger) *DiscordAdapter {
    if log == nil {
        log = slog.Default()
    }
    return &DiscordAdapter{
        logger:   log.With(slog.String("adapter", "discord")),
        sessions: make(map[string]*discordgo.Session),
    }
}

func (a *DiscordAdapter) Type() channel.ChannelType {
    return Type
}

func (a *DiscordAdapter) Descriptor() channel.Descriptor {
    return channel.Descriptor{
        Type:        Type,
        DisplayName: "Discord",
        Capabilities: channel.ChannelCapabilities{
            Text:           true,
            Markdown:       true,
            Reply:          true,
            Attachments:    true,
            Media:          true,
            Streaming:      true,
            BlockStreaming: true,
            Reactions:      true,
        },
        ConfigSchema: channel.ConfigSchema{
            Version: 1,
            Fields: map[string]channel.FieldSchema{
                "botToken": {
                    Type:     channel.FieldSecret,
                    Required: true,
                    Title:    "Bot Token",
                },
            },
        },
        UserConfigSchema: channel.ConfigSchema{
            Version: 1,
            Fields: map[string]channel.FieldSchema{
                "user_id":    {Type: channel.FieldString},
                "channel_id": {Type: channel.FieldString},
                "guild_id":   {Type: channel.FieldString},
                "username":   {Type: channel.FieldString},
            },
        },
        TargetSpec: channel.TargetSpec{
            Format: "channel_id | user_id",
            Hints: []channel.TargetHint{
                {Label: "Channel ID", Example: "1234567890123456789"},
                {Label: "User ID", Example: "1234567890123456789"},
            },
        },
    }
}

func (a *DiscordAdapter) getOrCreateSession(token, configID string) (*discordgo.Session, error) {
    a.mu.RLock()
    session, ok := a.sessions[token]
    a.mu.RUnlock()
    if ok {
        return session, nil
    }

    a.mu.Lock()
    defer a.mu.Unlock()
    if s, ok := a.sessions[token]; ok {
        return s, nil
    }

    session, err := discordgo.New("Bot " + token)
    if err != nil {
        a.logger.Error("create session failed", slog.String("config_id", configID), slog.Any("error", err))
        return nil, err
    }

    session.Identify.Intents = discordgo.IntentsAll
 
    a.sessions[token] = session
    return session, nil
}

func (a *DiscordAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
    if a.logger != nil {
        a.logger.Info("start", slog.String("config_id", cfg.ID))
    }

    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return nil, err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return nil, err
    }

    session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
        if m.Author != nil && m.Author.Bot {
            return
        }

        if ctx.Err() != nil {
            return
        }

        text := strings.TrimSpace(m.Content)
        botId := s.State.User.ID
        if text == "" && len(m.Attachments) == 0 {
            return
        }

        attachments := a.collectAttachments(m.Message)
        chatType := "direct"
        if m.GuildID != "" {
            chatType = "guild"
        }

        isMentioned := a.isBotMentioned(m.Message, botId)
        isReplyToBot := m.ReferencedMessage != nil &&
            m.ReferencedMessage.Author != nil &&
            m.ReferencedMessage.Author.ID == botId

        msg := channel.InboundMessage{
            Channel: Type,
            Message: channel.Message{
                ID:          m.ID,
                Format:      channel.MessageFormatPlain,
                Text:        text,
                Attachments: attachments,
            },
            BotID:       cfg.BotID,
            ReplyTarget: m.ChannelID,
            Sender: channel.Identity{
                SubjectID:   m.Author.ID,
                DisplayName: m.Author.Username,
                Attributes: map[string]string{
                    "user_id":  m.Author.ID,
                    "username": m.Author.Username,
                },
            },
            Conversation: channel.Conversation{
                ID:   m.ChannelID,
                Type: chatType,
            },
            ReceivedAt: time.Now().UTC(),
            Source:     "discord",
            Metadata: map[string]any{
                "guild_id":        m.GuildID,
                "is_mentioned":    isMentioned,
                "is_reply_to_bot": isReplyToBot,
            },
        }

        if a.logger != nil {
            a.logger.Info("inbound received",
                slog.String("config_id", cfg.ID),
                slog.String("chat_type", chatType),
                slog.String("user_id", m.Author.ID),
                slog.String("username", m.Author.Username),
                slog.String("text", common.SummarizeText(text)),
            )
        }

        go func() {
            if err := handler(ctx, cfg, msg); err != nil && a.logger != nil {
                a.logger.Error("handle inbound failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
            }
        }()
    })

    if err := session.Open(); err != nil {
        return nil, fmt.Errorf("discord open connection: %w", err)
    }

    stop := func(stopCtx context.Context) error {
        if a.logger != nil {
            a.logger.Info("stop", slog.String("config_id", cfg.ID))
        }
        return session.Close()
    }

    return channel.NewConnection(cfg, stop), nil
}

func (a *DiscordAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return err
    }

    channelID := strings.TrimSpace(msg.Target)
    if channelID == "" {
        return fmt.Errorf("discord target is required")
    }

    err = sendDiscordText(session, channelID, msg)
    return err
}

func sendDiscordText(session *discordgo.Session, channelID string, message channel.OutboundMessage) error {
    text_truncated := truncateDiscordText(message.Message.Text)
    var err error
    if message.Message.Reply != nil && message.Message.Reply.MessageID != "" {
        _, err = session.ChannelMessageSendReply(channelID, text_truncated, &discordgo.MessageReference{
            ChannelID: channelID,
            MessageID: message.Message.Reply.MessageID,
        })
    } else {
        _, err = session.ChannelMessageSend(channelID, text_truncated)
    }

    return err

}

func truncateDiscordText(text string) string {
    const discordMaxLength = 2000
    if len(text) > discordMaxLength {
        text = text[:discordMaxLength-3] + "..."
    }
    return text
}

func (a *DiscordAdapter) OpenStream(ctx context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
    target = strings.TrimSpace(target)
    if target == "" {
        return nil, fmt.Errorf("discord target is required")
    }

    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return nil, err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return nil, err
    }

    return &discordOutboundStream{
        adapter: a,
        cfg:     cfg,
        target:  target,
        reply:   opts.Reply,
        session: session,
    }, nil
}

func (a *DiscordAdapter) ProcessingStarted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
    chatID := strings.TrimSpace(info.ReplyTarget)
    if chatID == "" {
        return channel.ProcessingStatusHandle{}, nil
    }

    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return channel.ProcessingStatusHandle{}, err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return channel.ProcessingStatusHandle{}, err
    }

    // Discord typing indicator
    err = session.ChannelTyping(chatID)
    return channel.ProcessingStatusHandle{}, err
}

func (a *DiscordAdapter) ProcessingCompleted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle) error {
    return nil
}

func (a *DiscordAdapter) ProcessingFailed(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle, cause error) error {
    return nil
}

func (a *DiscordAdapter) React(ctx context.Context, cfg channel.ChannelConfig, target string, messageID string, emoji string) error {
    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return err
    }

    return session.MessageReactionAdd(target, messageID, emoji)
}

func (a *DiscordAdapter) Unreact(ctx context.Context, cfg channel.ChannelConfig, target string, messageID string, emoji string) error {
    discordCfg, err := parseConfig(cfg.Credentials)
    if err != nil {
        return err
    }

    session, err := a.getOrCreateSession(discordCfg.BotToken, cfg.ID)
    if err != nil {
        return err
    }

    return session.MessageReactionRemove(target, messageID, emoji, "@me")
}

func (a *DiscordAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
    return normalizeConfig(raw)
}

func (a *DiscordAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
    return normalizeUserConfig(raw)
}

func (a *DiscordAdapter) NormalizeTarget(raw string) string {
    return normalizeTarget(raw)
}

func (a *DiscordAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
    return resolveTarget(userConfig)
}

func (a *DiscordAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
    return matchBinding(config, criteria)
}

func (a *DiscordAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
    return buildUserConfig(identity)
}

func (a *DiscordAdapter) collectAttachments(msg *discordgo.Message) []channel.Attachment {
    if msg == nil || len(msg.Attachments) == 0 {
        return nil
    }

    attachments := make([]channel.Attachment, 0, len(msg.Attachments))
    for _, att := range msg.Attachments {
        attachment := channel.Attachment{
            Type:           channel.AttachmentFile,
            URL:            att.URL,
            PlatformKey:    att.ID,
            SourcePlatform: Type.String(),
            Name:           att.Filename,
            Size:           int64(att.Size),
        }

        if att.ContentType != "" {
            switch {
            case strings.HasPrefix(att.ContentType, "image/"):
                attachment.Type = channel.AttachmentImage
                attachment.Width = att.Width
                attachment.Height = att.Height
            case strings.HasPrefix(att.ContentType, "video/"):
                attachment.Type = channel.AttachmentVideo
            case strings.HasPrefix(att.ContentType, "audio/"):
                attachment.Type = channel.AttachmentAudio
            }
        }

        attachments = append(attachments, attachment)
    }

    return attachments
}

func (a *DiscordAdapter) isBotMentioned(msg *discordgo.Message, botID string) bool {
    if msg == nil {
        return false
    }

    for _, mention := range msg.Mentions {
        if mention != nil && mention.ID == botID {
            return true
        }
    }

    if msg.MentionEveryone {
        return true
    }

    botMention := "<@" + botID + ">"
    botNickMention := "<@!" + botID + ">"
    content := strings.ToLower(msg.Content)
    return strings.Contains(content, strings.ToLower(botMention)) ||
        strings.Contains(content, strings.ToLower(botNickMention))
}