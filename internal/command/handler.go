package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/browsercontexts"
	emailpkg "github.com/memohai/memoh/internal/email"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/searchproviders"
	"github.com/memohai/memoh/internal/settings"
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
	bot, err := a.BotService.Get(ctx, botID)
	if err != nil {
		return "", err
	}
	if bot.OwnerUserID == channelIdentityID {
		return "owner", nil
	}
	return "", nil
}

// Handler processes slash commands intercepted before they reach the LLM.
type Handler struct {
	registry        *Registry
	roleResolver    MemberRoleResolver
	scheduleService *schedule.Service
	settingsService *settings.Service
	mcpConnService  *mcp.ConnectionService

	modelsService      *models.Service
	providersService   *providers.Service
	memProvService     *memprovider.Service
	searchProvService  *searchproviders.Service
	browserCtxService  *browsercontexts.Service
	emailService       *emailpkg.Service
	emailOutboxService *emailpkg.OutboxService
	heartbeatService   *heartbeat.Service
	queries            CommandQueries
	aclEvaluator       AccessEvaluator
	skillLoader        SkillLoader
	containerFS        ContainerFS

	logger *slog.Logger
}

// ExecuteInput carries the caller identity and channel context for command execution.
type ExecuteInput struct {
	BotID             string
	ChannelIdentityID string
	UserID            string
	Text              string
	ChannelType       string
	ConversationType  string
	ConversationID    string
	ThreadID          string
	RouteID           string
	SessionID         string
}

// NewHandler creates a Handler with all required services.
func NewHandler(
	log *slog.Logger,
	roleResolver MemberRoleResolver,
	scheduleService *schedule.Service,
	settingsService *settings.Service,
	mcpConnService *mcp.ConnectionService,
	modelsService *models.Service,
	providersService *providers.Service,
	memProvService *memprovider.Service,
	searchProvService *searchproviders.Service,
	browserCtxService *browsercontexts.Service,
	emailService *emailpkg.Service,
	emailOutboxService *emailpkg.OutboxService,
	heartbeatService *heartbeat.Service,
	queries CommandQueries,
	aclEvaluator AccessEvaluator,
	skillLoader SkillLoader,
	containerFS ContainerFS,
) *Handler {
	if log == nil {
		log = slog.Default()
	}
	h := &Handler{
		roleResolver:       roleResolver,
		scheduleService:    scheduleService,
		settingsService:    settingsService,
		mcpConnService:     mcpConnService,
		modelsService:      modelsService,
		providersService:   providersService,
		memProvService:     memProvService,
		searchProvService:  searchProvService,
		browserCtxService:  browserCtxService,
		emailService:       emailService,
		emailOutboxService: emailOutboxService,
		heartbeatService:   heartbeatService,
		queries:            queries,
		aclEvaluator:       aclEvaluator,
		skillLoader:        skillLoader,
		containerFS:        containerFS,
		logger:             log.With(slog.String("component", "command")),
	}
	h.registry = h.buildRegistry()
	return h
}

// topLevelCommands are standalone commands (no sub-actions) that are
// recognised by IsCommand and listed in /help. They are handled outside
// the regular resource-group dispatch (e.g. in the channel inbound
// processor which has the required routing context).
var topLevelCommands = map[string]string{
	"new":  "Start a new conversation (resets session context)",
	"stop": "Stop the current generation",
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
	if _, ok := topLevelCommands[parsed.Resource]; ok {
		return true
	}
	_, ok := h.registry.groups[parsed.Resource]
	return ok
}

// Execute parses and runs a slash command, returning the text reply.
func (h *Handler) Execute(ctx context.Context, botID, channelIdentityID, text string) (string, error) {
	return h.ExecuteWithInput(ctx, ExecuteInput{
		BotID:             botID,
		ChannelIdentityID: channelIdentityID,
		Text:              text,
	})
}

// ExecuteWithInput parses and runs a slash command with channel/session context.
func (h *Handler) ExecuteWithInput(ctx context.Context, input ExecuteInput) (string, error) {
	cmdText := ExtractCommandText(input.Text)
	if cmdText == "" {
		return h.registry.GlobalHelp(), nil
	}
	parsed, err := Parse(cmdText)
	if err != nil {
		return h.registry.GlobalHelp(), nil
	}

	// Resolve the user's role in this bot.
	role := ""
	roleIdentityID := input.ChannelIdentityID
	if strings.TrimSpace(input.UserID) != "" {
		roleIdentityID = strings.TrimSpace(input.UserID)
	}
	if h.roleResolver != nil && roleIdentityID != "" {
		r, err := h.roleResolver.GetMemberRole(ctx, input.BotID, roleIdentityID)
		if err != nil {
			h.logger.Warn("failed to resolve member role",
				slog.String("bot_id", input.BotID),
				slog.String("role_identity_id", roleIdentityID),
				slog.Any("error", err),
			)
		} else {
			role = r
		}
	}

	cc := CommandContext{
		Ctx:               ctx,
		BotID:             input.BotID,
		Role:              role,
		Args:              parsed.Args,
		ChannelIdentityID: strings.TrimSpace(input.ChannelIdentityID),
		UserID:            strings.TrimSpace(input.UserID),
		ChannelType:       strings.TrimSpace(input.ChannelType),
		ConversationType:  strings.TrimSpace(input.ConversationType),
		ConversationID:    strings.TrimSpace(input.ConversationID),
		ThreadID:          strings.TrimSpace(input.ThreadID),
		RouteID:           strings.TrimSpace(input.RouteID),
		SessionID:         strings.TrimSpace(input.SessionID),
	}

	// /help
	if parsed.Resource == "help" {
		if parsed.Action == "" {
			return h.registry.GlobalHelp(), nil
		}
		if len(parsed.Args) == 0 {
			return h.registry.GroupHelp(parsed.Action), nil
		}
		return h.registry.ActionHelp(parsed.Action, parsed.Args[0]), nil
	}

	// Top-level commands (e.g. /new) are handled by the channel inbound
	// processor which has the required routing context. If Execute is
	// called for one of these, return a short usage hint.
	if desc, ok := topLevelCommands[parsed.Resource]; ok {
		return fmt.Sprintf("/%s - %s", parsed.Resource, desc), nil
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

	if sub.IsWrite && role != "owner" {
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
