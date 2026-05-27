package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agentteam"
	"github.com/memohai/memoh/internal/session"
)

// TeamSessionMetadataWriter writes team-context hints back into the
// session row so subsequent turns can resolve `#N` issue refs without
// the agent re-stating the team UUID. The interface lets the test
// suite stub the session service without depending on its concrete
// type.
type TeamSessionMetadataWriter interface {
	Get(ctx context.Context, sessionID string) (session.Session, error)
	UpdateMetadata(ctx context.Context, sessionID string, metadata map[string]any) (session.Session, error)
}

// TeamProvider exposes team / issue / handoff tools to agents.
type TeamProvider struct {
	service        *agentteam.Service
	sessionService TeamSessionMetadataWriter
	logger         *slog.Logger
}

// NewTeamProvider builds the team tool provider.
func NewTeamProvider(log *slog.Logger, service *agentteam.Service) *TeamProvider {
	if log == nil {
		log = slog.Default()
	}
	return &TeamProvider{
		service: service,
		logger:  log.With(slog.String("tool", "team")),
	}
}

// SetSessionService injects the writer used to persist team context
// hints (last_team_id / last_issue_id) into the active chat session
// after a successful team tool invocation. When unset the team tools
// still work but later turns in the same session lose the implicit
// team context — they must be re-supplied via the `team` argument.
func (p *TeamProvider) SetSessionService(s TeamSessionMetadataWriter) {
	if p == nil {
		return
	}
	p.sessionService = s
}

