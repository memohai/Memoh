package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type IAMHandler struct {
	queries dbstore.Queries
	rbac    *rbac.Service
}

func NewIAMHandler(queries dbstore.Queries, rbacService *rbac.Service) *IAMHandler {
	return &IAMHandler{queries: queries, rbac: rbacService}
}

func (h *IAMHandler) Register(e *echo.Echo) {
	g := e.Group("/iam")
	g.GET("/roles", h.ListRoles)
	g.GET("/groups", h.ListGroups)
	g.POST("/groups", h.UpsertGroup)
	g.PUT("/groups/:id", h.UpsertGroup)
	g.DELETE("/groups/:id", h.DeleteGroup)
	g.GET("/groups/:id/members", h.ListGroupMembers)
	g.POST("/groups/:id/members", h.UpsertGroupMember)
	g.DELETE("/groups/:id/members/:user_id", h.DeleteGroupMember)
	g.GET("/sso/providers", h.ListSSOProviders)
	g.POST("/sso/providers", h.UpsertSSOProvider)
	g.PUT("/sso/providers/:id", h.UpsertSSOProvider)
	g.DELETE("/sso/providers/:id", h.DeleteSSOProvider)
	g.GET("/sso/providers/:id/group-mappings", h.ListSSOGroupMappings)
	g.POST("/sso/providers/:id/group-mappings", h.UpsertSSOGroupMapping)
	g.DELETE("/sso/providers/:id/group-mappings/:external_group", h.DeleteSSOGroupMapping)
	g.GET("/bots/:bot_id/principal-roles", h.ListBotPrincipalRoles)
	g.POST("/bots/:bot_id/principal-roles", h.AssignBotPrincipalRole)
	g.DELETE("/principal-roles/:id", h.DeletePrincipalRole)
}

type IAMListRolesResponse struct {
	Items []IAMRoleResponse `json:"items"`
}

type IAMRoleResponse struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Scope     string `json:"scope"`
	IsSystem  bool   `json:"is_system"`
	CreatedAt string `json:"created_at"`
}

type IAMListGroupsResponse struct {
	Items []IAMGroupResponse `json:"items"`
}

type IAMGroupResponse struct {
	ID          string          `json:"id"`
	Key         string          `json:"key"`
	DisplayName string          `json:"display_name"`
	Source      string          `json:"source"`
	ExternalID  string          `json:"external_id,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type IAMGroupRequest struct {
	Key         string          `json:"key"`
	DisplayName string          `json:"display_name"`
	Source      string          `json:"source,omitempty"`
	ExternalID  string          `json:"external_id,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type IAMListGroupMembersResponse struct {
	Items []IAMGroupMemberResponse `json:"items"`
}

type IAMGroupMemberResponse struct {
	UserID      string `json:"user_id"`
	GroupID     string `json:"group_id"`
	Source      string `json:"source"`
	ProviderID  string `json:"provider_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type IAMGroupMemberRequest struct {
	UserID     string `json:"user_id"`
	Source     string `json:"source,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
}

type IAMListSSOProvidersResponse struct {
	Items []IAMSSOProviderResponse `json:"items"`
}

type IAMSSOProviderResponse struct {
	ID                 string          `json:"id"`
	Type               string          `json:"type"`
	Key                string          `json:"key"`
	Name               string          `json:"name"`
	Enabled            bool            `json:"enabled"`
	Config             json.RawMessage `json:"config"`
	AttributeMapping   json.RawMessage `json:"attribute_mapping"`
	JITEnabled         bool            `json:"jit_enabled"`
	EmailLinkingPolicy string          `json:"email_linking_policy"`
	TrustEmail         bool            `json:"trust_email"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type IAMSSOProviderRequest struct {
	Type               string          `json:"type"`
	Key                string          `json:"key"`
	Name               string          `json:"name"`
	Enabled            bool            `json:"enabled"`
	Config             json.RawMessage `json:"config"`
	AttributeMapping   json.RawMessage `json:"attribute_mapping"`
	JITEnabled         *bool           `json:"jit_enabled,omitempty"`
	EmailLinkingPolicy string          `json:"email_linking_policy,omitempty"`
	TrustEmail         bool            `json:"trust_email"`
}

type IAMListSSOGroupMappingsResponse struct {
	Items []IAMSSOGroupMappingResponse `json:"items"`
}

type IAMSSOGroupMappingResponse struct {
	ProviderID       string `json:"provider_id"`
	ExternalGroup    string `json:"external_group"`
	GroupID          string `json:"group_id"`
	GroupKey         string `json:"group_key"`
	GroupDisplayName string `json:"group_display_name"`
	CreatedAt        string `json:"created_at"`
}

type IAMSSOGroupMappingRequest struct {
	ExternalGroup string `json:"external_group"`
	GroupID       string `json:"group_id"`
}

type IAMListPrincipalRolesResponse struct {
	Items []IAMPrincipalRoleResponse `json:"items"`
}

type IAMPrincipalRoleResponse struct {
	ID               string `json:"id"`
	PrincipalType    string `json:"principal_type"`
	PrincipalID      string `json:"principal_id"`
	RoleID           string `json:"role_id"`
	RoleKey          string `json:"role_key"`
	RoleScope        string `json:"role_scope"`
	ResourceType     string `json:"resource_type"`
	ResourceID       string `json:"resource_id,omitempty"`
	Source           string `json:"source"`
	ProviderID       string `json:"provider_id,omitempty"`
	UserUsername     string `json:"user_username,omitempty"`
	UserEmail        string `json:"user_email,omitempty"`
	UserDisplayName  string `json:"user_display_name,omitempty"`
	GroupKey         string `json:"group_key,omitempty"`
	GroupDisplayName string `json:"group_display_name,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type IAMPrincipalRoleRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	RoleKey       string `json:"role_key"`
}

