package flow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/accounts"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/agentteam"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/providers"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/toolapproval"
)

const (
	defaultMaxContextMinutes = 24 * 60
)

// SkillEntry represents a skill loaded from the container.
type SkillEntry struct {
	Name        string
	Description string
	Content     string
	Path        string
	Metadata    map[string]any
}

// SkillLoader loads skills for a given bot from its container.
type SkillLoader interface {
	LoadSkills(ctx context.Context, botID string) ([]SkillEntry, error)
}

// ConversationSettingsReader defines settings lookup behavior needed by flow resolution.
type ConversationSettingsReader interface {
	GetSettings(ctx context.Context, conversationID string) (conversation.Settings, error)
}

// gatewayAssetLoader resolves content_hash references to binary payloads for gateway dispatch.
type gatewayAssetLoader interface {
	OpenForGateway(ctx context.Context, botID, contentHash string) (reader io.ReadCloser, mime string, err error)
}

type botChannelConfigReader interface {
	ListBotConfigs(ctx context.Context, botID string) ([]channel.ChannelConfig, error)
}

// Resolver orchestrates chat with the internal agent.
type Resolver struct {
	agent             *agentpkg.Agent
	modelsService     *models.Service
	queries           dbstore.Queries
	memoryRegistry    *memprovider.Registry
	conversationSvc   ConversationSettingsReader
	messageService    messagepkg.Service
	settingsService   *settings.Service
	accountService    *accounts.Service
	sessionService    SessionService
	routeService      RouteService
	compactionService *compaction.Service
	eventPublisher    messageevent.Publisher
	skillLoader       SkillLoader
	assetLoader       gatewayAssetLoader
	channelStore      botChannelConfigReader
	teamService       *agentteam.Service
	pipeline          *pipelinepkg.Pipeline
	streamHTTPClient  *http.Client
	bgManager         *background.Manager
	toolApproval      *toolapproval.Service
	outboundFn        func(ctx context.Context, botID, channelType, target, text string) error
	bgNotifDeferred   sync.Map // key: "botID:sessionID" → wake arrived while a session turn was active
	sessionTurnMu     sync.Mutex
	sessionTurnRefs   map[string]int // key: "botID:sessionID" → active turn refcount
	timeout           time.Duration
	clockLocation     *time.Location
	logger            *slog.Logger
}

// NewResolver creates a Resolver that uses the internal agent directly.
func NewResolver(
	log *slog.Logger,
	modelsService *models.Service,
	queries dbstore.Queries,
	conversationSvc ConversationSettingsReader,
	messageService messagepkg.Service,
	settingsService *settings.Service,
	accountService *accounts.Service,
	a *agentpkg.Agent,
	clockLocation *time.Location,
	timeout time.Duration,
) *Resolver {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if clockLocation == nil {
		clockLocation = time.UTC
	}
	// HTTP client with timeouts for LLM provider streaming.
	// - DialTimeout: fail fast on connection issues
	// - ResponseHeaderTimeout: catch servers that accept TCP but never respond
	// - Timeout: overall request lifetime cap (prevents stuck SSE body reads)
	streamHTTPClient := &http.Client{
		Timeout: 10 * time.Minute, // overall cap, matches resolver timeout
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	return &Resolver{
		agent:            a,
		modelsService:    modelsService,
		queries:          queries,
		conversationSvc:  conversationSvc,
		messageService:   messageService,
		settingsService:  settingsService,
		accountService:   accountService,
		streamHTTPClient: streamHTTPClient,
		sessionTurnRefs:  make(map[string]int),
		timeout:          timeout,
		clockLocation:    clockLocation,
		logger:           log.With(slog.String("service", "conversation_resolver")),
	}
}

// SetMemoryRegistry sets the provider registry for memory operations.
func (r *Resolver) SetMemoryRegistry(registry *memprovider.Registry) {
	r.memoryRegistry = registry
}

// SetTeamService injects the agentteam service used to assemble team-aware
// prompt sections.
func (r *Resolver) SetTeamService(svc *agentteam.Service) {
	r.teamService = svc
}

// TriggerHandoff runs the handoff for the target bot. It builds a base
// run config for the bot, injects team / issue / handoff context, and
// generates a single agent turn. The bot is expected to post a result
// comment back to the issue via the team tools; the post-comment hook
// will then close the handoff and queue a return for the delegator.
//
// The conversation is persisted under a stable bot_session keyed on
// (bot_id, issue_id). This means a bot working on the same issue across
// multiple handoffs keeps a continuous thread that shows up in its chat
// sidebar — users can read what the bot actually did, not only the
// summary comment it posted to the issue.
func (r *Resolver) TriggerHandoff(ctx context.Context, handoff agentteam.Handoff, triggerComment agentteam.Comment) error {
	if r.agent == nil {
		return errors.New("agentteam: agent not configured")
	}
	if strings.TrimSpace(handoff.ToBotID) == "" {
		return errors.New("agentteam: handoff target bot id required")
	}
	if r.teamService == nil {
		return errors.New("agentteam: team service not configured on resolver")
	}
	authorName := r.resolveCommentAuthorName(ctx, triggerComment)
	issue := r.lookupIssue(ctx, handoff.IssueID)
	// Resolve the bot's session for this handoff. When the dispatcher
	// pinned an explicit target session (return path: A delegated from
	// S1 → handoff.target_session_id == S1 → A wakes back up in S1),
	// honour it verbatim. Otherwise fall back to (bot, issue) so the
	// target bot has a stable per-issue scratch session across multiple
	// @mentions on the same issue.
	sessionID := strings.TrimSpace(handoff.TargetSessionID)
	if sessionID == "" {
		var sessErr error
		sessionID, sessErr = r.ensureHandoffSession(ctx, handoff.ToBotID, handoff.TeamID, handoff.IssueID, issue)
		if sessErr != nil {
			r.logger.Warn(
				"ensure handoff session failed (continuing without persistence)",
				slog.String("bot_id", handoff.ToBotID),
				slog.String("issue_id", handoff.IssueID),
				slog.Any("error", sessErr),
			)
		}
	}
	req := conversation.ChatRequest{
		BotID:     handoff.ToBotID,
		ChatID:    handoff.ToBotID,
		SessionID: sessionID,
		Query:     buildHandoffPrompt(handoff, triggerComment, authorName, issue),
		UserID:    triggerComment.AuthorUserID,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return fmt.Errorf("resolve handoff: %w", err)
	}
	cfg := rc.runConfig
	cfg.SessionType = "chat"
	cfg.Identity.TeamID = handoff.TeamID
	cfg.Identity.IssueID = handoff.IssueID
	cfg.Identity.HandoffID = handoff.ID
	cfg.Identity.TriggerKind = "handoff"
	if _, err := r.teamService.Store().MarkHandoffDispatched(ctx, handoff.ID, sessionID); err != nil {
		r.logger.Warn("mark handoff dispatched failed", slog.String("handoff_id", handoff.ID), slog.Any("error", err))
	}
	cfg = r.prepareRunConfig(ctx, cfg)
	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return fmt.Errorf("agent generate: %w", err)
	}
	if sessionID != "" {
		outputMessages := sdkMessagesToModelMessages(result.Messages)
		roundMessages := prependUserMessage(req.Query, outputMessages)
		if storeErr := r.storeRound(ctx, req, roundMessages, rc.model.ID); storeErr != nil {
			r.logger.Warn(
				"store handoff round failed",
				slog.String("handoff_id", handoff.ID),
				slog.String("session_id", sessionID),
				slog.Any("error", storeErr),
			)
		}
	}
	return nil
}

