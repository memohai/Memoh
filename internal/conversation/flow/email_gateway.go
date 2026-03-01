package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

const emailTriggerTokenTTL = 10 * time.Minute

// EmailChatGateway implements email.ChatTriggerer by delegating to the Resolver.
type EmailChatGateway struct {
	resolver  *Resolver
	queries   *sqlc.Queries
	jwtSecret string
	logger    *slog.Logger
}

func NewEmailChatGateway(resolver *Resolver, queries *sqlc.Queries, jwtSecret string, logger *slog.Logger) *EmailChatGateway {
	return &EmailChatGateway{
		resolver:  resolver,
		queries:   queries,
		jwtSecret: jwtSecret,
		logger:    logger,
	}
}

func (g *EmailChatGateway) TriggerBotChat(ctx context.Context, botID, content string) error {
	if g == nil || g.resolver == nil {
		return fmt.Errorf("chat resolver not configured")
	}

	ownerUserID, err := g.resolveBotOwner(ctx, botID)
	if err != nil {
		return fmt.Errorf("resolve bot owner: %w", err)
	}

	token, err := g.generateToken(ownerUserID)
	if err != nil {
		return fmt.Errorf("generate trigger token: %w", err)
	}

	_, err = g.resolver.Chat(ctx, conversation.ChatRequest{
		BotID:          botID,
		ChatID:         botID,
		Query:          content,
		UserID:         ownerUserID,
		Token:          token,
		CurrentChannel: "email",
	})
	if err != nil {
		return fmt.Errorf("trigger chat: %w", err)
	}

	g.logger.Info("email trigger chat completed",
		slog.String("bot_id", botID))
	return nil
}

func (g *EmailChatGateway) resolveBotOwner(ctx context.Context, botID string) (string, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return "", err
	}
	bot, err := g.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return "", fmt.Errorf("get bot: %w", err)
	}
	ownerID := bot.OwnerUserID.String()
	if ownerID == "" {
		return "", fmt.Errorf("bot owner not found")
	}
	return ownerID, nil
}

func (g *EmailChatGateway) generateToken(userID string) (string, error) {
	if strings.TrimSpace(g.jwtSecret) == "" {
		return "", fmt.Errorf("jwt secret not configured")
	}
	signed, _, err := auth.GenerateToken(userID, g.jwtSecret, emailTriggerTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}
