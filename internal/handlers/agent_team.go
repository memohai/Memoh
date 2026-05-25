package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/agentteam"
)

// AgentTeamHandler exposes REST endpoints for Agent Team management.
type AgentTeamHandler struct {
	service *agentteam.Service
	logger  *slog.Logger
}

// NewAgentTeamHandler builds the handler.
func NewAgentTeamHandler(log *slog.Logger, service *agentteam.Service) *AgentTeamHandler {
	return &AgentTeamHandler{
		service: service,
		logger:  log.With(slog.String("handler", "agent_team")),
	}
}

// Register wires the handler into the Echo router.
func (h *AgentTeamHandler) Register(e *echo.Echo) {
	teams := e.Group("/teams")
	teams.POST("", h.CreateTeam)
	teams.GET("", h.ListTeams)
	teams.GET("/:team_id", h.GetTeam)
	teams.PUT("/:team_id", h.UpdateTeam)
	teams.DELETE("/:team_id", h.DeleteTeam)

	teams.POST("/:team_id/members", h.AddMember)
	teams.GET("/:team_id/members", h.ListMembers)
	teams.PUT("/:team_id/members/:member_id", h.UpdateMember)
	teams.DELETE("/:team_id/members/:member_id", h.RemoveMember)

	teams.POST("/:team_id/issues", h.CreateIssue)
	teams.GET("/:team_id/issues", h.ListIssues)
	teams.GET("/:team_id/issues/:issue_id", h.GetIssue)
	teams.PUT("/:team_id/issues/:issue_id", h.UpdateIssue)
	teams.POST("/:team_id/issues/:issue_id/assign", h.AssignIssue)
	teams.DELETE("/:team_id/issues/:issue_id", h.DeleteIssue)

	teams.POST("/:team_id/issues/:issue_id/comments", h.PostComment)
	teams.GET("/:team_id/issues/:issue_id/comments", h.ListComments)

	teams.GET("/:team_id/issues/:issue_id/handoffs", h.ListHandoffs)

	issues := e.Group("/issues")
	issues.GET("", h.ListAllIssues)
}

// ── Response shapes ─────────────────────────────────────────────────────────