// ensureHandoffSession looks up (or creates) the stable bot_session row
// that backs all handoff turns for a given (bot, issue). The session is
// the single thread a user reads to follow what this bot did on this
// issue across mention → return → re-mention cycles.
//
// Session metadata always carries:
//
//	team_id  : the team this session lives under
//	issue_id : the issue this session is dedicated to
//	kind     : "team_handoff"
//
// We use the `kind` discriminator so a future query can list all handoff
// sessions distinctly from regular chat sessions.
func (r *Resolver) ensureHandoffSession(ctx context.Context, botID, teamID, issueID string, issue *agentteam.Issue) (string, error) {
	if r.sessionService == nil {
		return "", errors.New("session service not configured")
	}
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(issueID) == "" {
		return "", errors.New("bot id and issue id are required")
	}
	sessions, err := r.sessionService.ListByBot(ctx, botID)
	if err != nil {
		return "", fmt.Errorf("list bot sessions: %w", err)
	}
	for _, s := range sessions {
		if s.Type != sessionpkg.TypeChat {
			continue
		}
		meta := s.Metadata
		if meta == nil {
			continue
		}
		if kind, _ := meta["kind"].(string); kind != "team_handoff" {
			continue
		}
		if mIssue, _ := meta["issue_id"].(string); mIssue == issueID {
			return s.ID, nil
		}
	}
	title := buildHandoffSessionTitle(issue)
	created, err := r.sessionService.Create(ctx, sessionpkg.CreateInput{
		BotID: botID,
		Type:  sessionpkg.TypeChat,
		Title: title,
		Metadata: map[string]any{
			"kind":     "team_handoff",
			"team_id":  teamID,
			"issue_id": issueID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create handoff session: %w", err)
	}
	return created.ID, nil
}

func buildHandoffSessionTitle(issue *agentteam.Issue) string {
	if issue == nil {
		return "Team issue"
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		return fmt.Sprintf("Issue #%d", issue.Number)
	}
	return fmt.Sprintf("Issue #%d %s", issue.Number, title)
}

// resolveCommentAuthorName returns the display name of a comment's author
// (bot or user). Falls back to a stable string when the author row cannot
// be resolved so the prompt never leaves the bot guessing.
func (r *Resolver) resolveCommentAuthorName(ctx context.Context, c agentteam.Comment) string {
	switch c.AuthorType {
	case agentteam.ActorBot:
		if name := r.lookupBotDisplayName(ctx, c.AuthorBotID); name != "" {
			return name
		}
		return "another bot"
	case agentteam.ActorUser:
		if name := r.lookupUserDisplayName(ctx, c.AuthorUserID); name != "" {
			return name
		}
		return "a teammate"
	case agentteam.ActorSystem:
		return "the platform"
	}
	return "a teammate"
}

func (r *Resolver) lookupBotDisplayName(ctx context.Context, botID string) string {
	if r.queries == nil || strings.TrimSpace(botID) == "" {
		return ""
	}
	uuid, err := db.ParseUUID(botID)
	if err != nil {
		return ""
	}
	row, err := r.queries.GetBotByID(ctx, uuid)
	if err != nil || !row.DisplayName.Valid {
		return ""
	}
	return strings.TrimSpace(row.DisplayName.String)
}

func (r *Resolver) lookupUserDisplayName(ctx context.Context, userID string) string {
	if r.queries == nil || strings.TrimSpace(userID) == "" {
		return ""
	}
	uuid, err := db.ParseUUID(userID)
	if err != nil {
		return ""
	}
	row, err := r.queries.GetUserByID(ctx, uuid)
	if err != nil {
		return ""
	}
	if row.DisplayName.Valid && strings.TrimSpace(row.DisplayName.String) != "" {
		return strings.TrimSpace(row.DisplayName.String)
	}
	if row.Username.Valid && strings.TrimSpace(row.Username.String) != "" {
		return strings.TrimSpace(row.Username.String)
	}
	return ""
}

func (r *Resolver) lookupIssue(ctx context.Context, issueID string) *agentteam.Issue {
	if r.teamService == nil || strings.TrimSpace(issueID) == "" {
		return nil
	}
	issue, err := r.teamService.GetIssue(ctx, issueID)
	if err != nil {
		return nil
	}
	return &issue
}

// buildHandoffPrompt composes the user-message payload Bot2 sees on the
// turn that delivers a handoff. It states three things explicitly so the
// bot never has to guess:
//
//  1. WHO is talking to it (resolved display name + actor type).
//  2. WHICH issue this is about (title + number, when available).
//  3. WHAT was said — the verbatim triggering comment body.
//
// It also pins the **routing contract**: the bot's `issue_comment` reply
// is auto-threaded under the triggering comment so the dispatcher can
// match it back to *this* handoff (and route the return to the right
// originating session). Bots are told not to override `parent_comment_id`
// unless they really mean to leave the thread.
//
// Two flavours:
//   - Initial @mention: "<Alice> @-mentioned you on issue #N <Title>" + comment text.
//   - Return handoff   : "Your earlier delegation came back — <Worker> posted an
//     update on issue #N. Evaluate and continue or exit silently."
func buildHandoffPrompt(handoff agentteam.Handoff, comment agentteam.Comment, authorName string, issue *agentteam.Issue) string {
	header := buildIssueHeader(issue)
	if handoff.FromActorType == agentteam.ActorSystem && comment.AuthorType == agentteam.ActorBot {
		return strings.TrimSpace(
			"Your earlier delegation on " + header + " has come back. **" + authorName + "** just posted an update.\n\n" +
				"Evaluate the result: continue with the next step, delegate further (`@<Name>`), or exit silently if no action is needed. " +
				"This wake-up is targeted at the originating session — your reply lands wherever you were before, not in a new chat.\n\n" +
				"## Update from " + authorName + "\n\n" + comment.Content,
		)
	}
	return strings.TrimSpace(
		"**" + authorName + "** (" + string(comment.AuthorType) + ") just @mentioned you on " + header + ".\n\n" +
			"Read the message below and reply by calling the `issue_comment` tool — the reply is automatically threaded under their @mention so the platform can route the return back to whichever session " + authorName + " sent this from. " +
			"Do NOT override `parent_comment_id` unless you really want to leave this thread. " +
			"Address them by name when you reply. If no action is needed, you may exit silently.\n\n" +
			"## Message from " + authorName + "\n\n" + comment.Content,
	)
}

// buildIssueHeader renders a short identifier for the issue the handoff
// belongs to. Falls back to "the team issue" when the issue row could not
// be loaded.
func buildIssueHeader(issue *agentteam.Issue) string {
	if issue == nil {
		return "the team issue"
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		return fmt.Sprintf("issue #%d", issue.Number)
	}
	return fmt.Sprintf("issue #%d \"%s\"", issue.Number, title)
}

// buildSelfIdentitySection assembles the bot's "Who you are" paragraph
// that sits at the very top of every system prompt. It is the canonical
// place for `bots.display_name`. Returns an empty string when the bot has
// no display name configured or when the bot row cannot be loaded.
//
// This intentionally does NOT depend on team membership: the bot's name
// is its own identity regardless of which session type or team context
// it is running under.
func (r *Resolver) buildSelfIdentitySection(ctx context.Context, botID string) string {
	if r.queries == nil || strings.TrimSpace(botID) == "" {
		return ""
	}
	uuid, err := db.ParseUUID(botID)
	if err != nil {
		return ""
	}
	row, err := r.queries.GetBotByID(ctx, uuid)
	if err != nil {
		return ""
	}
	name := ""
	if row.DisplayName.Valid {
		name = strings.TrimSpace(row.DisplayName.String)
	}
	if name == "" {
		return ""
	}
	return "You are **" + name + "**. " +
		"This is your canonical name across Memoh — refer to yourself by this name, " +
		"and it is also how other people and bots will @mention you."
}

// buildTeamSection assembles the team-aware system prompt fragment when the
// session is scoped to a team. It tells the bot, in order:
//
//  1. WHICH TEAM — team name, description, shared-dir display name, team-level instructions.
//  2. YOUR TEAM ROLE — role + per-team instructions for THIS bot in this team.
//  3. TEAMMATES — roster of bots and humans with their @-mention tokens.
//
// The bot's canonical display name is intentionally NOT repeated here —
// `buildSelfIdentitySection` already pins it at the top of the prompt.
//
// Returns "" outside a team context.
func (r *Resolver) buildTeamSection(ctx context.Context, teamID, botID string) string {
	if r.teamService == nil || strings.TrimSpace(teamID) == "" {
		return ""
	}
	team, err := r.teamService.GetTeam(ctx, teamID)
	if err != nil {
		if r.logger != nil {
			r.logger.Debug("team lookup failed", slog.String("team_id", teamID), slog.Any("error", err))
		}
		return ""
	}

	var sb strings.Builder

	// (1) Team context.
	dirName := agentteam.TeamDirName(team)
	sb.WriteString("## Active Team\n\n")
	sb.WriteString("- Team: " + team.Name + "\n")
	sb.WriteString("- Team id: " + team.ID + "\n")
	sb.WriteString("- Shared workspace: `/team/" + dirName + "/` — write team artifacts here. `/team/` only contains directories for the teams you belong to; teams you are not a member of are not visible at all.\n")
	if team.Description != "" {
		sb.WriteString("- Description: " + team.Description + "\n")
	}
	if team.Instructions != "" {
		sb.WriteString("- Team instructions:\n  > " + team.Instructions + "\n")
	}

	// (2) Your role inside this specific team (only when set — falls
	// through silently otherwise so a no-role member doesn't clutter the
	// prompt).
	if strings.TrimSpace(botID) != "" {
		if self, mErr := r.teamService.Store().GetMemberByBot(ctx, teamID, botID); mErr == nil {
			if self.Role != "" || self.Instructions != "" {
				sb.WriteString("\n### Your role in this team\n\n")
				if self.Role != "" {
					sb.WriteString("- Role: " + self.Role + "\n")
				}
				if self.Instructions != "" {
					sb.WriteString("- Team-specific instructions:\n  > " + self.Instructions + "\n")
				}
			}
		}
	}

	// (3) Teammates.
	members, err := r.teamService.ListMembers(ctx, teamID)
	if err == nil && len(members) > 0 {
		sb.WriteString("\n### Teammates\n\n")
		for _, m := range members {
			if m.MemberType == agentteam.MemberBot && m.BotID == botID {
				continue
			}
			label := strings.TrimSpace(m.DisplayName)
			if label == "" {
				continue
			}
			mention := mentionTokenFor(label)
			line := "- **" + label + "** — " + string(m.MemberType)
			if m.Role != "" {
				line += ", role: " + m.Role
			}
			line += " — mention as `" + mention + "`"
			if m.Instructions != "" {
				line += "  \n  > " + m.Instructions
			}
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

// mentionTokenFor renders the ready-to-paste `@Name` token for a member.
// Names containing whitespace use the quoted form so they survive the
// `agentteam.ParseMentions` regex as a single label.
func mentionTokenFor(name string) string {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return ""
	}
	if strings.ContainsAny(cleaned, " \t") {
		return `@"` + strings.ReplaceAll(cleaned, `"`, `'`) + `"`
	}
	return "@" + cleaned
}

// SetSkillLoader sets the skill loader used to populate usable skills in gateway requests.
func (r *Resolver) SetSkillLoader(sl SkillLoader) {
	r.skillLoader = sl
}

// SetGatewayAssetLoader configures optional asset loading used to inline
// attachments before calling the agent gateway.
func (r *Resolver) SetGatewayAssetLoader(loader gatewayAssetLoader) {
	r.assetLoader = loader
}

// SetChannelStore configures the bot channel config store used to load
// platform identity metadata for system prompt generation.
func (r *Resolver) SetChannelStore(store botChannelConfigReader) {
	r.channelStore = store
}

// SetCompactionService configures the compaction service for context compaction.
func (r *Resolver) SetCompactionService(s *compaction.Service) {
	r.compactionService = s
}

// SetBackgroundManager configures the background task manager so that
// background exec notifications are injected into the agent loop.
func (r *Resolver) SetBackgroundManager(m *background.Manager) {
	r.bgManager = m
}

func (r *Resolver) SetToolApprovalService(s *toolapproval.Service) {
	r.toolApproval = s
}

// SetOutboundFn configures the function used to deliver background notification
// responses to the user. The agent's text output is delivered through the same
// path as normal responses.
func (r *Resolver) SetOutboundFn(fn func(ctx context.Context, botID, channelType, target, text string) error) {
	r.outboundFn = fn
}

// SetPipeline configures the DCP pipeline for RC-based context assembly.
// When set, resolve() will use RC from the pipeline instead of loading
// history from bot_history_messages for sessions that have pipeline data.
func (r *Resolver) SetPipeline(p *pipelinepkg.Pipeline) {
	r.pipeline = p
}

// Pipeline returns the configured pipeline, or nil.
func (r *Resolver) Pipeline() *pipelinepkg.Pipeline {
	return r.pipeline
}

// InlineImageAttachments resolves image content hashes to sdk.ImagePart values
// using the configured asset loader. Intended for the discuss driver to inline
// images from new RC segments before calling the LLM.
func (r *Resolver) InlineImageAttachments(ctx context.Context, botID string, refs []pipelinepkg.ImageAttachmentRef) []sdk.ImagePart {
	if r == nil || r.assetLoader == nil || len(refs) == 0 {
		return nil
	}
	var parts []sdk.ImagePart
	for _, ref := range refs {
		contentHash := strings.TrimSpace(ref.ContentHash)
		if contentHash == "" {
			continue
		}
		dataURL, mime, err := r.inlineAssetAsDataURL(ctx, botID, contentHash, "image", strings.TrimSpace(ref.Mime))
		if err != nil {
			if r.logger != nil {
				r.logger.Warn(
					"inline discuss image attachment failed",
					slog.Any("error", err),
					slog.String("bot_id", botID),
					slog.String("content_hash", contentHash),
				)
			}
			continue
		}
		parts = append(parts, sdk.ImagePart{
			Image:     dataURL,
			MediaType: mime,
		})
	}
	return parts
}

type usageInfo struct {
	InputTokens  *int `json:"inputTokens"`
	OutputTokens *int `json:"outputTokens"`
}

type resolvedContext struct {
	runConfig       agentpkg.RunConfig
	model           models.GetResponse
	provider        sqlc.Provider
	query           string // headerified query
	injectedRecords *[]conversation.InjectedMessageRecord
	estimatedTokens int // estimated input token count for compaction
}

func (r *Resolver) resolve(ctx context.Context, req conversation.ChatRequest) (resolvedContext, error) {
	if strings.TrimSpace(req.Query) == "" && len(req.Attachments) == 0 {
		return resolvedContext{}, errors.New("query or attachments is required")
	}
	if strings.TrimSpace(req.BotID) == "" {
		return resolvedContext{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(req.ChatID) == "" {
		return resolvedContext{}, errors.New("chat id is required")
	}

	runCfg, chatModel, provider, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             req.BotID,
		ChatID:            req.ChatID,
		SessionID:         req.SessionID,
		RouteID:           req.RouteID,
		UserID:            req.UserID,
		ChannelIdentityID: req.SourceChannelIdentityID,
		CurrentPlatform:   req.CurrentChannel,
		ReplyTarget:       req.ReplyTarget,
		ConversationType:  req.ConversationType,
		SessionToken:      req.ChatToken,
		Model:             req.Model,
		Provider:          req.Provider,
		ReasoningEffort:   req.ReasoningEffort,
	})
	if err != nil {
		r.logger.Error(
			"resolve: buildBaseRunConfig failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return resolvedContext{}, err
	}
	memoryMsg := r.loadMemoryContextMessage(ctx, req)
	reqMessages := pruneMessagesForGateway(nonNilModelMessages(req.Messages))
	if memoryMsg != nil {
		pruned, _ := pruneMessageForGateway(*memoryMsg)
		memoryMsg = &pruned
	}

	// When the DCP pipeline has data for this session, build context from
	// the rendered event stream (RC) + bot turn responses (TR) instead of
	// loading raw history from bot_history_messages. The current inbound
	// message is already in the RC, so it must not be appended again.
	usePipeline := r.pipeline != nil && strings.TrimSpace(req.SessionID) != ""
	if usePipeline {
		if _, loaded := r.pipeline.GetIC(strings.TrimSpace(req.SessionID)); !loaded {
			usePipeline = false
		}
	}

	contextTokenBudget := 0
	if chatModel.Config.ContextWindow != nil && *chatModel.Config.ContextWindow > 0 {
		contextTokenBudget = *chatModel.Config.ContextWindow
	}

	var messages []conversation.ModelMessage
	var estimatedTokens int
	if usePipeline {
		messages = r.buildMessagesFromPipeline(ctx, req, contextTokenBudget)
	} else if r.conversationSvc != nil {
		loaded, loadErr := r.loadMessages(ctx, req.ChatID, req.SessionID, defaultMaxContextMinutes)
		if loadErr != nil {
			r.logger.Error(
				"resolve: loadMessages failed",
				slog.String("bot_id", req.BotID),
				slog.Any("error", loadErr),
			)
			return resolvedContext{}, loadErr
		}
		loaded = pruneHistoryForGateway(loaded)
		loaded = dedupePersistedCurrentUserMessage(loaded, req)
		loaded = r.replaceCompactedMessages(ctx, loaded)
		messages, estimatedTokens = trimMessagesByTokens(r.logger, loaded, contextTokenBudget)
		// When context reaches 70% of the contextTokenBudget (the user-configured
		// budget cap), run synchronous compaction before sending the request.
		// contextTokenBudget is the authoritative limit for how much context
		// the user wants to send to the LLM. We compact at 70% to keep the
		// context healthy and avoid edge-case timeouts.
		compactionThreshold := 0
		if contextTokenBudget > 0 {
			compactionThreshold = contextTokenBudget * 70 / 100
		}
		if compactionThreshold > 0 && estimatedTokens >= compactionThreshold {
			r.logger.Warn(
				"resolve: context reached compaction threshold, running synchronous compaction",
				slog.String("bot_id", req.BotID),
				slog.Int("estimated_tokens", estimatedTokens),
				slog.Int("context_token_budget", contextTokenBudget),
				slog.Int("compaction_threshold", compactionThreshold),
			)
			r.runCompactionSync(ctx, req, estimatedTokens)
			// Reload messages after compaction.
			loaded, loadErr = r.loadMessages(ctx, req.ChatID, req.SessionID, defaultMaxContextMinutes)
			if loadErr != nil {
				r.logger.Error(
					"resolve: reload messages after compaction failed",
					slog.String("bot_id", req.BotID),
					slog.Any("error", loadErr),
				)
				return resolvedContext{}, loadErr
			}
			loaded = pruneHistoryForGateway(loaded)
			loaded = dedupePersistedCurrentUserMessage(loaded, req)
			loaded = r.replaceCompactedMessages(ctx, loaded)
			messages, estimatedTokens = trimMessagesByTokens(r.logger, loaded, contextTokenBudget)
			// Remove tool messages from the recent context — they are large
			// and unnecessary when we already have a summary. Keep only
			// user/assistant conversation turns.
			messages = stripToolMessages(messages)
		}
		_ = estimatedTokens
	}
	if memoryMsg != nil {
		messages = append(messages, *memoryMsg)
	}
	if !usePipeline {
		messages = append(messages, reqMessages...)
	}
	messages = sanitizeMessages(messages)
	// Strip tool messages and tool-call-only assistant messages from context.
	// Tool outputs are large and waste tokens; the LLM doesn't need raw tool
	// results when summaries and memory tools are available for lookup.
	if len(messages) > 10 {
		messages = stripToolMessages(messages)
	}
	messages = repairToolCallClosures(messages, syntheticToolClosureError)

	displayName := r.resolveDisplayName(ctx, req)
	mergedAttachments := r.routeAndMergeAttachments(ctx, chatModel, req)

	tz := runCfg.Identity.TimezoneLocation
	if tz == nil {
		tz = time.UTC
	}
	headerifiedQuery := FormatUserHeader(UserMessageHeaderInput{
		MessageID:         strings.TrimSpace(req.ExternalMessageID),
		ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
		DisplayName:       displayName,
		Channel:           req.CurrentChannel,
		ConversationType:  strings.TrimSpace(req.ConversationType),
		ConversationName:  strings.TrimSpace(req.ConversationName),
		Target:            strings.TrimSpace(req.ReplyTarget),
		AttachmentPaths:   extractAttachmentPaths(mergedAttachments),
		Time:              time.Now().In(tz),
		Timezone:          runCfg.Identity.Timezone,
	}, req.Query)
	runCfg.Messages = modelMessagesToSDKMessages(nonNilModelMessages(messages))
	// When using the pipeline the user message is already in the RC;
	// don't send it to the LLM again. headerifiedQuery is still kept
	// for storeRound so the user message gets persisted.
	if !usePipeline {
		runCfg.Query = headerifiedQuery
	}
	runCfg.InlineImages = extractNativeImageParts(mergedAttachments)

	var injectedRecords *[]conversation.InjectedMessageRecord
	if req.InjectCh != nil {
		agentInjectCh := make(chan agentpkg.InjectMessage, cap(req.InjectCh))
		go func() {
			for msg := range req.InjectCh {
				agentMsg := agentpkg.InjectMessage{
					Text:            msg.Text,
					HeaderifiedText: msg.HeaderifiedText,
				}
				// Inline any image attachments from the injected message so the
				// model receives them as vision input alongside the text.
				if runCfg.SupportsImageInput && len(msg.Attachments) > 0 {
					agentMsg.ImageParts = r.inlineInjectAttachments(ctx, req.BotID, msg.Attachments)
				}
				agentInjectCh <- agentMsg
			}
			close(agentInjectCh)
		}()
		runCfg.InjectCh = agentInjectCh

		records := make([]conversation.InjectedMessageRecord, 0)
		injectedRecords = &records
		var recMu sync.Mutex
		runCfg.InjectedRecorder = func(headerifiedText string, insertAfter int) {
			recMu.Lock()
			*injectedRecords = append(*injectedRecords, conversation.InjectedMessageRecord{
				HeaderifiedText: headerifiedText,
				InsertAfter:     insertAfter,
			})
			recMu.Unlock()
		}
	}

	return resolvedContext{
		runConfig:       runCfg,
		model:           chatModel,
		provider:        provider,
		query:           headerifiedQuery,
		injectedRecords: injectedRecords,
		estimatedTokens: estimatedTokens,
	}, nil
}

// Chat sends a synchronous chat request and stores the result.
func (r *Resolver) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	rc, err := r.resolve(ctx, req)
	if err != nil {
		return conversation.ChatResponse{}, err
	}
	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	req.Query = rc.query

	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)

	cfg := rc.runConfig
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return conversation.ChatResponse{}, err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(req.Query, outputMessages)
	if err := r.storeRound(ctx, req, roundMessages, rc.model.ID); err != nil {
		return conversation.ChatResponse{}, err
	}

	if result.Usage != nil {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, result.Usage.InputTokens)
	}

	return conversation.ChatResponse{
		Messages: outputMessages,
		Model:    rc.model.ModelID,
		Provider: rc.provider.ClientType,
	}, nil
}

// baseRunConfigParams holds parameters for buildBaseRunConfig that differ
// between chat and discuss callers.
type baseRunConfigParams struct {
	BotID             string
	ChatID            string
	SessionID         string
	RouteID           string
	UserID            string
	ChannelIdentityID string
	CurrentPlatform   string
	ReplyTarget       string
	ConversationType  string
	SessionToken      string //nolint:gosec // session credential material, not a hardcoded secret
	SessionType       string
	Model             string
	Provider          string
	ReasoningEffort   string // caller-provided override (empty = use bot default)
}

// buildBaseRunConfig creates a RunConfig with model, credentials, skills,
// identity and system prompt — everything except Messages/Query/InlineImages.
// Both resolve() and ResolveRunConfig() delegate to this shared builder.
func (r *Resolver) buildBaseRunConfig(ctx context.Context, p baseRunConfigParams) (agentpkg.RunConfig, models.GetResponse, sqlc.Provider, error) {
	botSettings, err := r.loadBotSettings(ctx, p.BotID)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, err
	}
	loopDetectionEnabled := r.loadBotLoopDetectionEnabled(ctx, p.BotID)
	userTimezoneName, userClockLocation := r.resolveTimezone(ctx, p.BotID, p.UserID)

	chatID := p.ChatID
	if chatID == "" {
		chatID = p.BotID
	}

	req := buildModelSelectionRequest(p, chatID)

	chatModel, provider, err := r.selectChatModel(ctx, req, botSettings, conversation.Settings{})
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, err
	}

	reasoningEffort := p.ReasoningEffort
	if reasoningEffort == "" && chatModel.HasCompatibility(models.CompatReasoning) && botSettings.ReasoningEnabled {
		reasoningEffort = botSettings.ReasoningEffort
	}
	var reasoningConfig *models.ReasoningConfig
	if reasoningEffort != "" {
		reasoningConfig = &models.ReasoningConfig{Enabled: true, Effort: reasoningEffort}
	}

	authResolver := providers.NewService(nil, r.queries, "")
	authCtx := oauthctx.WithUserID(ctx, p.UserID)
	creds, err := authResolver.ResolveModelCredentials(authCtx, provider)
	if err != nil {
		return agentpkg.RunConfig{}, models.GetResponse{}, sqlc.Provider{}, fmt.Errorf("resolve provider credentials: %w", err)
	}

	sdkModel := models.NewSDKChatModel(models.SDKModelConfig{
		ModelID:         chatModel.ModelID,
		ClientType:      provider.ClientType,
		APIKey:          creds.APIKey,
		CodexAccountID:  creds.CodexAccountID,
		BaseURL:         providers.ProviderConfigString(provider, "base_url"),
		HTTPClient:      r.streamHTTPClient,
		ReasoningConfig: reasoningConfig,
	})

	var agentSkills []agentpkg.SkillEntry
	if r.skillLoader != nil {
		entries, skillErr := r.skillLoader.LoadSkills(ctx, p.BotID)
		if skillErr != nil {
			r.logger.Warn("failed to load skills", slog.String("bot_id", p.BotID), slog.Any("error", skillErr))
		} else {
			for _, e := range entries {
				if skill, ok := normalizeGatewaySkill(e); ok {
					agentSkills = append(agentSkills, skill)
				}
			}
		}
	}
	if agentSkills == nil {
		agentSkills = []agentpkg.SkillEntry{}
	}

	cfg := agentpkg.RunConfig{
		Model:              sdkModel,
		ReasoningEffort:    reasoningEffort,
		PromptCacheTTL:     providers.ProviderConfigString(provider, "prompt_cache_ttl"),
		SessionType:        p.SessionType,
		SupportsImageInput: chatModel.HasCompatibility(models.CompatVision),
		SupportsToolCall:   chatModel.HasCompatibility(models.CompatToolCall),
		DisplayEnabled:     botSettings.DisplayEnabled,
		Identity: agentpkg.SessionContext{
			BotID:             p.BotID,
			ChatID:            chatID,
			SessionID:         p.SessionID,
			ChannelIdentityID: strings.TrimSpace(p.ChannelIdentityID),
			CurrentPlatform:   p.CurrentPlatform,
			ReplyTarget:       strings.TrimSpace(p.ReplyTarget),
			ConversationType:  strings.TrimSpace(p.ConversationType),
			Timezone:          userTimezoneName,
			TimezoneLocation:  userClockLocation,
			SessionToken:      p.SessionToken,
		},
		Skills:            agentSkills,
		LoopDetection:     agentpkg.LoopDetectionConfig{Enabled: loopDetectionEnabled},
		BackgroundManager: r.bgManager,
	}
	if r.toolApproval != nil {
		cfg.ToolApprovalHandler = r.buildToolApprovalHandler(p)
	}

	return cfg, chatModel, provider, nil
}

