package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
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

// SessionService is the interface the resolver uses for session metadata updates.
type SessionService interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
	UpdateTitle(ctx context.Context, sessionID, title string) (session.Session, error)
	UpdateMetadata(ctx context.Context, sessionID string, metadata map[string]any) (session.Session, error)
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
	if req.SkipTitleGeneration {
		return
	}
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
	// Only generate a title when the session doesn't have one yet. Reading the
	// DB title (rather than an in-memory cache) keeps this guard restart-safe:
	// a session that already has an LLM-generated title is never re-run, even
	// after a server restart.
	if !shouldGenerateSessionTitle(sess) {
		return
	}

	promptTitle := fallbackSessionTitle(userQuery)

	// Persist a prompt-derived title immediately so a brand-new session is never
	// left "Untitled" — before (or without) the LLM title model. The sidebar
	// shows the user's first prompt right away; if a title model is configured
	// and the LLM succeeds below, it upgrades this.
	if strings.TrimSpace(sess.Title) == "" && promptTitle != "" {
		r.applyFallbackTitle(ctx, req, sessionID, userQuery)
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

func shouldGenerateSessionTitle(sess session.Session) bool {
	title := strings.TrimSpace(sess.Title)
	if title == "" {
		return true
	}
	if !session.IsACPRuntime(sess) {
		return false
	}

	agentID := metadataString(sess.Metadata, "acp_agent_id")
	normalized := acpprofile.NormalizeAgentID(agentID)
	if normalized == "" {
		return false
	}

	if strings.EqualFold(title, normalized) {
		return true
	}
	if profile, ok := acpprofile.Lookup(normalized); ok && strings.EqualFold(title, strings.TrimSpace(profile.DisplayName)) {
		return true
	}
	return false
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

	cacheTTL := providers.ProviderConfigString(provider, "prompt_cache_ttl")
	system, messages, _ := models.ApplyPromptCache(
		sdkModel, cacheTTL, "", []sdk.Message{sdk.UserMessage(prompt)}, nil,
	)

	client := sdk.NewClient()
	text, err := client.GenerateText(genCtx,
		sdk.WithModel(sdkModel),
		sdk.WithSystem(system),
		sdk.WithMessages(messages),
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

// fallbackTitleMaxRunes caps the prompt-derived fallback title. Mirrors the
// frontend provisionalSessionTitle so the value the client shows optimistically
// matches what the server persists (no flicker when the SSE lands).
const fallbackTitleMaxRunes = 50

// stripMarkdown removes structural markdown so a prompt-derived title reads as
// clean prose: fenced/inline code, images, links, heading/list/blockquote
// markers, emphasis, and table pipes. Mirrors the frontend stripMarkdown used by
// provisionalSessionTitle — keep them in lockstep so the persisted fallback
// equals the optimistic client display.
var (
	fallbackReFencedCode = regexp.MustCompile("(?s)```.*?```")
	fallbackReInlineCode = regexp.MustCompile("`([^`]+)`")
	fallbackReImage      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	fallbackReLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	fallbackReHeading    = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	fallbackReBlockquote = regexp.MustCompile(`(?m)^\s*>+\s?`)
	fallbackReListUl     = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	fallbackReListOl     = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	fallbackReEmphasis   = regexp.MustCompile(`(\*\*|\*|__|~~)`)
	fallbackReTablePipe  = regexp.MustCompile(`\|`)
	fallbackReWhitespace = regexp.MustCompile(`\s+`)
)

func stripMarkdown(md string) string {
	md = fallbackReFencedCode.ReplaceAllString(md, " ")
	md = fallbackReInlineCode.ReplaceAllString(md, "$1")
	md = fallbackReImage.ReplaceAllString(md, "$1")
	md = fallbackReLink.ReplaceAllString(md, "$1")
	md = fallbackReHeading.ReplaceAllString(md, "")
	md = fallbackReBlockquote.ReplaceAllString(md, "")
	md = fallbackReListUl.ReplaceAllString(md, "")
	md = fallbackReListOl.ReplaceAllString(md, "")
	md = fallbackReEmphasis.ReplaceAllString(md, "")
	md = fallbackReTablePipe.ReplaceAllString(md, " ")
	return md
}

// fallbackSessionTitle derives a one-line sidebar title from a user's first
// message: strip markdown across the whole prompt (so a complete code fence is
// dropped, not just its opening line), take the first remaining line, collapse
// whitespace, and cap at fallbackTitleMaxRunes with an ellipsis. Returns "" when
// there is no usable text.
func fallbackSessionTitle(userQuery string) string {
	stripped := strings.TrimSpace(stripMarkdown(userQuery))
	if stripped == "" {
		return ""
	}
	firstLine := strings.SplitN(stripped, "\n", 2)[0]
	firstLine = strings.TrimSpace(fallbackReWhitespace.ReplaceAllString(firstLine, " "))
	if firstLine == "" {
		return ""
	}
	if truncated := truncate(firstLine, fallbackTitleMaxRunes); truncated != firstLine {
		return truncated + "…"
	}
	return firstLine
}

// applyFallbackTitle persists a prompt-derived title for a session that would
// otherwise stay untitled, then publishes it so clients replace the
// "Untitled" placeholder immediately. No-op when the prompt yields no usable
// text.
func (r *Resolver) applyFallbackTitle(ctx context.Context, req conversation.ChatRequest, sessionID, userQuery string) {
	title := fallbackSessionTitle(userQuery)
	if title == "" {
		return
	}
	if _, err := r.sessionService.UpdateTitle(ctx, sessionID, title); err != nil {
		r.logger.Warn("title gen: failed to apply fallback title", slog.String("session_id", sessionID), slog.Any("error", err))
		return
	}
	r.logger.Info("title gen: applied fallback title", slog.String("session_id", sessionID), slog.String("title", title))
	r.publishSessionTitleUpdated(req.BotID, sessionID, title)
}
