package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/sqlc"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/session"
)

const (
	titlePromptMaxInputChars = 500
	titleGenerateTimeout     = 60 * time.Second
)

// SessionService is the interface the resolver uses for session title updates.
type SessionService interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
	UpdateTitle(ctx context.Context, sessionID, title string) (session.Session, error)
}

// SetSessionService configures the session service used for auto title generation.
func (r *Resolver) SetSessionService(s SessionService) {
	r.sessionService = s
}

// SetEventPublisher configures the event publisher for broadcasting events
// such as session title updates.
func (r *Resolver) SetEventPublisher(p messageevent.Publisher) {
	r.eventPublisher = p
}

// maybeGenerateSessionTitle checks whether the session needs an auto-generated
// title and, if so, calls the configured title model to produce one.
// It is fired asynchronously when a user message is received so the title
// appears as early as possible without blocking the chat flow.
func (r *Resolver) maybeGenerateSessionTitle(ctx context.Context, req conversation.ChatRequest, userQuery string) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" || r.sessionService == nil {
		return
	}

	userQuery = strings.TrimSpace(userQuery)
	if userQuery == "" {
		return
	}

	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		r.logger.Warn("title gen: failed to get session", slog.String("session_id", sessionID), slog.Any("error", err))
		return
	}
	if strings.TrimSpace(sess.Title) != "" {
		return
	}

	botSettings, err := r.loadBotSettings(ctx, req.BotID)
	if err != nil {
		r.logger.Warn("title gen: failed to load bot settings", slog.String("bot_id", req.BotID), slog.Any("error", err))
		return
	}
	titleModelID := strings.TrimSpace(botSettings.TitleModelID)
	if titleModelID == "" {
		r.logger.Debug("title gen: no title model configured", slog.String("bot_id", req.BotID))
		return
	}

	r.logger.Info("title gen: generating title", slog.String("session_id", sessionID), slog.String("title_model_id", titleModelID))

	titleModel, provider, err := r.fetchChatModel(ctx, titleModelID)
	if err != nil {
		r.logger.Warn("title gen: failed to resolve title model", slog.String("model_id", titleModelID), slog.Any("error", err))
		return
	}

	title := r.generateTitle(ctx, req.UserID, titleModel, provider, userQuery)
	if title == "" {
		return
	}

	if _, err := r.sessionService.UpdateTitle(ctx, sessionID, title); err != nil {
		r.logger.Warn("title gen: failed to update session title", slog.String("session_id", sessionID), slog.Any("error", err))
	} else {
		r.logger.Info("title gen: session title updated", slog.String("session_id", sessionID), slog.String("title", title))
		r.publishSessionTitleUpdated(req.BotID, sessionID, title)
	}
}

func (r *Resolver) generateTitle(ctx context.Context, userID string, model models.GetResponse, provider sqlc.Provider, userQuery string) string {
	userSnippet := truncate(strings.TrimSpace(userQuery), titlePromptMaxInputChars)
	if userSnippet == "" {
		return ""
	}

	prompt := "Generate a concise title (max 30 characters) for a conversation that starts with the following user message. " +
		"Return ONLY the title text, nothing else.\n\n" +
		"User: " + userSnippet

	authResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, userID)
	creds, err := authResolver.ResolveModelCredentials(authCtx, provider)
	if err != nil {
		r.logger.Warn("title gen: failed to resolve provider credentials", slog.Any("error", err))
		return ""
	}

	modelCfg := models.SDKModelConfig{
		ModelID:        model.ModelID,
		ClientType:     provider.ClientType,
		APIKey:         creds.APIKey,
		CodexAccountID: creds.CodexAccountID,
		BaseURL:        providers.ProviderConfigString(provider, "base_url"),
	}
	sdkModel := models.NewSDKChatModel(modelCfg)

	genCtx, cancel := context.WithTimeout(ctx, titleGenerateTimeout)
	defer cancel()

	client := sdk.NewClient()
	text, err := client.GenerateText(genCtx,
		sdk.WithModel(sdkModel),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage(prompt)}),
	)
	if err != nil {
		r.logger.Warn("title gen: LLM call failed", slog.Any("error", err))
		return ""
	}

	title := strings.TrimSpace(text)
	title = strings.Trim(title, "\"'`")
	title = strings.TrimSpace(title)
	return title
}

func (r *Resolver) publishSessionTitleUpdated(botID, sessionID, title string) {
	if r.eventPublisher == nil {
		return
	}
	data, err := json.Marshal(map[string]string{
		"session_id": sessionID,
		"title":      title,
	})
	if err != nil {
		return
	}
	r.eventPublisher.Publish(messageevent.Event{
		Type:  messageevent.EventTypeSessionTitleUpdated,
		BotID: botID,
		Data:  data,
	})
}

func truncate(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