func (r *Resolver) buildToolApprovalHandler(p baseRunConfigParams) func(context.Context, sdk.ToolCall) (sdk.ToolApprovalResult, error) {
	return func(ctx context.Context, call sdk.ToolCall) (sdk.ToolApprovalResult, error) {
		input := toolapproval.CreatePendingInput{
			BotID:                        p.BotID,
			SessionID:                    p.SessionID,
			RouteID:                      p.RouteID,
			ChannelIdentityID:            p.ChannelIdentityID,
			RequestedByChannelIdentityID: p.ChannelIdentityID,
			ToolCallID:                   call.ToolCallID,
			ToolName:                     call.ToolName,
			ToolInput:                    call.Input,
			SourcePlatform:               p.CurrentPlatform,
			ReplyTarget:                  p.ReplyTarget,
			ConversationType:             p.ConversationType,
		}
		eval, err := r.toolApproval.EvaluatePolicy(ctx, input)
		if err != nil {
			return sdk.ToolApprovalResult{}, err
		}
		if eval.Decision == toolapproval.DecisionBypass {
			return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
		}
		if !isInteractiveApprovalSession(p.SessionType) {
			req, err := r.toolApproval.CreatePending(ctx, input)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			reason := "tool execution requires approval, but this session type cannot request approval"
			rejected, err := r.toolApproval.Reject(ctx, req.ID, p.ChannelIdentityID, reason)
			if err != nil {
				return sdk.ToolApprovalResult{}, err
			}
			return sdk.ToolApprovalResult{
				Decision:   sdk.ToolApprovalDecisionRejected,
				ApprovalID: rejected.ID,
				Reason:     reason,
				Metadata:   approvalResultMetadata(rejected),
			}, nil
		}
		eval, err = r.toolApproval.Evaluate(ctx, input)
		if err != nil {
			return sdk.ToolApprovalResult{}, err
		}
		return sdk.ToolApprovalResult{
			Decision:   sdk.ToolApprovalDecisionDeferred,
			ApprovalID: eval.Request.ID,
			Metadata:   approvalResultMetadata(eval.Request),
		}, nil
	}
}