// ListRoles godoc
// @Summary List IAM roles
// @Description List built-in IAM roles
// @Tags iam
// @Security BearerAuth
// @Success 200 {object} IAMListRolesResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/roles [get].
func (h *IAMHandler) ListRoles(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	rows, err := h.queries.ListRoles(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMRoleResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamRoleResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListRolesResponse{Items: items})
}

// ListGroups godoc
// @Summary List IAM groups
// @Description List IAM groups
// @Tags iam
// @Security BearerAuth
// @Success 200 {object} IAMListGroupsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups [get].
func (h *IAMHandler) ListGroups(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	rows, err := h.queries.ListIAMGroups(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMGroupResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamGroupResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListGroupsResponse{Items: items})
}

// UpsertGroup godoc
// @Summary Upsert IAM group
// @Description Create or update an IAM group
// @Tags iam
// @Security BearerAuth
// @Param id path string false "Group ID"
// @Param payload body IAMGroupRequest true "Group payload"
// @Success 200 {object} IAMGroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups [post]
// @Router /iam/groups/{id} [put].
func (h *IAMHandler) UpsertGroup(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	var req IAMGroupRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = string(rbac.SourceManual)
	}
	metadata := req.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	id, err := requestID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	row, err := h.queries.UpsertIAMGroup(c.Request().Context(), sqlc.UpsertIAMGroupParams{
		ID:          id,
		Key:         key,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Source:      source,
		ExternalID:  textValue(req.ExternalID),
		Metadata:    []byte(metadata),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.JSON(http.StatusOK, iamGroupResponse(row))
}

// DeleteGroup godoc
// @Summary Delete IAM group
// @Description Delete an IAM group
// @Tags iam
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups/{id} [delete].
func (h *IAMHandler) DeleteGroup(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	id, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.queries.DeleteIAMGroup(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.NoContent(http.StatusNoContent)
}

// ListGroupMembers godoc
// @Summary List IAM group members
// @Description List members of an IAM group
// @Tags iam
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Success 200 {object} IAMListGroupMembersResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups/{id}/members [get].
func (h *IAMHandler) ListGroupMembers(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	groupID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := h.queries.ListIAMGroupMembers(c.Request().Context(), groupID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMGroupMemberResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamGroupMemberResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListGroupMembersResponse{Items: items})
}

// UpsertGroupMember godoc
// @Summary Add IAM group member
// @Description Add a user to an IAM group
// @Tags iam
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param payload body IAMGroupMemberRequest true "Group member payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups/{id}/members [post].
func (h *IAMHandler) UpsertGroupMember(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	groupID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var req IAMGroupMemberRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	userID, err := db.ParseUUID(req.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = string(rbac.SourceManual)
	}
	row, err := h.queries.UpsertIAMGroupMember(c.Request().Context(), sqlc.UpsertIAMGroupMemberParams{
		UserID:     userID,
		GroupID:    groupID,
		Source:     source,
		ProviderID: db.ParseUUIDOrEmpty(req.ProviderID),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.JSON(http.StatusOK, map[string]string{
		"user_id":  row.UserID.String(),
		"group_id": row.GroupID.String(),
		"source":   row.Source,
	})
}

// DeleteGroupMember godoc
// @Summary Delete IAM group member
// @Description Remove a user from an IAM group
// @Tags iam
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param user_id path string true "User ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/groups/{id}/members/{user_id} [delete].
func (h *IAMHandler) DeleteGroupMember(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	groupID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	userID, err := db.ParseUUID(c.Param("user_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.queries.DeleteIAMGroupMember(c.Request().Context(), sqlc.DeleteIAMGroupMemberParams{UserID: userID, GroupID: groupID}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.NoContent(http.StatusNoContent)
}

// ListSSOProviders godoc
// @Summary List IAM SSO providers
// @Description List all IAM SSO providers for administration
// @Tags iam
// @Security BearerAuth
// @Success 200 {object} IAMListSSOProvidersResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers [get].
func (h *IAMHandler) ListSSOProviders(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	rows, err := h.queries.ListSSOProviders(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMSSOProviderResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamSSOProviderResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListSSOProvidersResponse{Items: items})
}

// UpsertSSOProvider godoc
// @Summary Upsert IAM SSO provider
// @Description Create or update an IAM SSO provider
// @Tags iam
// @Security BearerAuth
// @Param id path string false "SSO provider ID"
// @Param payload body IAMSSOProviderRequest true "SSO provider payload"
// @Success 200 {object} IAMSSOProviderResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers [post]
// @Router /iam/sso/providers/{id} [put].
func (h *IAMHandler) UpsertSSOProvider(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	var req IAMSSOProviderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Type) != "oidc" && strings.TrimSpace(req.Type) != "saml" {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be oidc or saml")
	}
	if strings.TrimSpace(req.Key) == "" || strings.TrimSpace(req.Name) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key and name are required")
	}
	id, err := requestID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	config := req.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	config, err = mergeMaskedSSOConfig(c.Request().Context(), h.queries, id, strings.TrimSpace(req.Type), config)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	attributeMapping := req.AttributeMapping
	if len(attributeMapping) == 0 {
		attributeMapping = json.RawMessage(`{}`)
	}
	jit := true
	if req.JITEnabled != nil {
		jit = *req.JITEnabled
	}
	policy := strings.TrimSpace(req.EmailLinkingPolicy)
	if policy == "" {
		policy = "link_existing"
	}
	row, err := h.queries.UpsertSSOProvider(c.Request().Context(), sqlc.UpsertSSOProviderParams{
		ID:                 id,
		Type:               strings.TrimSpace(req.Type),
		Key:                strings.TrimSpace(req.Key),
		Name:               strings.TrimSpace(req.Name),
		Enabled:            req.Enabled,
		Config:             []byte(config),
		AttributeMapping:   []byte(attributeMapping),
		JitEnabled:         jit,
		EmailLinkingPolicy: policy,
		TrustEmail:         req.TrustEmail,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, iamSSOProviderResponse(row))
}

// DeleteSSOProvider godoc
// @Summary Delete IAM SSO provider
// @Description Delete an IAM SSO provider
// @Tags iam
// @Security BearerAuth
// @Param id path string true "SSO provider ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers/{id} [delete].
func (h *IAMHandler) DeleteSSOProvider(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	id, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.queries.DeleteSSOProvider(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.NoContent(http.StatusNoContent)
}

// ListSSOGroupMappings godoc
// @Summary List SSO group mappings
// @Description List external group to IAM group mappings for an SSO provider
// @Tags iam
// @Security BearerAuth
// @Param id path string true "SSO provider ID"
// @Success 200 {object} IAMListSSOGroupMappingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers/{id}/group-mappings [get].
func (h *IAMHandler) ListSSOGroupMappings(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	providerID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := h.queries.ListSSOGroupMappingsByProvider(c.Request().Context(), providerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMSSOGroupMappingResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamSSOGroupMappingResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListSSOGroupMappingsResponse{Items: items})
}

// UpsertSSOGroupMapping godoc
// @Summary Upsert SSO group mapping
// @Description Create or update an external group to IAM group mapping
// @Tags iam
// @Security BearerAuth
// @Param id path string true "SSO provider ID"
// @Param payload body IAMSSOGroupMappingRequest true "SSO group mapping payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers/{id}/group-mappings [post].
func (h *IAMHandler) UpsertSSOGroupMapping(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	providerID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var req IAMSSOGroupMappingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.ExternalGroup) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "external_group is required")
	}
	groupID, err := db.ParseUUID(req.GroupID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	row, err := h.queries.UpsertSSOGroupMapping(c.Request().Context(), sqlc.UpsertSSOGroupMappingParams{
		ProviderID:    providerID,
		ExternalGroup: strings.TrimSpace(req.ExternalGroup),
		GroupID:       groupID,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.JSON(http.StatusOK, map[string]string{
		"provider_id":    row.ProviderID.String(),
		"external_group": row.ExternalGroup,
		"group_id":       row.GroupID.String(),
	})
}

// DeleteSSOGroupMapping godoc
// @Summary Delete SSO group mapping
// @Description Delete an external group mapping from an SSO provider
// @Tags iam
// @Security BearerAuth
// @Param id path string true "SSO provider ID"
// @Param external_group path string true "External group"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/sso/providers/{id}/group-mappings/{external_group} [delete].
func (h *IAMHandler) DeleteSSOGroupMapping(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	providerID, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	externalGroup, err := url.PathUnescape(c.Param("external_group"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.queries.DeleteSSOGroupMapping(c.Request().Context(), sqlc.DeleteSSOGroupMappingParams{
		ProviderID:    providerID,
		ExternalGroup: strings.TrimSpace(externalGroup),
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.NoContent(http.StatusNoContent)
}

// ListBotPrincipalRoles godoc
// @Summary List bot role assignments
// @Description List user and group role assignments for a bot
// @Tags iam
// @Security BearerAuth
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} IAMListPrincipalRolesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/bots/{bot_id}/principal-roles [get].
func (h *IAMHandler) ListBotPrincipalRoles(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	botID, err := db.ParseUUID(c.Param("bot_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := h.queries.ListPrincipalRoles(c.Request().Context(), sqlc.ListPrincipalRolesParams{
		ResourceType: string(rbac.ResourceBot),
		ResourceID:   botID,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]IAMPrincipalRoleResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, iamPrincipalRoleResponse(row))
	}
	return c.JSON(http.StatusOK, IAMListPrincipalRolesResponse{Items: items})
}

// AssignBotPrincipalRole godoc
// @Summary Assign bot role
// @Description Assign a bot-scoped role to a user or group
// @Tags iam
// @Security BearerAuth
// @Param bot_id path string true "Bot ID"
// @Param payload body IAMPrincipalRoleRequest true "Principal role payload"
// @Success 200 {object} map[string]string
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/bots/{bot_id}/principal-roles [post].
func (h *IAMHandler) AssignBotPrincipalRole(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	botID, err := db.ParseUUID(c.Param("bot_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var req IAMPrincipalRoleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	principalType := strings.TrimSpace(req.PrincipalType)
	if principalType != string(rbac.PrincipalUser) && principalType != string(rbac.PrincipalGroup) {
		return echo.NewHTTPError(http.StatusBadRequest, "principal_type must be user or group")
	}
	principalID, err := db.ParseUUID(req.PrincipalID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	role, err := h.queries.GetRoleByKey(c.Request().Context(), strings.TrimSpace(req.RoleKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusBadRequest, "role not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if role.Scope != string(rbac.ResourceBot) {
		return echo.NewHTTPError(http.StatusBadRequest, "role must be bot-scoped")
	}
	row, err := h.queries.AssignPrincipalRole(c.Request().Context(), sqlc.AssignPrincipalRoleParams{
		PrincipalType: principalType,
		PrincipalID:   principalID,
		RoleID:        role.ID,
		ResourceType:  string(rbac.ResourceBot),
		ResourceID:    botID,
		Source:        string(rbac.SourceManual),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.NoContent(http.StatusNoContent)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.JSON(http.StatusOK, map[string]string{"id": row.ID.String()})
}

// DeletePrincipalRole godoc
// @Summary Delete role assignment
// @Description Delete a user or group role assignment
// @Tags iam
// @Security BearerAuth
// @Param id path string true "Principal role assignment ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /iam/principal-roles/{id} [delete].
func (h *IAMHandler) DeletePrincipalRole(c echo.Context) error {
	if err := h.requireSystemAdmin(c); err != nil {
		return err
	}
	id, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.queries.DeletePrincipalRole(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.rbac.ClearCache()
	return c.NoContent(http.StatusNoContent)
}

func (h *IAMHandler) requireSystemAdmin(c echo.Context) error {
	if h.queries == nil || h.rbac == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "iam service not configured")
	}
	userID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	allowed, err := h.rbac.HasPermission(c.Request().Context(), rbac.Check{
		UserID:        userID,
		PermissionKey: rbac.PermissionSystemAdmin,
		ResourceType:  rbac.ResourceSystem,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !allowed {
		return echo.NewHTTPError(http.StatusForbidden, "system admin required")
	}
	return nil
}

func requestID(raw string) (pgtype.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = uuid.NewString()
	}
	return db.ParseUUID(raw)
}

func textValue(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func nullUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}

func timeString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func iamRoleResponse(row sqlc.IamRole) IAMRoleResponse {
	return IAMRoleResponse{ID: row.ID.String(), Key: row.Key, Scope: row.Scope, IsSystem: row.IsSystem, CreatedAt: timeString(row.CreatedAt)}
}

func iamGroupResponse(row sqlc.IamGroup) IAMGroupResponse {
	return IAMGroupResponse{
		ID:          row.ID.String(),
		Key:         row.Key,
		DisplayName: row.DisplayName,
		Source:      row.Source,
		ExternalID:  db.TextToString(row.ExternalID),
		Metadata:    json.RawMessage(row.Metadata),
		CreatedAt:   timeString(row.CreatedAt),
		UpdatedAt:   timeString(row.UpdatedAt),
	}
}

func iamGroupMemberResponse(row sqlc.ListIAMGroupMembersRow) IAMGroupMemberResponse {
	return IAMGroupMemberResponse{
		UserID:      row.UserID.String(),
		GroupID:     row.GroupID.String(),
		Source:      row.Source,
		ProviderID:  nullUUIDString(row.ProviderID),
		Username:    db.TextToString(row.Username),
		Email:       db.TextToString(row.Email),
		DisplayName: db.TextToString(row.DisplayName),
		CreatedAt:   timeString(row.CreatedAt),
	}
}

func iamSSOProviderResponse(row sqlc.IamSsoProvider) IAMSSOProviderResponse {
	return IAMSSOProviderResponse{
		ID:                 row.ID.String(),
		Type:               row.Type,
		Key:                row.Key,
		Name:               row.Name,
		Enabled:            row.Enabled,
		Config:             maskSSOConfig(row.Type, json.RawMessage(row.Config)),
		AttributeMapping:   json.RawMessage(row.AttributeMapping),
		JITEnabled:         row.JitEnabled,
		EmailLinkingPolicy: row.EmailLinkingPolicy,
		TrustEmail:         row.TrustEmail,
		CreatedAt:          timeString(row.CreatedAt),
		UpdatedAt:          timeString(row.UpdatedAt),
	}
}

func mergeMaskedSSOConfig(ctx context.Context, queries dbstore.Queries, id pgtype.UUID, providerType string, raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := parseObjectJSON(raw, "config")
	if err != nil {
		return nil, err
	}
	if providerType != "oidc" || cfg["client_secret"] != "********" {
		return raw, nil
	}
	row, err := queries.GetSSOProviderByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return raw, nil
		}
		return nil, err
	}
	existing, err := parseObjectJSON(json.RawMessage(row.Config), "config")
	if err != nil {
		return nil, err
	}
	if secret, ok := existing["client_secret"]; ok {
		cfg["client_secret"] = secret
	}
	merged, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(merged), nil
}

func maskSSOConfig(providerType string, raw json.RawMessage) json.RawMessage {
	if providerType != "oidc" {
		return raw
	}
	cfg, err := parseObjectJSON(raw, "config")
	if err != nil {
		return raw
	}
	if _, ok := cfg["client_secret"]; ok {
		cfg["client_secret"] = "********"
	}
	masked, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return json.RawMessage(masked)
}

func parseObjectJSON(raw json.RawMessage, field string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errors.New(field + " must be a JSON object")
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

func iamSSOGroupMappingResponse(row sqlc.ListSSOGroupMappingsByProviderRow) IAMSSOGroupMappingResponse {
	return IAMSSOGroupMappingResponse{
		ProviderID:       row.ProviderID.String(),
		ExternalGroup:    row.ExternalGroup,
		GroupID:          row.GroupID.String(),
		GroupKey:         row.GroupKey,
		GroupDisplayName: row.GroupDisplayName,
		CreatedAt:        timeString(row.CreatedAt),
	}
}

func iamPrincipalRoleResponse(row sqlc.ListPrincipalRolesRow) IAMPrincipalRoleResponse {
	return IAMPrincipalRoleResponse{
		ID:               row.ID.String(),
		PrincipalType:    row.PrincipalType,
		PrincipalID:      row.PrincipalID.String(),
		RoleID:           row.RoleID.String(),
		RoleKey:          row.RoleKey,
		RoleScope:        row.RoleScope,
		ResourceType:     row.ResourceType,
		ResourceID:       nullUUIDString(row.ResourceID),
		Source:           row.Source,
		ProviderID:       nullUUIDString(row.ProviderID),
		UserUsername:     db.TextToString(row.UserUsername),
		UserEmail:        db.TextToString(row.UserEmail),
		UserDisplayName:  db.TextToString(row.UserDisplayName),
		GroupKey:         db.TextToString(row.GroupKey),
		GroupDisplayName: db.TextToString(row.GroupDisplayName),
		CreatedAt:        timeString(row.CreatedAt),
	}
}