// Tools returns the team-aware tool list. The team tools are not registered
// when the bot is running as a subagent (those run inside a single bot's
// context and should not initiate cross-bot handoffs).
func (p *TeamProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p == nil || p.service == nil {
		return nil, nil
	}
	if session.IsSubagent {
		return nil, nil
	}
	if strings.TrimSpace(session.BotID) == "" {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "team_list",
			Description: "List the teams the current bot belongs to. Returns id, name, description, shared_dir_name, and whether the bot has a designated role.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, _ any) (any, error) {
				teams, err := p.service.ListTeamsForBot(ctx.Context, sess.BotID)
				if err != nil {
					return nil, err
				}
				items := make([]map[string]any, 0, len(teams))
				for _, t := range teams {
					items = append(items, map[string]any{
						"id":              t.ID,
						"name":            t.Name,
						"description":     t.Description,
						"shared_dir_name": t.SharedDirName,
					})
				}
				return map[string]any{"ok": true, "count": len(items), "teams": items}, nil
			},
		},
		{
			Name:        "team_members",
			Description: "List members of a team. Each member's name comes from the underlying bot or user record (no separate per-team display name). Use the `mention` field as ready-to-paste text to @mention that member in an issue comment.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team": map[string]any{"type": "string", "description": "Team name (preferred, e.g. \"backend\") or full UUID. Defaults to the current session team."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				team, err := p.resolveTeamRef(ctx.Context, sess, extractTeamArg(args))
				if err != nil {
					return nil, err
				}
				members, err := p.service.ListMembers(ctx.Context, team.ID)
				if err != nil {
					return nil, err
				}
				items := make([]map[string]any, 0, len(members))
				for _, m := range members {
					items = append(items, map[string]any{
						"id":           m.ID,
						"member_type":  string(m.MemberType),
						"bot_id":       m.BotID,
						"user_id":      m.UserID,
						"role":         m.Role,
						"display_name": m.DisplayName,
						"instructions": m.Instructions,
						"mention":      formatMentionToken(m.DisplayName),
					})
				}
				return map[string]any{
					"ok":        true,
					"team_id":   team.ID,
					"team_name": team.Name,
					"count":     len(items),
					"members":   items,
				}, nil
			},
		},
		{
			Name:        "issue_create",
			Description: "Create a new team issue. Use when the user asks for a task that needs multi-step or multi-bot collaboration, persistent tracking, or shared files. Returns the new issue with its id and number.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team":             map[string]any{"type": "string", "description": "Team name (preferred, e.g. \"backend\") or full UUID. Defaults to the current session team."},
					"title":            map[string]any{"type": "string", "description": "Short issue title."},
					"description":      map[string]any{"type": "string", "description": "Detailed description / requirements."},
					"status":           map[string]any{"type": "string", "description": "Initial status: backlog, todo (default), in_progress, blocked, review."},
					"assignee_bot_id":  map[string]any{"type": "string", "description": "Optional bot to assign as the executor. Accepts the bot's display name (preferred, e.g. \"Backend Bot\") or its UUID."},
					"assignee_user_id": map[string]any{"type": "string", "description": "Optional user to assign. Accepts the user's display name or UUID."},
					"parent_issue": map[string]any{
						"description": "Optional parent issue (per-team integer number or UUID) for sub-tasks.",
						"oneOf": []any{
							map[string]any{"type": "integer"},
							map[string]any{"type": "string"},
						},
					},
				},
				"required": []string{"title"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				team, err := p.resolveTeamRef(ctx.Context, sess, extractTeamArg(args))
				if err != nil {
					return nil, err
				}
				title := strings.TrimSpace(StringArg(args, "title"))
				if title == "" {
					return nil, errors.New("title is required")
				}
				status := agentteam.IssueStatus(strings.TrimSpace(StringArg(args, "status")))
				assigneeBot, assigneeUser, assigneeType, err := p.resolveAssigneeIDs(
					ctx.Context,
					team.ID,
					StringArg(args, "assignee_bot_id"),
					StringArg(args, "assignee_user_id"),
				)
				if err != nil {
					return nil, err
				}
				parentRef := strings.TrimSpace(StringArg(args, "parent_issue"))
				if parentRef == "" {
					parentRef = strings.TrimSpace(StringArg(args, "parent_issue_id"))
				}
				parentUUID := ""
				if parentRef != "" {
					parent, perr := p.service.ResolveIssueRef(ctx.Context, team.ID, parentRef)
					if perr != nil {
						return nil, errors.New("parent_issue: " + perr.Error())
					}
					parentUUID = parent.ID
				}
				issue, err := p.service.CreateIssue(ctx.Context, agentteam.CreateIssueInput{
					TeamID:          team.ID,
					Title:           title,
					Description:     StringArg(args, "description"),
					Status:          status,
					AssigneeType:    assigneeType,
					AssigneeBotID:   assigneeBot,
					AssigneeUserID:  assigneeUser,
					CreatedByType:   agentteam.ActorBot,
					CreatedByBotID:  sess.BotID,
					ParentIssueID:   parentUUID,
					SourceSessionID: sess.SessionID,
				})
				if err != nil {
					return nil, p.translateTeamFKError(ctx.Context, sess, team.ID, err)
				}
				p.rememberTeamContext(ctx.Context, sess, issue.TeamID, issue.ID)
				return issueToMap(issue), nil
			},
		},
		{
			Name:        "issue_list",
			Description: "List issues in a team. Pass open=true to limit the result to non-terminal statuses.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team": map[string]any{"type": "string", "description": "Team name (preferred, e.g. \"backend\") or full UUID. Defaults to the current session team."},
					"open": map[string]any{"type": "boolean", "description": "When true, only list issues that are not done/cancelled."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				team, err := p.resolveTeamRef(ctx.Context, sess, extractTeamArg(args))
				if err != nil {
					return nil, err
				}
				onlyOpen, _, err := BoolArg(args, "open")
				if err != nil {
					return nil, err
				}
				var (
					issues []agentteam.Issue
					ierr   error
				)
				if onlyOpen {
					issues, ierr = p.service.ListOpenIssuesByTeam(ctx.Context, team.ID)
				} else {
					issues, ierr = p.service.ListIssuesByTeam(ctx.Context, team.ID)
				}
				if ierr != nil {
					return nil, ierr
				}
				items := make([]map[string]any, 0, len(issues))
				for _, i := range issues {
					items = append(items, issueToMap(i))
				}
				return map[string]any{
					"ok":        true,
					"team_id":   team.ID,
					"team_name": team.Name,
					"count":     len(items),
					"issues":    items,
				}, nil
			},
		},
		{
			Name:        "issue_get",
			Description: "Fetch a team issue with its comments. Defaults to the current session's issue when `issue` is omitted.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue": map[string]any{
						"description": "Issue reference. Either the per-team integer number (e.g. 3 or \"#3\") or the full UUID. Defaults to the current session issue.",
						"oneOf": []any{
							map[string]any{"type": "integer"},
							map[string]any{"type": "string"},
						},
					},
					"team": map[string]any{"type": "string", "description": "Team name or UUID. Required for numeric issue refs when the session has no implicit team. Defaults to the current session team."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				issue, err := p.resolveSessionIssue(ctx.Context, sess, args)
				if err != nil {
					return nil, err
				}
				comments, err := p.service.ListComments(ctx.Context, issue.ID)
				if err != nil {
					return nil, err
				}
				commentItems := make([]map[string]any, 0, len(comments))
				for _, cmt := range comments {
					commentItems = append(commentItems, commentToMap(cmt))
				}
				return map[string]any{
					"ok":       true,
					"issue":    issueToMap(issue),
					"comments": commentItems,
				}, nil
			},
		},
		{
			Name: "issue_comment",
			Description: "Post a comment on a team issue. " +
				"To delegate work, write `@<TeamMemberName>` exactly as listed in the team roster (use the quoted form `@\"Name With Spaces\"` when needed). " +
				"When you are answering a `@mention` from another bot, the reply is automatically threaded under that mention so the return wakes the right session — leave `parent_comment_id` empty unless you really want to start a new sub-thread. " +
				"Defaults to the current session's issue.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue": map[string]any{
						"description": "Issue reference. Either the per-team integer number (e.g. 3 or \"#3\") or the full UUID. Defaults to the current session issue.",
						"oneOf": []any{
							map[string]any{"type": "integer"},
							map[string]any{"type": "string"},
						},
					},
					"team":              map[string]any{"type": "string", "description": "Team name or UUID. Required for numeric issue refs when the session has no implicit team. Defaults to the current session team."},
					"content":           map[string]any{"type": "string", "description": "Markdown content for the comment."},
					"parent_comment_id": map[string]any{"type": "string", "description": "Optional parent comment ID to reply in a thread. When the bot is running inside a handoff, the trigger comment is used automatically; pass an explicit value (or the special token \"none\") to override."},
				},
				"required": []string{"content"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				issue, err := p.resolveSessionIssue(ctx.Context, sess, args)
				if err != nil {
					return nil, err
				}
				issueID := issue.ID
				content := StringArg(args, "content")
				if strings.TrimSpace(content) == "" {
					return nil, errors.New("content is required")
				}

				// Resolve parent_comment_id: explicit "none" means
				// "post top-level even though a handoff is active";
				// any non-empty value is used as-is; otherwise we
				// default to the handoff's trigger comment so the
				// dispatcher can route the closure deterministically.
				parent := strings.TrimSpace(StringArg(args, "parent_comment_id"))
				switch strings.ToLower(parent) {
				case "none", "null", "false", "-":
					parent = ""
				case "":
					if strings.TrimSpace(sess.HandoffID) != "" {
						if trigger, err := p.resolveHandoffTriggerComment(ctx.Context, sess.HandoffID); err == nil {
							parent = trigger
						}
					}
				}

				cmt, err := p.service.PostComment(ctx.Context, agentteam.CreateCommentInput{
					IssueID:         issueID,
					AuthorType:      agentteam.ActorBot,
					AuthorBotID:     sess.BotID,
					Content:         content,
					ParentCommentID: parent,
					SourceSessionID: sess.SessionID,
				})
				if err != nil {
					return nil, err
				}
				p.rememberTeamContext(ctx.Context, sess, cmt.TeamID, cmt.IssueID)
				return commentToMap(cmt), nil
			},
		},
		{
			Name:        "issue_status",
			Description: "Update an issue's status. Allowed values: backlog, todo, in_progress, blocked, review, done, cancelled.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue": map[string]any{
						"description": "Issue reference. Either the per-team integer number (e.g. 3 or \"#3\") or the full UUID. Defaults to the current session issue.",
						"oneOf": []any{
							map[string]any{"type": "integer"},
							map[string]any{"type": "string"},
						},
					},
					"team":   map[string]any{"type": "string", "description": "Team name or UUID. Required for numeric issue refs when the session has no implicit team. Defaults to the current session team."},
					"status": map[string]any{"type": "string", "description": "New status."},
				},
				"required": []string{"status"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				target, err := p.resolveSessionIssue(ctx.Context, sess, args)
				if err != nil {
					return nil, err
				}
				status := agentteam.IssueStatus(strings.TrimSpace(StringArg(args, "status")))
				if status == "" {
					return nil, errors.New("status is required")
				}
				updated, err := p.service.UpdateIssue(ctx.Context, target.ID, agentteam.UpdateIssueInput{Status: &status})
				if err != nil {
					return nil, err
				}
				p.rememberTeamContext(ctx.Context, sess, updated.TeamID, updated.ID)
				return issueToMap(updated), nil
			},
		},
		{
			Name:        "issue_assign",
			Description: "Assign or reassign a team issue. Pass empty values to clear the assignee.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"issue": map[string]any{
						"description": "Issue reference. Either the per-team integer number (e.g. 3 or \"#3\") or the full UUID. Defaults to the current session issue.",
						"oneOf": []any{
							map[string]any{"type": "integer"},
							map[string]any{"type": "string"},
						},
					},
					"team":             map[string]any{"type": "string", "description": "Team name or UUID. Required for numeric issue refs when the session has no implicit team. Defaults to the current session team."},
					"assignee_bot_id":  map[string]any{"type": "string", "description": "Bot to assign. Accepts the bot's display name (preferred) or UUID. Mutually exclusive with assignee_user_id."},
					"assignee_user_id": map[string]any{"type": "string", "description": "User to assign. Accepts the user's display name or UUID. Mutually exclusive with assignee_bot_id."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				target, err := p.resolveSessionIssue(ctx.Context, sess, args)
				if err != nil {
					return nil, err
				}
				botID, userID, assigneeType, err := p.resolveAssigneeIDs(
					ctx.Context,
					target.TeamID,
					StringArg(args, "assignee_bot_id"),
					StringArg(args, "assignee_user_id"),
				)
				if err != nil {
					return nil, err
				}
				updated, err := p.service.AssignIssue(ctx.Context, target.ID, agentteam.AssignIssueInput{
					AssigneeType:   assigneeType,
					AssigneeBotID:  botID,
					AssigneeUserID: userID,
				})
				if err != nil {
					return nil, err
				}
				p.rememberTeamContext(ctx.Context, sess, updated.TeamID, updated.ID)
				return issueToMap(updated), nil
			},
		},
	}, nil
}