func approvalResultMetadata(req toolapproval.Request) map[string]any {
	return map[string]any{
		"short_id":     req.ShortID,
		"status":       req.Status,
		"tool_name":    req.ToolName,
		"tool_call_id": req.ToolCallID,
	}
}

func isInteractiveApprovalSession(sessionType string) bool {
	switch strings.ToLower(strings.TrimSpace(sessionType)) {
	case "", "chat":
		return true
	default:
		return false
	}
}

func buildModelSelectionRequest(p baseRunConfigParams, chatID string) conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:          p.BotID,
		ChatID:         chatID,
		SessionID:      p.SessionID,
		CurrentChannel: p.CurrentPlatform,
		Model:          p.Model,
		Provider:       p.Provider,
	}
}

// ResolveRunConfig builds a complete RunConfig (model, system prompt, tools,
// identity) for a bot+session without loading messages or requiring a query.
// The caller is responsible for filling RunConfig.Messages.
// Used by the discuss driver to reuse the resolver's model/tools/prompt pipeline.
func (r *Resolver) ResolveRunConfig(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (pipelinepkg.ResolveRunConfigResult, error) {
	if strings.TrimSpace(botID) == "" {
		return pipelinepkg.ResolveRunConfigResult{}, errors.New("bot id is required")
	}

	cfg, chatModel, _, err := r.buildBaseRunConfig(ctx, baseRunConfigParams{
		BotID:             botID,
		SessionID:         sessionID,
		ChannelIdentityID: channelIdentityID,
		CurrentPlatform:   currentPlatform,
		ReplyTarget:       replyTarget,
		ConversationType:  conversationType,
		SessionToken:      chatToken,
		SessionType:       "discuss",
	})
	if err != nil {
		return pipelinepkg.ResolveRunConfigResult{}, err
	}

	cfg = r.prepareRunConfig(ctx, cfg)
	return pipelinepkg.ResolveRunConfigResult{
		RunConfig: cfg,
		ModelID:   chatModel.ID,
	}, nil
}

// prepareRunConfig generates the system prompt and appends the user message.
func (r *Resolver) prepareRunConfig(ctx context.Context, cfg agentpkg.RunConfig) agentpkg.RunConfig {
	supportsImageInput := cfg.SupportsImageInput
	var files []agentpkg.SystemFile
	if r.agent != nil {
		nowFn := time.Now
		if cfg.Identity.TimezoneLocation != nil {
			nowFn = func() time.Time { return time.Now().In(cfg.Identity.TimezoneLocation) }
		}
		fs := agentpkg.NewFSClient(r.agent.BridgeProvider(), cfg.Identity.BotID, nowFn)
		files = fs.LoadSystemFiles(ctx)
	}

	now := time.Now().UTC()
	if cfg.Identity.TimezoneLocation != nil {
		now = now.In(cfg.Identity.TimezoneLocation)
	}
	platformIdentitiesSection := ""
	if r.channelStore != nil {
		channelConfigs, err := r.channelStore.ListBotConfigs(ctx, cfg.Identity.BotID)
		if err != nil {
			r.logger.Warn(
				"load bot platform identities failed",
				slog.String("bot_id", cfg.Identity.BotID),
				slog.Any("error", err),
			)
		} else {
			platformIdentitiesSection = buildPlatformIdentitiesSection(channelConfigs)
		}
	}
	selfIdentitySection := r.buildSelfIdentitySection(ctx, cfg.Identity.BotID)
	teamSection := r.buildTeamSection(ctx, cfg.Identity.TeamID, cfg.Identity.BotID)
	cfg.System = agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{
		SessionType:               cfg.SessionType,
		Skills:                    cfg.Skills,
		Files:                     files,
		Now:                       now,
		Timezone:                  cfg.Identity.Timezone,
		SupportsImageInput:        supportsImageInput,
		DisplayEnabled:            cfg.DisplayEnabled,
		PlatformIdentitiesSection: platformIdentitiesSection,
		TeamSection:               teamSection,
		SelfIdentitySection:       selfIdentitySection,
	})

	if cfg.Query != "" {
		var extra []sdk.MessagePart
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				extra = append(extra, img)
			}
		}
		cfg.Messages = append(cfg.Messages, sdk.UserMessage(cfg.Query, extra...))
	} else if len(cfg.InlineImages) > 0 {
		// Pipeline path: the user query is already embedded in the RC messages,
		// but image parts are not rendered by the pipeline renderer. Inject the
		// inline images into the last user message so the model receives them.
		imageParts := make([]sdk.MessagePart, 0, len(cfg.InlineImages))
		for _, img := range cfg.InlineImages {
			if strings.TrimSpace(img.Image) != "" {
				imageParts = append(imageParts, img)
			}
		}
		if len(imageParts) > 0 {
			injected := false
			for i := len(cfg.Messages) - 1; i >= 0; i-- {
				if cfg.Messages[i].Role == sdk.MessageRoleUser {
					cfg.Messages[i].Content = append(cfg.Messages[i].Content, imageParts...)
					injected = true
					break
				}
			}
			if !injected {
				cfg.Messages = append(cfg.Messages, sdk.UserMessage("", imageParts...))
			}
		}
	}

	return cfg
}

