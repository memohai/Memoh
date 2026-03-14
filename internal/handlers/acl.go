package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/bots"
	identitypkg "github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/channel/identities"
	identitypkg "github.com/memohai/memoh/internal/identity"
)

type ACLHandler struct {
	service         *acl.Service
	botService      *bots.Service
	accountService  *accounts.Service
	identityService *identities.Service
}

func NewACLHandler(service *acl.Service, botService *bots.Service, accountService *accounts.Service, identityService *identities.Service) *ACLHandler {
	return &ACLHandler{
		service:         service,
		botService:      botService,
		accountService:  accountService,
		identityService: identityService,
	}
}

func (h *ACLHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id")
	group.GET("/whitelist", h.ListWhitelist)
	group.PUT("/whitelist", h.UpsertWhitelist)
	group.DELETE("/whitelist/:rule_id", h.DeleteWhitelist)
	group.GET("/blacklist", h.ListBlacklist)
	group.PUT("/blacklist", h.UpsertBlacklist)
	group.DELETE("/blacklist/:rule_id", h.DeleteBlacklist)
	group.GET("/access/users", h.SearchUsers)
	group.GET("/access/channel_identities", h.SearchChannelIdentities)
	group.GET("/access/channel_identities/:channel_identity_id/conversations", h.ListObservedConversationsByChannelIdentity)
}

// ListWhitelist godoc
// @Summary List bot whitelist
// @Description List guest allow rules for chat trigger
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} acl.ListRulesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/whitelist [get].
func (h *ACLHandler) ListWhitelist(c echo.Context) error {
	botID, _, err := h.requireManageAccess(c)
	if err != nil {
		return err
	}
	items, err := h.service.ListWhitelist(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, acl.ListRulesResponse{Items: items})
}

