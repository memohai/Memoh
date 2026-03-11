package command

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/browsercontexts"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	emailpkg "github.com/memohai/memoh/internal/email"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/provider"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/subagent"
)

// MemberRoleResolver resolves a user's role within a bot.
type MemberRoleResolver interface {
	GetMemberRole(ctx context.Context, botID, channelIdentityID string) (string, error)
}

// BotMemberRoleAdapter adapts bots.Service to MemberRoleResolver.
type BotMemberRoleAdapter struct {
	BotService *bots.Service
}

func (a *BotMemberRoleAdapter) GetMemberRole(ctx context.Context, botID, channelIdentityID string) (string, error) {
	member, err := a.BotService.GetMember(ctx, botID, channelIdentityID)
	if err != nil {
		return "", err
	}
	return member.Role, nil
}

// Handler processes slash commands intercepted before they reach the LLM.
type Handler struct {
	registry        *Registry
	roleResolver    MemberRoleResolver
	subagentService *subagent.Service
	scheduleService *schedule.Service
	settingsService *settings.Service
	mcpConnService  *mcp.ConnectionService
	inboxService    *inbox.Service

	modelsService      *models.Service
	providersService   *providers.Service
	memProvService     *memprovider.Service
	searchProvService  *searchproviders.Service
	browserCtxService  *browsercontexts.Service
	emailService       *emailpkg.Service
	emailOutboxService *emailpkg.OutboxService
	heartbeatService   *heartbeat.Service
	queries            *dbsqlc.Queries
	skillLoader        SkillLoader
	containerFS        ContainerFS

	logger *slog.Logger
}

// NewHandler creates a Handler with all required services.
func NewHandler(
	log *slog.Logger,
	roleResolver MemberRoleResolver,
	subagentService *subagent.Service,
	scheduleService *schedule.Service,
	settingsService *settings.Service,
	mcpConnService *mcp.ConnectionService,
	inboxService *inbox.Service,
	modelsService *models.Service,
	providersService *providers.Service,
	memProvService *memprovider.Service,
	searchProvService *searchproviders.Service,
	browserCtxService *browsercontexts.Service,
	emailService *emailpkg.Service,
	emailOutboxService *emailpkg.OutboxService,
	heartbeatService *heartbeat.Service,
	queries *dbsqlc.Queries,
	skillLoader SkillLoader,
	containerFS ContainerFS,
) *Handler {
	if log == nil {
		log = slog.Default()
	}
	h := &Handler{
		roleResolver:       roleResolver,
		subagentService:    subagentService,
		scheduleService:    scheduleService,
		settingsService:    settingsService,
		mcpConnService:     mcpConnService,
		inboxService:       inboxService,
		modelsService:      modelsService,
		providersService:   providersService,
		memProvService:     memProvService,
		searchProvService:  searchProvService,
		browserCtxService:  browserCtxService,
		emailService:       emailService,
		emailOutboxService: emailOutboxService,
		heartbeatService:   heartbeatService,
		queries:            queries,
		skillLoader:        skillLoader,
		containerFS:        containerFS,
		logger:             log.With(slog.String("component", "command")),
	}
	h.registry = h.buildRegistry()
	return h
}

// IsCommand reports whether the text contains a slash command.
// Handles both direct commands ("/help") and mention-prefixed commands ("@bot /help").
func (h *Handler) IsCommand(text string) bool {
	cmdText := ExtractCommandText(text)
	if cmdText == "" || len(cmdText) < 2 {
		return false
	}
	// Validate that it refers to a known command, not arbitrary "/path/to/file".
	parsed, err := Parse(cmdText)
	if err != nil {
		return false
	}
	if parsed.Resource == "help" {
		return true
	}
	_, ok := h.registry.groups[parsed.Resource]
	return ok
}

// Execute parses and runs a slash command, returning the text reply.
func (h *Handler) Execute(ctx context.Context, botID, channelIdentityID, text string) (string, error) {
	cmdText := ExtractCommandText(text)
	if cmdText == "" {
		return h.registry.GlobalHelp(), nil
	}
	parsed, err := Parse(cmdText)
	if err != nil {
		return h.registry.GlobalHelp(), nil
	}

	// Resolve the user's role in this bot.
	role := ""
	if h.roleResolver != nil && channelIdentityID != "" {
		r, err := h.roleResolver.GetMemberRole(ctx, botID, channelIdentityID)
		if err != nil {
			h.logger.Warn("failed to resolve member role",
				slog.String("bot_id", botID),
				slog.String("channel_identity_id", channelIdentityID),
				slog.Any("error", err),
			)
		} else {
			role = r
		}
	}

	cc := CommandContext{
		Ctx:   ctx,
		BotID: botID,
		Role:  role,
		Args:  parsed.Args,
	}

	// /help
	if parsed.Resource == "help" {
		return h.registry.GlobalHelp(), nil
	}

	group, ok := h.registry.groups[parsed.Resource]
	if !ok {
		return fmt.Sprintf("Unknown command: /%s\n\n%s", parsed.Resource, h.registry.GlobalHelp()), nil
	}

	if parsed.Action == "" {
		if group.DefaultAction != "" {
			parsed.Action = group.DefaultAction
		} else {
			return group.Usage(), nil
		}
	}

	sub, ok := group.commands[parsed.Action]
	if !ok {
		return fmt.Sprintf("Unknown action \"%s\" for /%s.\n\n%s", parsed.Action, parsed.Resource, group.Usage()), nil
	}

	if sub.IsWrite && role != bots.MemberRoleOwner {
		return "Permission denied: only the bot owner can execute this command.", nil
	}

	result, handlerErr := safeExecute(sub.Handler, cc)
	if handlerErr != nil {
		return fmt.Sprintf("Error: %s", handlerErr.Error()), nil
	}
	return result, nil
}

// safeExecute runs a sub-command handler and recovers from panics.
func safeExecute(fn func(CommandContext) (string, error), cc CommandContext) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return fn(cc)
}