func normalizeGatewaySkill(entry SkillEntry) (agentpkg.SkillEntry, bool) {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return agentpkg.SkillEntry{}, false
	}
	description := strings.TrimSpace(entry.Description)
	if description == "" {
		description = name
	}
	content := strings.TrimSpace(entry.Content)
	if content == "" {
		content = description
	}
	return agentpkg.SkillEntry{
		Name:        name,
		Description: description,
		Content:     content,
		Path:        strings.TrimSpace(entry.Path),
		Metadata:    entry.Metadata,
	}, true
}

func normalizeUserMessageContent(msg conversation.ModelMessage) conversation.ModelMessage {
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		return msg
	}
	normalized, changed := normalizeUserContentParts(msg.Content)
	if !changed {
		return msg
	}
	msg.Content = normalized
	return msg
}

func normalizeUserContentParts(content json.RawMessage) (json.RawMessage, bool) {
	if len(content) == 0 {
		return nil, false
	}
	var parts []map[string]any
	if err := json.Unmarshal(content, &parts); err != nil || len(parts) == 0 {
		return nil, false
	}

	changed := false
	rebuilt := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(strings.ToLower(readAnyString(part["type"])))
		switch partType {
		case "image":
			normalized, ok, didChange := normalizeUserImagePart(part)
			if didChange {
				changed = true
			}
			if ok {
				rebuilt = append(rebuilt, normalized)
			}
		default:
			rebuilt = append(rebuilt, part)
		}
	}
	if !changed {
		return nil, false
	}
	if len(rebuilt) == 0 {
		rebuilt = append(rebuilt, map[string]any{
			"type": "text",
			"text": "[User sent an attachment]",
		})
	}
	data, err := json.Marshal(rebuilt)
	if err != nil {
		return nil, false
	}
	return data, true
}

