package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/compaction"
	dbstore "github.com/memohai/memoh/internal/db/store"
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
	emailService       *emailpkg.Service
	emailOutboxService *emailpkg.OutboxService
	heartbeatService   *heartbeat.Service
	compactionService  *compaction.Service
	queries            CommandQueries
	sqlcQueries        dbstore.Queries
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

// SetCompactionService configures the compaction service for the /compact command.
func (h *Handler) SetCompactionService(s *compaction.Service, q dbstore.Queries) {
	h.compactionService = s
	h.sqlcQueries = q
}

// CurrentContext resolves the bot's current model/heartbeat/reasoning state for
// enriching command output (e.g. the /new confirmation). It is a read-only view
// over existing bot settings and makes no changes.
func (h *Handler) CurrentContext(ctx context.Context, botID string) (CurrentContext, error) {
	cc := CommandContext{Ctx: ctx, BotID: strings.TrimSpace(botID)}
	s, err := h.getBotSettings(cc)
	if err != nil {
		return CurrentContext{}, err
	}
	return CurrentContext{
		ChatModel:        h.resolveModelName(cc, s.ChatModelID),
		HeartbeatModel:   h.resolveModelName(cc, s.HeartbeatModelID),
		ReasoningEnabled: s.ReasoningEnabled,
		ReasoningEffort:  s.ReasoningEffort,
		ContextWindow:    h.resolveContextWindow(cc),
	}, nil
}

// topLevelCommands are standalone commands (no sub-actions) that are
// recognised by IsCommand and listed in /help. They are handled outside
// the regular resource-group dispatch (e.g. in the channel inbound
// processor which has the required routing context).
var topLevelCommands = map[string]string{
	"new":     "Start a new conversation (resets session context)",
	"stop":    "Stop the current generation",
	"approve": "Approve the latest or specified pending tool call",
	"reject":  "Reject the latest or specified pending tool call",
}

// resourceAliases maps alternate spellings to the canonical command resource so
// that common variants all resolve (e.g. /setting, /reason, /effort, /think,
// /commands). Keys and values are lowercase (Parse lowercases the resource).
var resourceAliases = map[string]string{
	"setting":   "settings",
	"commands":  "help",
	"cmds":      "help",
	"reason":    "reasoning",
	"reasoning": "reasoning",
	"effort":    "reasoning",
	"think":     "reasoning",
}

// canonicalResource resolves a parsed resource through resourceAliases.
func canonicalResource(resource string) string {
	if c, ok := resourceAliases[resource]; ok {
		return c
	}
	return resource
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
	resource := canonicalResource(parsed.Resource)
	if resource == "help" {
		return true
	}
	if _, ok := topLevelCommands[resource]; ok {
		return true
	}
	_, ok := h.registry.groups[resource]
	return ok
}

// IsCommandShaped reports whether text looks like a slash command (a leading
// slash followed by a command-name token), whether or not it is registered. It
// lets the channel layer reply with a helpful "unknown command" hint instead of
// forwarding a mistyped command to the model. Paths/URLs (e.g. "/a/b") are
// rejected so they are not mistaken for commands.
func (*Handler) IsCommandShaped(text string) bool {
	cmdText := ExtractCommandText(text)
	if cmdText == "" || len(cmdText) < 2 {
		return false
	}
	parsed, err := Parse(cmdText)
	if err != nil {
		return false
	}
	return isCommandName(parsed.Resource)
}