// UpsertWhitelist godoc
// @Summary Upsert bot whitelist entry
// @Description Add a guest allow rule for chat trigger
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param payload body acl.UpsertRuleRequest true "Whitelist payload"
// @Success 200 {object} acl.Rule
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/whitelist [put].
func (h *ACLHandler) UpsertWhitelist(c echo.Context) error {
	botID, actorID, err := h.requireManageAccess(c)
	if err != nil {
		return err
	}
	var req acl.UpsertRuleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	item, err := h.service.AddWhitelistEntry(c.Request().Context(), botID, actorID, req)
	if err != nil {
		if errors.Is(err, acl.ErrInvalidRuleSubject) || errors.Is(err, acl.ErrInvalidSourceScope) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteWhitelist godoc
// @Summary Delete bot whitelist entry
// @Description Delete a guest allow rule by rule ID
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param rule_id path string true "Rule ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/whitelist/{rule_id} [delete].
func (h *ACLHandler) DeleteWhitelist(c echo.Context) error {
	if _, _, err := h.requireManageAccess(c); err != nil {
		return err
	}
	ruleID := strings.TrimSpace(c.Param("rule_id"))
	if ruleID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "rule id is required")
	}
	if err := h.service.DeleteRule(c.Request().Context(), ruleID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ListBlacklist godoc
// @Summary List bot blacklist
// @Description List guest deny rules for chat trigger
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} acl.ListRulesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/blacklist [get].
func (h *ACLHandler) ListBlacklist(c echo.Context) error {
	botID, _, err := h.requireManageAccess(c)
	if err != nil {
		return err
	}
	items, err := h.service.ListBlacklist(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, acl.ListRulesResponse{Items: items})
}

// UpsertBlacklist godoc
// @Summary Upsert bot blacklist entry
// @Description Add a guest deny rule for chat trigger
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param payload body acl.UpsertRuleRequest true "Blacklist payload"
// @Success 200 {object} acl.Rule
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/blacklist [put].
func (h *ACLHandler) UpsertBlacklist(c echo.Context) error {
	botID, actorID, err := h.requireManageAccess(c)
	if err != nil {
		return err
	}
	var req acl.UpsertRuleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	item, err := h.service.AddBlacklistEntry(c.Request().Context(), botID, actorID, req)
	if err != nil {
		if errors.Is(err, acl.ErrInvalidRuleSubject) || errors.Is(err, acl.ErrInvalidSourceScope) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteBlacklist godoc
// @Summary Delete bot blacklist entry
// @Description Delete a guest deny rule by rule ID
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param rule_id path string true "Rule ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/blacklist/{rule_id} [delete].
func (h *ACLHandler) DeleteBlacklist(c echo.Context) error {
	if _, _, err := h.requireManageAccess(c); err != nil {
		return err
	}
	ruleID := strings.TrimSpace(c.Param("rule_id"))
	if ruleID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "rule id is required")
	}
	if err := h.service.DeleteRule(c.Request().Context(), ruleID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// SearchUsers godoc
// @Summary Search access users
// @Description Search user candidates for bot access control
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param q query string false "Search query"
// @Param limit query int false "Max results"
// @Success 200 {object} acl.UserCandidateListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/access/users [get].
func (h *ACLHandler) SearchUsers(c echo.Context) error {
	if _, _, err := h.requireManageAccess(c); err != nil {
		return err
	}
	items, err := h.accountService.SearchAccounts(c.Request().Context(), strings.TrimSpace(c.QueryParam("q")), parseLimit(c.QueryParam("limit")))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	result := make([]acl.UserCandidate, 0, len(items))
	for _, item := range items {
		result = append(result, acl.UserCandidate{
			ID:          item.ID,
			Username:    item.Username,
			DisplayName: item.DisplayName,
			AvatarURL:   item.AvatarURL,
			Email:       item.Email,
		})
	}
	return c.JSON(http.StatusOK, acl.UserCandidateListResponse{Items: result})
}

// SearchChannelIdentities godoc
// @Summary Search access channel identities
// @Description Search locally observed channel identity candidates for bot access control
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param q query string false "Search query"
// @Param limit query int false "Max results"
// @Success 200 {object} acl.ChannelIdentityCandidateListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/access/channel_identities [get].
func (h *ACLHandler) SearchChannelIdentities(c echo.Context) error {
	if _, _, err := h.requireManageAccess(c); err != nil {
		return err
	}
	items, err := h.identityService.Search(c.Request().Context(), strings.TrimSpace(c.QueryParam("q")), parseLimit(c.QueryParam("limit")))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	result := make([]acl.ChannelIdentityCandidate, 0, len(items))
	for _, item := range items {
		result = append(result, acl.ChannelIdentityCandidate{
			ID:                item.ID,
			UserID:            item.UserID,
			Channel:           item.Channel,
			ChannelSubjectID:  item.ChannelSubjectID,
			DisplayName:       item.DisplayName,
			AvatarURL:         item.AvatarURL,
			LinkedUsername:    item.LinkedUsername,
			LinkedDisplayName: item.LinkedDisplayName,
			LinkedAvatarURL:   item.LinkedAvatarURL,
		})
	}
	return c.JSON(http.StatusOK, acl.ChannelIdentityCandidateListResponse{Items: result})
}

// ListObservedConversationsByChannelIdentity godoc
// @Summary List observed conversations for a channel identity
// @Description List previously observed conversation candidates for a channel identity under a bot
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Param channel_identity_id path string true "Channel Identity ID"
// @Success 200 {object} acl.ObservedConversationCandidateListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/access/channel_identities/{channel_identity_id}/conversations [get].
func (h *ACLHandler) ListObservedConversationsByChannelIdentity(c echo.Context) error {
	botID, _, err := h.requireManageAccess(c)
	if err != nil {
		return err
	}
	channelIdentityID := strings.TrimSpace(c.Param("channel_identity_id"))
	if err := identitypkg.ValidateChannelIdentityID(channelIdentityID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	items, err := h.service.ListObservedConversationsByChannelIdentity(c.Request().Context(), botID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, acl.ObservedConversationCandidateListResponse{Items: items})
}

func (h *ACLHandler) requireManageAccess(c echo.Context) (string, string, error) {
	actorID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, actorID, botID); err != nil {
		return "", "", err
	}
	return botID, actorID, nil
}

func parseLimit(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}