func normalizeUserImagePart(part map[string]any) (map[string]any, bool, bool) {
	raw, ok := part["image"]
	if !ok {
		return nil, false, true
	}
	if image, ok := raw.(string); ok && strings.TrimSpace(image) != "" {
		return part, true, false
	}
	bytes, ok := anyIndexedByteObject(raw)
	if !ok {
		return nil, false, true
	}
	cloned := cloneAnyMap(part)
	mediaType := strings.TrimSpace(readAnyString(cloned["mediaType"]))
	encoded := base64.StdEncoding.EncodeToString(bytes)
	if mediaType != "" {
		cloned["image"] = "data:" + mediaType + ";base64," + encoded
	} else {
		cloned["image"] = encoded
	}
	return cloned, true, true
}

func cloneAnyMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func readAnyString(value any) string {
	text, _ := value.(string)
	return text
}

func anyIndexedByteObject(value any) ([]byte, bool) {
	obj, ok := value.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, false
	}
	indexes := make([]int, 0, len(obj))
	values := make(map[int]byte, len(obj))
	for key, raw := range obj {
		idx, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil || idx < 0 {
			return nil, false
		}
		byteValue, ok := anyNumberToByte(raw)
		if !ok {
			return nil, false
		}
		indexes = append(indexes, idx)
		values[idx] = byteValue
	}
	sort.Ints(indexes)
	if indexes[len(indexes)-1]+1 != len(indexes) {
		return nil, false
	}
	bytes := make([]byte, len(indexes))
	for _, idx := range indexes {
		bytes[idx] = values[idx]
	}
	return bytes, true
}