// isCommandName reports whether r is a plausible command name: a letter followed
// by letters/digits/_/-, at most 32 chars. This excludes paths ("path/to/file"),
// which contain a slash, and other non-command slashes.
func isCommandName(r string) bool {
	if r == "" || len(r) > 32 {
		return false
	}
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case (c >= '0' && c <= '9' || c == '_' || c == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}

// UnknownCommandMessage is the reply for slash-command-shaped input that is not a
// known command. It points the user at /commands and offers the no-slash escape.
func UnknownCommandMessage(text string) string {
	parsed, _ := Parse(ExtractCommandText(text))
	return fmt.Sprintf(
		"Unknown command %s. Run %s to see what's available, or resend without the leading slash to send it as a regular message.",
		CmdRef(parsed.Resource), CmdRef("commands"),
	)
}

// Execute parses and runs a slash command, returning the text reply.
func (h *Handler) Execute(ctx context.Context, botID, channelIdentityID, text string) (string, error) {
	return h.ExecuteWithInput(ctx, ExecuteInput{
		BotID:             botID,
		ChannelIdentityID: channelIdentityID,
		Text:              text,
	})
}

// ExecuteWithInput parses and runs a slash command with channel/session
// context, returning the plain-text reply. It delegates to ExecuteResult and
// flattens the structured result to its text form.
func (h *Handler) ExecuteWithInput(ctx context.Context, input ExecuteInput) (string, error) {
	res, err := h.ExecuteResult(ctx, input)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.Text, nil
}

// ExecuteResult parses and runs a slash command, returning a neutral Result.
// The Result always carries complete Text; Interactive is set only by commands
// that opt into rich rendering via SubCommand.ResultHandler.
func (h *Handler) ExecuteResult(ctx context.Context, input ExecuteInput) (*Result, error) {
	cmdText := ExtractCommandText(input.Text)
	if cmdText == "" {
		return &Result{Text: h.registry.GlobalHelp()}, nil
	}
	parsed, err := Parse(cmdText)
	if err != nil {
		return &Result{Text: h.registry.GlobalHelp()}, nil
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
	writeAccess := role == "owner" || allowsUnboundWriteCommands(input)

	cc := CommandContext{
		Ctx:               ctx,
		BotID:             input.BotID,
		Role:              role,
		WriteAccess:       writeAccess,
		Args:              parsed.Args,
		ChannelIdentityID: strings.TrimSpace(input.ChannelIdentityID),
		UserID:            strings.TrimSpace(input.UserID),
		ChannelType:       strings.TrimSpace(input.ChannelType),
		ConversationType:  strings.TrimSpace(input.ConversationType),
		ConversationID:    strings.TrimSpace(input.ConversationID),
		ThreadID:          strings.TrimSpace(input.ThreadID),
		RouteID:           strings.TrimSpace(input.RouteID),
		SessionID:         strings.TrimSpace(input.SessionID),
		Page:              parsed.Page,
		Prov:              parsed.Prov,
		Flat:              parsed.Flat,
		Range:             parsed.Range,
	}

	resource := canonicalResource(parsed.Resource)

	// /help (and its alias /commands)
	if resource == "help" {
		switch {
		case parsed.Action == "":
			return &Result{Text: h.registry.GlobalHelp()}, nil
		case len(parsed.Args) == 0:
			return &Result{Text: h.registry.GroupHelp(parsed.Action)}, nil
		default:
			return &Result{Text: h.registry.ActionHelp(parsed.Action, parsed.Args[0])}, nil
		}
	}

	// Top-level commands (e.g. /new) are handled by the channel inbound
	// processor which has the required routing context. If Execute is
	// called for one of these, return a short usage hint.
	if desc, ok := topLevelCommands[resource]; ok {
		return &Result{Text: fmt.Sprintf("/%s - %s", resource, desc)}, nil
	}

	group, ok := h.registry.groups[resource]
	if !ok {
		return &Result{Text: fmt.Sprintf("Unknown command %s. Run %s to see all commands.", CmdRef(parsed.Resource), CmdRef("help"))}, nil
	}

	if parsed.Action == "" {
		if group.DefaultAction != "" {
			parsed.Action = group.DefaultAction
		} else {
			return &Result{Text: group.Usage()}, nil
		}
	}

	sub, ok := group.commands[parsed.Action]
	if !ok {
		return &Result{Text: fmt.Sprintf("Unknown action %s for %s. Run %s to see its actions.", MdCode(parsed.Action), CmdRef(parsed.Resource), CmdRef("help "+parsed.Resource))}, nil
	}

	if sub.IsWrite && !writeAccess {
		return &Result{Text: fmt.Sprintf("⚠️ Only the bot owner can run %s. You can still chat normally.", CmdRef(parsed.Resource))}, nil
	}

	if sub.ResultHandler != nil {
		res, handlerErr := safeExecuteResult(sub.ResultHandler, cc)
		if handlerErr != nil {
			return &Result{Text: h.friendlyCommandError(parsed.Resource, handlerErr)}, nil
		}
		if res == nil {
			res = &Result{}
		}
		return res, nil
	}

	text, handlerErr := safeExecute(sub.Handler, cc)
	if handlerErr != nil {
		return &Result{Text: h.friendlyCommandError(parsed.Resource, handlerErr)}, nil
	}
	return &Result{Text: text}, nil
}

// friendlyCommandError converts a service/handler error into user-facing text.
// Clean domain errors (e.g. `schedule "x" not found`, `model "x" is ambiguous`)
// are surfaced sentence-cased, with a discovery pointer appended for not-found
// cases. Errors that look like infra/transport leaks (raw Go wrap chains,
// "dial tcp", IPs, deadlines, SQL/driver text) are replaced with a generic
// retry line so internals never reach chat.
func (h *Handler) friendlyCommandError(resource string, err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	res := strings.TrimSpace(resource)
	if msg != "" && !looksLikeInternalError(msg) {
		out := capitalizeFirst(msg)
		if !strings.HasSuffix(out, ".") {
			out += "."
		}
		if res != "" && strings.Contains(strings.ToLower(msg), "not found") {
			out += fmt.Sprintf(" Run %s to see what's available.", CmdRef(res+" list"))
		}
		return out
	}
	// Sanitized path: keep the raw error in logs, show the user a clean retry line.
	if h.logger != nil {
		h.logger.Warn("command failed", slog.String("resource", res), slog.Any("error", err))
	}
	if res == "" {
		return "⚠️ Something went wrong. Try again in a moment."
	}
	return fmt.Sprintf("⚠️ Couldn't complete %s right now. Try again in a moment.", CmdRef(res))
}

// looksLikeInternalError reports whether an error message carries infra/transport
// internals that must not reach chat (Go wrap chains, network/SQL/TLS details).
// It keys on content markers only — a length cap was removed because legitimate
// domain messages (e.g. an ambiguous-model list of provider-qualified IDs) can
// be long, and capping by length wrongly replaced them with a dead retry line.
func looksLikeInternalError(msg string) bool {
	lower := strings.ToLower(msg)
	markers := []string{
		"failed to ", "dial tcp", "connection refused", "context deadline",
		"i/o timeout", "no such host", "://", "pq:", "sql", "x509",
		"panic:", "goroutine", "invalid memory", "nil pointer",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// capitalizeFirst upper-cases the first rune of s, leaving the rest untouched.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func allowsUnboundWriteCommands(input ExecuteInput) bool {
	if strings.TrimSpace(input.UserID) != "" {
		return false
	}
	if strings.TrimSpace(input.ChannelIdentityID) == "" {
		return false
	}
	// QQ and personal WeChat no longer have a channel-identity bind flow, so
	// channel-scoped slash commands must not depend on a linked Web user.
	switch strings.ToLower(strings.TrimSpace(input.ChannelType)) {
	case "qq", "weixin":
		return true
	default:
		return false
	}
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

// safeExecuteResult runs a structured sub-command handler and recovers from panics.
func safeExecuteResult(fn func(CommandContext) (*Result, error), cc CommandContext) (result *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return fn(cc)
}