// extractTeamArg returns the raw team identifier supplied by the agent.
// It accepts the canonical `team` key first and falls back to the
// legacy `team_id` alias so existing prompts / tool calls keep working
// while the new schema is rolled out.
func extractTeamArg(args map[string]any) string {
	if v := strings.TrimSpace(StringArg(args, "team")); v != "" {
		return v
	}
	return strings.TrimSpace(StringArg(args, "team_id"))
}

// resolveTeamRef converts an agent-supplied team reference (name or
// UUID) into a concrete team. It replaces the previous "always UUID"
// contract that proved hostile to LLMs which kept hallucinating
// look-alike UUIDs (e.g. "38a4d160-21ab-43ce-..." instead of the real
// "38a4d160-9770-46a3-...").
//
// Resolution order:
//  1. Empty ref → fall back to the session's TeamID. If that is also
//     empty, return an error that lists the teams the bot belongs to
//     so the model can self-correct on the next turn.
//  2. Looks like a UUID → load via GetTeam to validate existence.
//  3. Otherwise → treat as a display name and look it up in the bot's
//     own team list using case-insensitive exact match. Names that
//     match zero or multiple teams produce errors that include the
//     candidate set, again so the model can self-correct.
func (p *TeamProvider) resolveTeamRef(ctx context.Context, sess SessionContext, ref string) (agentteam.Team, error) {
	if p == nil || p.service == nil {
		return agentteam.Team{}, errors.New("team service not configured")
	}
	cleaned := strings.TrimSpace(ref)
	if cleaned == "" {
		if sessTeam := strings.TrimSpace(sess.TeamID); sessTeam != "" {
			return p.service.GetTeam(ctx, sessTeam)
		}
		return agentteam.Team{}, fmt.Errorf("team is required: %s", p.candidatesHint(ctx, sess))
	}
	if isUUIDLike(cleaned) {
		t, err := p.service.GetTeam(ctx, cleaned)
		if err != nil {
			return agentteam.Team{}, fmt.Errorf("team not found: %s (%s)", cleaned, p.candidatesHint(ctx, sess))
		}
		return t, nil
	}
	teams, err := p.service.ListTeamsForBot(ctx, sess.BotID)
	if err != nil {
		return agentteam.Team{}, fmt.Errorf("list teams: %w", err)
	}
	matches := make([]agentteam.Team, 0, 2)
	for _, t := range teams {
		if strings.EqualFold(strings.TrimSpace(t.Name), cleaned) {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return agentteam.Team{}, fmt.Errorf("team %q not found among your teams: %s", cleaned, formatTeamCandidates(teams))
	default:
		return agentteam.Team{}, fmt.Errorf("ambiguous team name %q, %s", cleaned, formatTeamMatchUUIDs(matches))
	}
}

// candidatesHint renders a "your teams: [...]" suffix used in error
// messages. Falls back to a shorter form when team enumeration fails so
// the agent always sees an actionable hint.
func (p *TeamProvider) candidatesHint(ctx context.Context, sess SessionContext) string {
	if p == nil || p.service == nil || strings.TrimSpace(sess.BotID) == "" {
		return "no team context available"
	}
	teams, err := p.service.ListTeamsForBot(ctx, sess.BotID)
	if err != nil {
		return "team list unavailable"
	}
	return "your teams: " + formatTeamCandidates(teams)
}

// formatTeamCandidates renders teams in a stable, agent-friendly form.
// Order is alphabetical by name so prompt-cache keys stay stable across
// turns (the underlying ListTeamsForBot order is not guaranteed).
func formatTeamCandidates(teams []agentteam.Team) string {
	if len(teams) == 0 {
		return "[]"
	}
	names := make([]string, 0, len(teams))
	for _, t := range teams {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			name = t.ID
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

func formatTeamMatchUUIDs(matches []agentteam.Team) string {
	if len(matches) == 0 {
		return "use UUID: []"
	}
	ids := make([]string, 0, len(matches))
	for _, t := range matches {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return "use UUID: [" + strings.Join(ids, ", ") + "]"
}

// isUUIDLike reports whether a string parses as an RFC 4122 UUID.
// Anything that does not parse is treated as a display name by
// resolveTeamRef.
func isUUIDLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

// resolveSessionIssue picks an Issue from tool arguments and session
// context. It accepts:
//   - `issue` — per-team integer (3 / "#3") or UUID; required for
//     numeric refs to be scoped to the right team.
//   - `team` (or legacy `team_id`) — explicit team override that wins
//     over the session's TeamID. Useful when the agent wants to
//     reference an issue in a team that is not the session's default,
//     and required when the session has no implicit team context.
//
// When `issue` is omitted the active session's IssueID (always a UUID)
// is used.
func (p *TeamProvider) resolveSessionIssue(ctx context.Context, sess SessionContext, args map[string]any) (agentteam.Issue, error) {
	if p == nil || p.service == nil {
		return agentteam.Issue{}, errors.New("team service not configured")
	}
	teamID, err := p.resolveScopeTeamID(ctx, sess, args)
	if err != nil {
		return agentteam.Issue{}, err
	}
	raw, ok := args["issue"]
	if !ok || raw == nil {
		if v := strings.TrimSpace(StringArg(args, "issue_id")); v != "" {
			raw = v
		}
	}
	switch v := raw.(type) {
	case nil:
		if strings.TrimSpace(sess.IssueID) == "" {
			return agentteam.Issue{}, errors.New("no active session issue — pass `issue`")
		}
		return p.service.GetIssue(ctx, sess.IssueID)
	case string:
		return p.resolveIssueByRef(ctx, sess, teamID, v)
	case float64:
		if v != float64(int64(v)) {
			return agentteam.Issue{}, errors.New("issue must be an integer or string")
		}
		return p.resolveIssueByRef(ctx, sess, teamID, strconv.FormatInt(int64(v), 10))
	case int:
		return p.resolveIssueByRef(ctx, sess, teamID, strconv.Itoa(v))
	case int32:
		return p.resolveIssueByRef(ctx, sess, teamID, strconv.FormatInt(int64(v), 10))
	case int64:
		return p.resolveIssueByRef(ctx, sess, teamID, strconv.FormatInt(v, 10))
	default:
		return p.resolveIssueByRef(ctx, sess, teamID, strings.TrimSpace(StringArg(args, "issue")))
	}
}

// resolveIssueByRef wraps service.ResolveIssueRef with friendlier
// error messages for the agent loop. The service-layer messages name
// the right field but lack candidate hints; we add the bot's team list
// (or, for "issue not found in this team", the issue range) so the
// model can self-correct on the next turn instead of guessing UUIDs.
func (p *TeamProvider) resolveIssueByRef(ctx context.Context, sess SessionContext, teamID, ref string) (agentteam.Issue, error) {
	cleaned := strings.TrimPrefix(strings.TrimSpace(ref), "#")
	_, isNumeric := parsePositiveInt32(cleaned)
	if isNumeric && strings.TrimSpace(teamID) == "" {
		return agentteam.Issue{}, fmt.Errorf("issue %q is a numeric ref — pass `team` (or set the session team). %s", ref, p.candidatesHint(ctx, sess))
	}
	issue, err := p.service.ResolveIssueRef(ctx, teamID, ref)
	if err != nil {
		if isNumeric {
			return agentteam.Issue{}, fmt.Errorf("issue %q not found in team %s: %w", ref, teamID, err)
		}
		return agentteam.Issue{}, err
	}
	return issue, nil
}

func parsePositiveInt32(s string) (int32, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return 0, false
	}
	return int32(n), true
}

// resolveScopeTeamID returns the team UUID that should scope a numeric
// issue reference for the current call. Precedence:
//  1. Explicit `team` (or `team_id`) tool argument — resolved via
//     resolveTeamRef so name / UUID both work.
//  2. Session's TeamID (set by TriggerHandoff or hydrated from
//     bot_sessions.metadata).
//  3. Empty — caller is free to pass a UUID issue ref. Numeric refs
//     will surface their own "needs a team_id" error from the service
//     layer with no candidate hint here.
func (p *TeamProvider) resolveScopeTeamID(ctx context.Context, sess SessionContext, args map[string]any) (string, error) {
	if explicit := extractTeamArg(args); explicit != "" {
		team, err := p.resolveTeamRef(ctx, sess, explicit)
		if err != nil {
			return "", err
		}
		return team.ID, nil
	}
	return strings.TrimSpace(sess.TeamID), nil
}

func issueToMap(i agentteam.Issue) map[string]any {
	out := map[string]any{
		"id":           i.ID,
		"team_id":      i.TeamID,
		"number":       i.Number,
		"ref":          "#" + strconv.FormatInt(int64(i.Number), 10),
		"title":        i.Title,
		"description":  i.Description,
		"status":       string(i.Status),
		"created_by":   map[string]any{"type": string(i.CreatedByType), "bot_id": i.CreatedByBotID, "user_id": i.CreatedByUserID},
		"parent_issue": i.ParentIssueID,
		"created_at":   i.CreatedAt,
		"updated_at":   i.UpdatedAt,
	}
	if i.AssigneeType != "" {
		out["assignee"] = map[string]any{
			"type":    i.AssigneeType,
			"bot_id":  i.AssigneeBotID,
			"user_id": i.AssigneeUserID,
		}
	}
	return out
}

func commentToMap(cmt agentteam.Comment) map[string]any {
	return map[string]any{
		"id":                cmt.ID,
		"issue_id":          cmt.IssueID,
		"team_id":           cmt.TeamID,
		"parent_comment_id": cmt.ParentCommentID,
		"author": map[string]any{
			"type":    string(cmt.AuthorType),
			"bot_id":  cmt.AuthorBotID,
			"user_id": cmt.AuthorUserID,
		},
		"content":    cmt.Content,
		"created_at": cmt.CreatedAt,
	}
}

// resolveAssigneeIDs converts assignee_bot_id / assignee_user_id raw
// inputs into concrete bot / user IDs. The values are agent-friendly:
// either a UUID (preserved as-is) or a display name that is resolved
// against the team's roster. Resolving by name uses the same
// case-insensitive exact-match policy as MatchMember so we don't drift
// from the @mention contract.
//
// Returns the resolved (botID, userID, type, err). When both inputs
// are empty the result clears the assignee.
func (p *TeamProvider) resolveAssigneeIDs(ctx context.Context, teamID, rawBot, rawUser string) (string, string, string, error) {
	botInput := strings.TrimSpace(rawBot)
	userInput := strings.TrimSpace(rawUser)
	if botInput == "" && userInput == "" {
		return "", "", "", nil
	}
	if botInput != "" && userInput != "" {
		return "", "", "", errors.New("assignee_bot_id and assignee_user_id are mutually exclusive")
	}

	var roster []agentteam.Member
	loadRoster := func() ([]agentteam.Member, error) {
		if roster != nil {
			return roster, nil
		}
		members, err := p.service.ListMembers(ctx, teamID)
		if err != nil {
			return nil, err
		}
		roster = members
		return roster, nil
	}

	if botInput != "" {
		if isUUIDLike(botInput) {
			return botInput, "", "bot", nil
		}
		members, err := loadRoster()
		if err != nil {
			return "", "", "", err
		}
		matched, ok := agentteam.MatchMember(botInput, members)
		if !ok || matched.MemberType != agentteam.MemberBot {
			return "", "", "", fmt.Errorf("assignee_bot_id %q did not match a bot in the team", botInput)
		}
		return matched.BotID, "", "bot", nil
	}

	if isUUIDLike(userInput) {
		return "", userInput, "user", nil
	}
	members, err := loadRoster()
	if err != nil {
		return "", "", "", err
	}
	matched, ok := agentteam.MatchMember(userInput, members)
	if !ok || matched.MemberType != agentteam.MemberUser {
		return "", "", "", fmt.Errorf("assignee_user_id %q did not match a user in the team", userInput)
	}
	return "", matched.UserID, "user", nil
}

// translateTeamFKError converts the raw foreign-key violation that the
// store layer surfaces when a non-existent team UUID slips through into
// an agent-friendly "team not found: <id> (your teams: [...])" message.
// resolveTeamRef should already have rejected unknown teams in normal
// flows; this is a defensive net for races (team deleted between resolve
// and create) and for direct UUID inputs.
func (p *TeamProvider) translateTeamFKError(ctx context.Context, sess SessionContext, teamID string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "team_issues_team_id_fkey") ||
		(strings.Contains(lower, "foreign key") && strings.Contains(lower, "team")) ||
		strings.Contains(lower, "23503") {
		return fmt.Errorf("team not found: %s (%s)", teamID, p.candidatesHint(ctx, sess))
	}
	return err
}

// rememberTeamContext writes (team_id, issue_id) into the active chat
// session's metadata so the next turn's prepareRunConfig can hydrate
// cfg.Identity.TeamID / IssueID and the agent can then refer to issues
// by their per-team `#N` number without repeating the team UUID.
//
// The write is best-effort: a failure logs at debug and is dropped, so
// a transient DB hiccup never propagates back to the agent as a tool
// error. We only write for `chat`-type sessions whose metadata is not
// already locked to a team_handoff (those are managed by the resolver).
func (p *TeamProvider) rememberTeamContext(ctx context.Context, sess SessionContext, teamID, issueID string) {
	if p == nil || p.sessionService == nil {
		return
	}
	sessionID := strings.TrimSpace(sess.SessionID)
	teamID = strings.TrimSpace(teamID)
	if sessionID == "" || teamID == "" {
		return
	}
	// Handoff sessions already encode their team / issue in metadata
	// via the resolver. Don't second-guess them.
	if kind := strings.TrimSpace(sess.TriggerKind); kind == "handoff" || kind == "handoff_return" {
		return
	}
	current, err := p.sessionService.Get(ctx, sessionID)
	if err != nil {
		p.logger.Debug(
			"remember team context: session lookup failed",
			slog.String("session_id", sessionID),
			slog.Any("error", err),
		)
		return
	}
	if k, _ := current.Metadata["kind"].(string); k == "team_handoff" {
		return
	}
	meta := map[string]any{}
	for k, v := range current.Metadata {
		meta[k] = v
	}
	changed := false
	if cur, _ := meta["last_team_id"].(string); cur != teamID {
		meta["last_team_id"] = teamID
		changed = true
	}
	if strings.TrimSpace(issueID) != "" {
		if cur, _ := meta["last_issue_id"].(string); cur != issueID {
			meta["last_issue_id"] = issueID
			changed = true
		}
	}
	if !changed {
		return
	}
	if _, err := p.sessionService.UpdateMetadata(ctx, sessionID, meta); err != nil {
		p.logger.Debug(
			"remember team context: update metadata failed",
			slog.String("session_id", sessionID),
			slog.Any("error", err),
		)
	}
}

// resolveHandoffTriggerComment looks up the trigger comment id for a
// handoff. Used by the issue_comment tool to default `parent_comment_id`
// so a bot's reply is always threaded under the @mention that woke it.
// Returns "" on any error (caller treats as "no default").
func (p *TeamProvider) resolveHandoffTriggerComment(ctx context.Context, handoffID string) (string, error) {
	if p == nil || p.service == nil || strings.TrimSpace(handoffID) == "" {
		return "", errors.New("handoff lookup not available")
	}
	ho, err := p.service.Store().GetHandoff(ctx, handoffID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(ho.TriggerCommentID), nil
}

// formatMentionToken builds the ready-to-paste @ token for a team member.
// Names containing whitespace are emitted in the quoted form (`@"Frontend
// Bot"`) so the parser sees them as a single label. Names without spaces
// use the bare form (`@FrontendBot`).
func formatMentionToken(name string) string {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return ""
	}
	if strings.ContainsAny(cleaned, " \t") {
		return `@"` + strings.ReplaceAll(cleaned, `"`, `'`) + `"`
	}
	return "@" + cleaned
}