func anyNumberToByte(value any) (byte, bool) {
	floatValue, ok := value.(float64)
	if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
		return 0, false
	}
	if floatValue < 0 || floatValue > 255 || math.Trunc(floatValue) != floatValue {
		return 0, false
	}
	parsed, err := strconv.ParseUint(strconv.FormatFloat(floatValue, 'f', 0, 64), 10, 8)
	if err != nil {
		return 0, false
	}
	return byte(parsed), true
}

// extractAttachmentPaths collects container file paths from ALL gateway
// attachments — both tool_file_ref (fallback) and native images that carry a
// FallbackPath. This ensures the YAML user header always lists every
// attachment the user sent, regardless of whether the model consumes the
// image natively or via the read_media tool.
func extractAttachmentPaths(attachments []any) []string {
	var paths []string
	for _, att := range attachments {
		ga, ok := att.(gatewayAttachment)
		if !ok {
			continue
		}
		if ga.Transport == gatewayTransportToolFileRef && strings.TrimSpace(ga.Payload) != "" {
			paths = append(paths, ga.Payload)
		} else if strings.TrimSpace(ga.FallbackPath) != "" {
			paths = append(paths, ga.FallbackPath)
		}
	}
	return paths
}

// extractNativeImageParts returns sdk.ImagePart entries for attachments that
// the model can consume as inline multimodal input (vision-capable images with
// an inline data URL or public URL payload).
func extractNativeImageParts(attachments []any) []sdk.ImagePart {
	var parts []sdk.ImagePart
	for _, att := range attachments {
		ga, ok := att.(gatewayAttachment)
		if !ok || ga.Type != "image" {
			continue
		}
		transport := strings.ToLower(strings.TrimSpace(ga.Transport))
		if transport != gatewayTransportInlineDataURL && transport != gatewayTransportPublicURL {
			continue
		}
		payload := strings.TrimSpace(ga.Payload)
		if payload == "" {
			continue
		}
		parts = append(parts, sdk.ImagePart{
			Image:     payload,
			MediaType: strings.TrimSpace(ga.Mime),
		})
	}
	return parts
}
