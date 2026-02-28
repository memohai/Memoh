package email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/memohai/memoh/internal/inbox"
)

// ChatTriggerer triggers a proactive bot conversation (e.g. when a new email arrives).
type ChatTriggerer interface {
	TriggerBotChat(ctx context.Context, botID, content string) error
}

// Trigger pushes a notification to bot_inbox when a new email arrives
// and immediately triggers the bot's LLM to process it.
type Trigger struct {
	logger        *slog.Logger
	emailService  *Service
	botInbox      *inbox.Service
	chatTriggerer ChatTriggerer
}

func NewTrigger(log *slog.Logger, emailService *Service, botInbox *inbox.Service, chatTriggerer ChatTriggerer) *Trigger {
	return &Trigger{
		logger:        log.With(slog.String("component", "email_trigger")),
		emailService:  emailService,
		botInbox:      botInbox,
		chatTriggerer: chatTriggerer,
	}
}

// HandleInbound pushes a notification into each bound bot's inbox
// and triggers a conversation so the bot processes it immediately.
func (t *Trigger) HandleInbound(ctx context.Context, providerID string, mail InboundEmail) error {
	t.logger.Info("new email arrived",
		slog.String("provider_id", providerID),
		slog.String("from", mail.From),
		slog.String("subject", mail.Subject))

	bindings, err := t.emailService.ListReadableBindingsByProvider(ctx, providerID)
	if err != nil {
		t.logger.Error("failed to list readable bindings", slog.Any("error", err))
		return err
	}

	for _, binding := range bindings {
		content := fmt.Sprintf("New email received at %s from %s — %s", binding.EmailAddress, mail.From, mail.Subject)

		_, err := t.botInbox.Create(ctx, inbox.CreateRequest{
			BotID:  binding.BotID,
			Source: "email",
			Header: map[string]any{
				"provider_id":   providerID,
				"email_address": binding.EmailAddress,
				"from":          mail.From,
				"subject":       mail.Subject,
				"message_id":    mail.MessageID,
			},
			Content: content,
			Action:  inbox.ActionTrigger,
		})
		if err != nil {
			t.logger.Error("failed to create bot inbox notification",
				slog.String("bot_id", binding.BotID),
				slog.Any("error", err))
			continue
		}
		t.logger.Info("bot notified of new email",
			slog.String("bot_id", binding.BotID),
			slog.String("from", mail.From))

		if t.chatTriggerer != nil {
			go func(botID, text string) {
				if err := t.chatTriggerer.TriggerBotChat(ctx, botID, text); err != nil {
					t.logger.Error("failed to trigger bot chat for email",
						slog.String("bot_id", botID),
						slog.Any("error", err))
				}
			}(binding.BotID, content)
		}
	}

	return nil
}
