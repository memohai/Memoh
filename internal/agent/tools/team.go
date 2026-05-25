package tools

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agentteam"
)

// TeamProvider exposes team / issue / handoff tools to agents.
type TeamProvider struct {
	service *agentteam.Service
	logger  *slog.Logger
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
					"team_id": map[string]any{"type": "string", "description": "Team ID. Defaults to the current session team."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				teamID := strings.TrimSpace(StringArg(args, "team_id"))
				if teamID == "" {
					teamID = sess.TeamID
				}
				if teamID == "" {
					return nil, errors.New("team_id is required (no active team in session)")
				}
				members, err := p.service.ListMembers(ctx.Context, teamID)
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
				return map[string]any{"ok": true, "team_id": teamID, "count": len(items), "members": items}, nil
			},
		},
		{
			Name:        "issue_create",
			Description: "Create a new team issue. Use when the user asks for a task that needs multi-step or multi-bot collaboration, persistent tracking, or shared files. Returns the new issue with its id and number.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id":          map[string]any{"type": "string", "description": "Team ID. Defaults to the current session team."},
					"title":            map[string]any{"type": "string", "description": "Short issue title."},
					"description":      map[string]any{"type": "string", "description": "Detailed description / requirements."},
					"status":           map[string]any{"type": "string", "description": "Initial status: backlog, todo (default), in_progress, blocked, review."},
					"assignee_bot_id":  map[string]any{"type": "string", "description": "Optional bot id to assign as the executor."},
					"assignee_user_id": map[string]any{"type": "string", "description": "Optional user id to assign."},
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
				teamID := strings.TrimSpace(StringArg(args, "team_id"))
				if teamID == "" {
					teamID = sess.TeamID
				}
				if teamID == "" {
					return nil, errors.New("team_id is required (no active team in session)")
				}
				title := strings.TrimSpace(StringArg(args, "title"))
				if title == "" {
					return nil, errors.New("title is required")
				}
				status := agentteam.IssueStatus(strings.TrimSpace(StringArg(args, "status")))
				assigneeBot := strings.TrimSpace(StringArg(args, "assignee_bot_id"))
				assigneeUser := strings.TrimSpace(StringArg(args, "assignee_user_id"))
				assigneeType := ""
				switch {
				case assigneeBot != "":
					assigneeType = "bot"
				case assigneeUser != "":
					assigneeType = "user"
				}
				parentRef := strings.TrimSpace(StringArg(args, "parent_issue"))
				if parentRef == "" {
					parentRef = strings.TrimSpace(StringArg(args, "parent_issue_id"))
				}
				parentUUID := ""
				if parentRef != "" {
					parent, perr := p.service.ResolveIssueRef(ctx.Context, teamID, parentRef)
					if perr != nil {
						return nil, errors.New("parent_issue: " + perr.Error())
					}
					parentUUID = parent.ID
				}
				issue, err := p.service.CreateIssue(ctx.Context, agentteam.CreateIssueInput{
					TeamID:         teamID,
					Title:          title,
					Description:    StringArg(args, "description"),
					Status:         status,
					AssigneeType:   assigneeType,
					AssigneeBotID:  assigneeBot,
					AssigneeUserID: assigneeUser,
					CreatedByType:  agentteam.ActorBot,
					CreatedByBotID: sess.BotID,
					ParentIssueID:  parentUUID,
				})
				if err != nil {
					return nil, err
				}
				return issueToMap(issue), nil
			},
		},
		{
			Name:        "issue_list",
			Description: "List issues in a team. Pass open=true to limit the result to non-terminal statuses.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"team_id": map[string]any{"type": "string", "description": "Team ID. Defaults to the current session team."},
					"open":    map[string]any{"type": "boolean", "description": "When true, only list issues that are not done/cancelled."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				teamID := strings.TrimSpace(StringArg(args, "team_id"))
				if teamID == "" {
					teamID = sess.TeamID
				}
				if teamID == "" {
					return nil, errors.New("team_id is required (no active team in session)")
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
					issues, ierr = p.service.ListOpenIssuesByTeam(ctx.Context, teamID)
				} else {
					issues, ierr = p.service.ListIssuesByTeam(ctx.Context, teamID)
				}
				if ierr != nil {
					return nil, ierr
				}
				items := make([]map[string]any, 0, len(issues))
				for _, i := range issues {
					items = append(items, issueToMap(i))
				}
				return map[string]any{"ok": true, "team_id": teamID, "count": len(items), "issues": items}, nil
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
					"assignee_bot_id":  map[string]any{"type": "string", "description": "Bot id to assign. Mutually exclusive with assignee_user_id."},
					"assignee_user_id": map[string]any{"type": "string", "description": "User id to assign. Mutually exclusive with assignee_bot_id."},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				target, err := p.resolveSessionIssue(ctx.Context, sess, args)
				if err != nil {
					return nil, err
				}
				botID := strings.TrimSpace(StringArg(args, "assignee_bot_id"))
				userID := strings.TrimSpace(StringArg(args, "assignee_user_id"))
				assigneeType := ""
				switch {
				case botID != "":
					assigneeType = "bot"
				case userID != "":
					assigneeType = "user"
				}
				updated, err := p.service.AssignIssue(ctx.Context, target.ID, agentteam.AssignIssueInput{
					AssigneeType:   assigneeType,
					AssigneeBotID:  botID,
					AssigneeUserID: userID,
				})
				if err != nil {
					return nil, err
				}
				return issueToMap(updated), nil
			},
		},
	}, nil
}

// resolveSessionIssue picks an Issue from tool arguments and session
// context. It accepts an `issue` arg shaped as a per-team integer
// (3 / "#3") or a UUID. When `issue` is omitted, the active session's
// IssueID (always a UUID) is used.
func (p *TeamProvider) resolveSessionIssue(ctx context.Context, sess SessionContext, args map[string]any) (agentteam.Issue, error) {
	if p == nil || p.service == nil {
		return agentteam.Issue{}, errors.New("team service not configured")
	}
	teamID := strings.TrimSpace(sess.TeamID)
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
		return p.service.ResolveIssueRef(ctx, teamID, v)
	case float64:
		if v != float64(int64(v)) {
			return agentteam.Issue{}, errors.New("issue must be an integer or string")
		}
		return p.service.ResolveIssueRef(ctx, teamID, strconv.FormatInt(int64(v), 10))
	case int:
		return p.service.ResolveIssueRef(ctx, teamID, strconv.Itoa(v))
	case int32:
		return p.service.ResolveIssueRef(ctx, teamID, strconv.FormatInt(int64(v), 10))
	case int64:
		return p.service.ResolveIssueRef(ctx, teamID, strconv.FormatInt(v, 10))
	default:
		return p.service.ResolveIssueRef(ctx, teamID, strings.TrimSpace(StringArg(args, "issue")))
	}
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
