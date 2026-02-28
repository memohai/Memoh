package email

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/memohai/memoh/internal/inbox"
)

// Trigger pushes a notification to bot_inbox when a new email arrives,
// so the bot's LLM can decide whether to read it via MCP tools.
type Trigger struct {
	logger       *slog.Logger
	emailService *Service
	botInbox     *inbox.Service
}

func NewTrigger(log *slog.Logger, emailService *Service, botInbox *inbox.Service) *Trigger {
	return &Trigger{
		logger:       log.With(slog.String("component", "email_trigger")),
		emailService: emailService,
		botInbox:     botInbox,
	}
}

// HandleInbound pushes a notification into each bound bot's inbox.
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
		content := fmt.Sprintf("New email from %s — %s", mail.From, mail.Subject)

		_, err := t.botInbox.Create(ctx, inbox.CreateRequest{
			BotID:  binding.BotID,
			Source: "email",
			Header: map[string]any{
				"provider_id": providerID,
				"from":        mail.From,
				"subject":     mail.Subject,
				"message_id":  mail.MessageID,
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
	}

	return nil
}