// TeamResponse is the JSON wire format of a team.
type TeamResponse struct {
	ID            string          `json:"id"`
	OwnerUserID   string          `json:"owner_user_id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	SharedDirName string          `json:"shared_dir_name"`
	Instructions  string          `json:"instructions"`
	Metadata      json.RawMessage `json:"metadata"`
	ArchivedAt    *time.Time      `json:"archived_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// MemberResponse is the JSON wire format of a team member.
//
// display_name is read-only and is derived from the underlying bot
// (bots.display_name) or user (users.display_name) entity. It is NOT a
// settable field on agent_team_members.
type MemberResponse struct {
	ID           string          `json:"id"`
	TeamID       string          `json:"team_id"`
	MemberType   string          `json:"member_type"`
	BotID        string          `json:"bot_id,omitempty"`
	UserID       string          `json:"user_id,omitempty"`
	Role         string          `json:"role"`
	DisplayName  string          `json:"display_name"`
	Instructions string          `json:"instructions"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// IssueResponse is the JSON wire format of an issue.
type IssueResponse struct {
	ID              string          `json:"id"`
	TeamID          string          `json:"team_id"`
	Number          int32           `json:"number"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Status          string          `json:"status"`
	AssigneeType    string          `json:"assignee_type,omitempty"`
	AssigneeBotID   string          `json:"assignee_bot_id,omitempty"`
	AssigneeUserID  string          `json:"assignee_user_id,omitempty"`
	CreatedByType   string          `json:"created_by_type"`
	CreatedByBotID  string          `json:"created_by_bot_id,omitempty"`
	CreatedByUserID string          `json:"created_by_user_id,omitempty"`
	ParentIssueID   string          `json:"parent_issue_id,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	ClosedAt        *time.Time      `json:"closed_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// CommentResponse is the JSON wire format of an issue comment.
type CommentResponse struct {
	ID              string          `json:"id"`
	IssueID         string          `json:"issue_id"`
	TeamID          string          `json:"team_id"`
	ParentCommentID string          `json:"parent_comment_id,omitempty"`
	AuthorType      string          `json:"author_type"`
	AuthorBotID     string          `json:"author_bot_id,omitempty"`
	AuthorUserID    string          `json:"author_user_id,omitempty"`
	Content         string          `json:"content"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// HandoffResponse is the JSON wire format of an agent handoff.
type HandoffResponse struct {
	ID               string          `json:"id"`
	TeamID           string          `json:"team_id"`
	IssueID          string          `json:"issue_id,omitempty"`
	FromActorType    string          `json:"from_actor_type"`
	FromBotID        string          `json:"from_bot_id,omitempty"`
	FromUserID       string          `json:"from_user_id,omitempty"`
	ToBotID          string          `json:"to_bot_id"`
	TriggerCommentID string          `json:"trigger_comment_id,omitempty"`
	SourceSessionID  string          `json:"source_session_id,omitempty"`
	TargetSessionID  string          `json:"target_session_id,omitempty"`
	ResultCommentID  string          `json:"result_comment_id,omitempty"`
	ReturnHandoffID  string          `json:"return_handoff_id,omitempty"`
	Status           string          `json:"status"`
	FailureReason    string          `json:"failure_reason,omitempty"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

// ── Request shapes ──────────────────────────────────────────────────────────

type CreateTeamRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	SharedDirName string          `json:"shared_dir_name"`
	Instructions  string          `json:"instructions"`
	Metadata      json.RawMessage `json:"metadata"`
}

type UpdateTeamRequest struct {
	Name          *string         `json:"name"`
	Description   *string         `json:"description"`
	SharedDirName *string         `json:"shared_dir_name"`
	Instructions  *string         `json:"instructions"`
	Metadata      json.RawMessage `json:"metadata"`
}

// CreateMemberRequest is the body shape for adding a team member.
// Display name is intentionally absent: the member's name is always
// resolved from the underlying bots.display_name / users.display_name.
type CreateMemberRequest struct {
	MemberType   string          `json:"member_type"`
	BotID        string          `json:"bot_id"`
	UserID       string          `json:"user_id"`
	Role         string          `json:"role"`
	Instructions string          `json:"instructions"`
	Metadata     json.RawMessage `json:"metadata"`
}

type UpdateMemberRequest struct {
	Role         *string         `json:"role"`
	Instructions *string         `json:"instructions"`
	Metadata     json.RawMessage `json:"metadata"`
}

type CreateIssueRequest struct {
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Status         string          `json:"status"`
	AssigneeType   string          `json:"assignee_type"`
	AssigneeBotID  string          `json:"assignee_bot_id"`
	AssigneeUserID string          `json:"assignee_user_id"`
	ParentIssueID  string          `json:"parent_issue_id"`
	Metadata       json.RawMessage `json:"metadata"`
}

type UpdateIssueRequest struct {
	Title          *string         `json:"title"`
	Description    *string         `json:"description"`
	Status         *string         `json:"status"`
	AssigneeType   *string         `json:"assignee_type"`
	AssigneeBotID  *string         `json:"assignee_bot_id"`
	AssigneeUserID *string         `json:"assignee_user_id"`
	Metadata       json.RawMessage `json:"metadata"`
}

type AssignIssueRequest struct {
	AssigneeType   string `json:"assignee_type"`
	AssigneeBotID  string `json:"assignee_bot_id"`
	AssigneeUserID string `json:"assignee_user_id"`
}

type CreateCommentRequest struct {
	ParentCommentID string          `json:"parent_comment_id"`
	Content         string          `json:"content"`
	Metadata        json.RawMessage `json:"metadata"`
}

// ── Handlers ────────────────────────────────────────────────────────────────

// CreateTeam godoc
// @Summary Create a team
// @Tags teams
// @Accept json
// @Produce json
// @Param payload body CreateTeamRequest true "Team payload"
// @Success 201 {object} TeamResponse
// @Failure 400 {object} ErrorResponse
// @Router /teams [post].
func (h *AgentTeamHandler) CreateTeam(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req CreateTeamRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	team, err := h.service.CreateTeam(c.Request().Context(), agentteam.CreateTeamInput{
		OwnerUserID:   userID,
		Name:          req.Name,
		Description:   req.Description,
		SharedDirName: req.SharedDirName,
		Instructions:  req.Instructions,
		Metadata:      []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusCreated, teamToResponse(team))
}

// ListTeams godoc
// @Summary List teams for the current user
// @Tags teams
// @Produce json
// @Success 200 {array} TeamResponse
// @Router /teams [get].
func (h *AgentTeamHandler) ListTeams(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teams, err := h.service.ListTeamsByOwner(c.Request().Context(), userID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]TeamResponse, 0, len(teams))
	for _, t := range teams {
		out = append(out, teamToResponse(t))
	}
	return c.JSON(http.StatusOK, out)
}

// GetTeam godoc
// @Summary Get a team
// @Tags teams
// @Produce json
// @Param team_id path string true "Team ID"
// @Success 200 {object} TeamResponse
// @Failure 404 {object} ErrorResponse
// @Router /teams/{team_id} [get].
func (h *AgentTeamHandler) GetTeam(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	team, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusOK, teamToResponse(team))
}

// UpdateTeam godoc
// @Summary Update a team
// @Tags teams
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param payload body UpdateTeamRequest true "Update payload"
// @Success 200 {object} TeamResponse
// @Router /teams/{team_id} [put].
func (h *AgentTeamHandler) UpdateTeam(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	var req UpdateTeamRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	team, err := h.service.UpdateTeam(c.Request().Context(), teamID, agentteam.UpdateTeamInput{
		Name:          req.Name,
		Description:   req.Description,
		SharedDirName: req.SharedDirName,
		Instructions:  req.Instructions,
		Metadata:      []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusOK, teamToResponse(team))
}

// DeleteTeam godoc
// @Summary Archive a team
// @Tags teams
// @Param team_id path string true "Team ID"
// @Success 204
// @Router /teams/{team_id} [delete].
func (h *AgentTeamHandler) DeleteTeam(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	if _, err := h.service.ArchiveTeam(c.Request().Context(), teamID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AddMember godoc
// @Summary Add a team member
// @Tags teams
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param payload body CreateMemberRequest true "Member payload"
// @Success 201 {object} MemberResponse
// @Router /teams/{team_id}/members [post].
func (h *AgentTeamHandler) AddMember(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	var req CreateMemberRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	member, err := h.service.AddMember(c.Request().Context(), agentteam.CreateMemberInput{
		TeamID:       teamID,
		MemberType:   agentteam.MemberType(strings.ToLower(req.MemberType)),
		BotID:        req.BotID,
		UserID:       req.UserID,
		Role:         req.Role,
		Instructions: req.Instructions,
		Metadata:     []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusCreated, memberToResponse(member))
}

// ListMembers godoc
// @Summary List team members
// @Tags teams
// @Produce json
// @Param team_id path string true "Team ID"
// @Success 200 {array} MemberResponse
// @Router /teams/{team_id}/members [get].
func (h *AgentTeamHandler) ListMembers(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	members, err := h.service.ListMembers(c.Request().Context(), teamID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]MemberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, memberToResponse(m))
	}
	return c.JSON(http.StatusOK, out)
}

// UpdateMember godoc
// @Summary Update a member
// @Tags teams
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param member_id path string true "Member ID"
// @Param payload body UpdateMemberRequest true "Member payload"
// @Success 200 {object} MemberResponse
// @Router /teams/{team_id}/members/{member_id} [put].
func (h *AgentTeamHandler) UpdateMember(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	memberID := strings.TrimSpace(c.Param("member_id"))
	if teamID == "" || memberID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id and member_id are required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	var req UpdateMemberRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	member, err := h.service.UpdateMember(c.Request().Context(), memberID, agentteam.UpdateMemberInput{
		Role:         req.Role,
		Instructions: req.Instructions,
		Metadata:     []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusOK, memberToResponse(member))
}

// RemoveMember godoc
// @Summary Remove a team member
// @Tags teams
// @Param team_id path string true "Team ID"
// @Param member_id path string true "Member ID"
// @Success 204
// @Router /teams/{team_id}/members/{member_id} [delete].
func (h *AgentTeamHandler) RemoveMember(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	memberID := strings.TrimSpace(c.Param("member_id"))
	if teamID == "" || memberID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id and member_id are required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	if err := h.service.RemoveMember(c.Request().Context(), memberID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// CreateIssue godoc
// @Summary Create an issue
// @Tags issues
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param payload body CreateIssueRequest true "Issue payload"
// @Success 201 {object} IssueResponse
// @Router /teams/{team_id}/issues [post].
func (h *AgentTeamHandler) CreateIssue(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	var req CreateIssueRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	status := agentteam.IssueStatus(req.Status)
	if status == "" {
		status = agentteam.StatusTodo
	}
	issue, err := h.service.CreateIssue(c.Request().Context(), agentteam.CreateIssueInput{
		TeamID:          teamID,
		Title:           req.Title,
		Description:     req.Description,
		Status:          status,
		AssigneeType:    req.AssigneeType,
		AssigneeBotID:   req.AssigneeBotID,
		AssigneeUserID:  req.AssigneeUserID,
		CreatedByType:   agentteam.ActorUser,
		CreatedByUserID: userID,
		ParentIssueID:   req.ParentIssueID,
		Metadata:        []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusCreated, issueToResponse(issue))
}

// ListIssues godoc
// @Summary List team issues
// @Tags issues
// @Produce json
// @Param team_id path string true "Team ID"
// @Param status query string false "Only return open issues when set to 'open'"
// @Success 200 {array} IssueResponse
// @Router /teams/{team_id}/issues [get].
func (h *AgentTeamHandler) ListIssues(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	var issues []agentteam.Issue
	if strings.EqualFold(c.QueryParam("status"), "open") {
		issues, err = h.service.ListOpenIssuesByTeam(c.Request().Context(), teamID)
	} else {
		issues, err = h.service.ListIssuesByTeam(c.Request().Context(), teamID)
	}
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]IssueResponse, 0, len(issues))
	for _, i := range issues {
		out = append(out, issueToResponse(i))
	}
	return c.JSON(http.StatusOK, out)
}

// ListAllIssues godoc
// @Summary List all issues across teams for the current user
// @Tags issues
// @Produce json
// @Success 200 {array} IssueResponse
// @Router /issues [get].
func (h *AgentTeamHandler) ListAllIssues(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	issues, err := h.service.ListIssuesForOwner(c.Request().Context(), userID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]IssueResponse, 0, len(issues))
	for _, i := range issues {
		out = append(out, issueToResponse(i))
	}
	return c.JSON(http.StatusOK, out)
}

// GetIssue godoc
// @Summary Get an issue
// @Tags issues
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Success 200 {object} IssueResponse
// @Router /teams/{team_id}/issues/{issue_id} [get].
func (h *AgentTeamHandler) GetIssue(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	issue, err := h.service.GetIssue(c.Request().Context(), issueID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	if issue.TeamID != teamID {
		return echo.NewHTTPError(http.StatusNotFound, "issue not found in team")
	}
	return c.JSON(http.StatusOK, issueToResponse(issue))
}

// UpdateIssue godoc
// @Summary Update an issue
// @Tags issues
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Param payload body UpdateIssueRequest true "Issue payload"
// @Success 200 {object} IssueResponse
// @Router /teams/{team_id}/issues/{issue_id} [put].
func (h *AgentTeamHandler) UpdateIssue(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	var req UpdateIssueRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	input := agentteam.UpdateIssueInput{
		Title:          req.Title,
		Description:    req.Description,
		AssigneeType:   req.AssigneeType,
		AssigneeBotID:  req.AssigneeBotID,
		AssigneeUserID: req.AssigneeUserID,
		Metadata:       []byte(req.Metadata),
	}
	if req.Status != nil {
		status := agentteam.IssueStatus(*req.Status)
		input.Status = &status
	}
	issue, err := h.service.UpdateIssue(c.Request().Context(), issueID, input)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusOK, issueToResponse(issue))
}

// AssignIssue godoc
// @Summary Assign an issue
// @Tags issues
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Param payload body AssignIssueRequest true "Assignment"
// @Success 200 {object} IssueResponse
// @Router /teams/{team_id}/issues/{issue_id}/assign [post].
func (h *AgentTeamHandler) AssignIssue(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	var req AssignIssueRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	issue, err := h.service.AssignIssue(c.Request().Context(), issueID, agentteam.AssignIssueInput{
		AssigneeType:   req.AssigneeType,
		AssigneeBotID:  req.AssigneeBotID,
		AssigneeUserID: req.AssigneeUserID,
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusOK, issueToResponse(issue))
}

// DeleteIssue godoc
// @Summary Delete an issue
// @Tags issues
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Success 204
// @Router /teams/{team_id}/issues/{issue_id} [delete].
func (h *AgentTeamHandler) DeleteIssue(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	if err := h.service.DeleteIssue(c.Request().Context(), issueID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// PostComment godoc
// @Summary Post a comment on an issue
// @Tags issues
// @Accept json
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Param payload body CreateCommentRequest true "Comment payload"
// @Success 201 {object} CommentResponse
// @Router /teams/{team_id}/issues/{issue_id}/comments [post].
func (h *AgentTeamHandler) PostComment(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	var req CreateCommentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	comment, err := h.service.PostComment(c.Request().Context(), agentteam.CreateCommentInput{
		IssueID:         issueID,
		TeamID:          teamID,
		ParentCommentID: req.ParentCommentID,
		AuthorType:      agentteam.ActorUser,
		AuthorUserID:    userID,
		Content:         req.Content,
		Metadata:        []byte(req.Metadata),
	})
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	return c.JSON(http.StatusCreated, commentToResponse(comment))
}

// ListComments godoc
// @Summary List comments on an issue
// @Tags issues
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Success 200 {array} CommentResponse
// @Router /teams/{team_id}/issues/{issue_id}/comments [get].
func (h *AgentTeamHandler) ListComments(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	comments, err := h.service.ListComments(c.Request().Context(), issueID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]CommentResponse, 0, len(comments))
	for _, cmt := range comments {
		out = append(out, commentToResponse(cmt))
	}
	return c.JSON(http.StatusOK, out)
}

// ListHandoffs godoc
// @Summary List handoffs on an issue
// @Tags issues
// @Produce json
// @Param team_id path string true "Team ID"
// @Param issue_id path string true "Issue ID"
// @Success 200 {array} HandoffResponse
// @Router /teams/{team_id}/issues/{issue_id}/handoffs [get].
func (h *AgentTeamHandler) ListHandoffs(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	teamID := teamIDParam(c)
	if teamID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "team_id is required")
	}
	if _, err := h.service.GetTeamForOwner(c.Request().Context(), teamID, userID); err != nil {
		return mapAgentTeamHTTPError(err)
	}
	issueID, err := h.resolveIssueIDParam(c, teamID)
	if err != nil {
		return err
	}
	handoffs, err := h.service.Store().ListHandoffsByIssue(c.Request().Context(), issueID)
	if err != nil {
		return mapAgentTeamHTTPError(err)
	}
	out := make([]HandoffResponse, 0, len(handoffs))
	for _, ho := range handoffs {
		out = append(out, handoffToResponse(ho))
	}
	return c.JSON(http.StatusOK, out)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func (*AgentTeamHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func teamIDParam(c echo.Context) string {
	return strings.TrimSpace(c.Param("team_id"))
}

// resolveIssueIDParam reads the `:issue_id` path param and, when it is
// a per-team integer (`3` or `#3`), translates it into the underlying
// UUID. UUID values pass through verbatim. The team is consulted so we
// only resolve numbers in the right scope.
func (h *AgentTeamHandler) resolveIssueIDParam(c echo.Context, teamID string) (string, error) {
	raw := strings.TrimSpace(c.Param("issue_id"))
	if raw == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "issue_id is required")
	}
	cleaned := strings.TrimPrefix(raw, "#")
	if _, err := strconv.Atoi(cleaned); err != nil {
		return raw, nil
	}
	issue, err := h.service.ResolveIssueRef(c.Request().Context(), teamID, raw)
	if err != nil {
		return "", mapAgentTeamHTTPError(err)
	}
	return issue.ID, nil
}

func mapAgentTeamHTTPError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, agentteam.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if errors.Is(err, agentteam.ErrInvalidInput) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if errors.Is(err, agentteam.ErrAlreadyExists) {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	if errors.Is(err, agentteam.ErrLockHeld) {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	if errors.Is(err, agentteam.ErrForbidden) {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

func metaJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return json.RawMessage(b)
}

func teamToResponse(t agentteam.Team) TeamResponse {
	resp := TeamResponse{
		ID:            t.ID,
		OwnerUserID:   t.OwnerUserID,
		Name:          t.Name,
		Description:   t.Description,
		SharedDirName: t.SharedDirName,
		Instructions:  t.Instructions,
		Metadata:      metaJSON(t.Metadata),
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
	if t.HasArchivedAt {
		archived := t.ArchivedAt
		resp.ArchivedAt = &archived
	}
	return resp
}

func memberToResponse(m agentteam.Member) MemberResponse {
	return MemberResponse{
		ID:           m.ID,
		TeamID:       m.TeamID,
		MemberType:   string(m.MemberType),
		BotID:        m.BotID,
		UserID:       m.UserID,
		Role:         m.Role,
		DisplayName:  m.DisplayName,
		Instructions: m.Instructions,
		Metadata:     metaJSON(m.Metadata),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func issueToResponse(i agentteam.Issue) IssueResponse {
	resp := IssueResponse{
		ID:              i.ID,
		TeamID:          i.TeamID,
		Number:          i.Number,
		Title:           i.Title,
		Description:     i.Description,
		Status:          string(i.Status),
		AssigneeType:    i.AssigneeType,
		AssigneeBotID:   i.AssigneeBotID,
		AssigneeUserID:  i.AssigneeUserID,
		CreatedByType:   string(i.CreatedByType),
		CreatedByBotID:  i.CreatedByBotID,
		CreatedByUserID: i.CreatedByUserID,
		ParentIssueID:   i.ParentIssueID,
		Metadata:        metaJSON(i.Metadata),
		CreatedAt:       i.CreatedAt,
		UpdatedAt:       i.UpdatedAt,
	}
	if i.HasClosedAt {
		c := i.ClosedAt
		resp.ClosedAt = &c
	}
	return resp
}

func commentToResponse(cmt agentteam.Comment) CommentResponse {
	return CommentResponse{
		ID:              cmt.ID,
		IssueID:         cmt.IssueID,
		TeamID:          cmt.TeamID,
		ParentCommentID: cmt.ParentCommentID,
		AuthorType:      string(cmt.AuthorType),
		AuthorBotID:     cmt.AuthorBotID,
		AuthorUserID:    cmt.AuthorUserID,
		Content:         cmt.Content,
		Metadata:        metaJSON(cmt.Metadata),
		CreatedAt:       cmt.CreatedAt,
		UpdatedAt:       cmt.UpdatedAt,
	}
}

func handoffToResponse(h agentteam.Handoff) HandoffResponse {
	resp := HandoffResponse{
		ID:               h.ID,
		TeamID:           h.TeamID,
		IssueID:          h.IssueID,
		FromActorType:    string(h.FromActorType),
		FromBotID:        h.FromBotID,
		FromUserID:       h.FromUserID,
		ToBotID:          h.ToBotID,
		TriggerCommentID: h.TriggerCommentID,
		SourceSessionID:  h.SourceSessionID,
		TargetSessionID:  h.TargetSessionID,
		ResultCommentID:  h.ResultCommentID,
		ReturnHandoffID:  h.ReturnHandoffID,
		Status:           string(h.Status),
		FailureReason:    h.FailureReason,
		Metadata:         metaJSON(h.Metadata),
		CreatedAt:        h.CreatedAt,
		UpdatedAt:        h.UpdatedAt,
	}
	if h.HasCompletedAt {
		c := h.CompletedAt
		resp.CompletedAt = &c
	}
	return resp
}
