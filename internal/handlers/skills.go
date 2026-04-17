package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
)

type SkillItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Raw         string         `json:"raw"`
	SourcePath  string         `json:"source_path,omitempty"`
	SourceRoot  string         `json:"source_root,omitempty"`
	SourceKind  string         `json:"source_kind,omitempty"`
	Managed     bool           `json:"managed,omitempty"`
	State       string         `json:"state,omitempty"`
	ShadowedBy  string         `json:"shadowed_by,omitempty"`
}

type SkillsResponse struct {
	Skills []SkillItem `json:"skills"`
}

type SkillsUpsertRequest struct {
	Skills []string `json:"skills"`
}

type SkillsDeleteRequest struct {
	Names []string `json:"names"`
}

type SkillsActionRequest struct {
	Action     string `json:"action"`
	TargetPath string `json:"target_path"`
}

type skillsOpResponse struct {
	OK bool `json:"ok"`
}

// ListSkills godoc
// @Summary List skills from the bot container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/skills [get].
func (h *ContainerdHandler) ListSkills(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skills, err := h.listSkillsFromContainer(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, SkillsResponse{Skills: skills})
}

// UpsertSkills godoc
// @Summary Upload skills into Memoh-managed directory
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsUpsertRequest true "Skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/skills [post].
func (h *ContainerdHandler) UpsertSkills(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req SkillsUpsertRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.Skills) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "skills is required")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	for _, raw := range req.Skills {
		parsed := skillset.ParseFile(raw, "")
		dirPath, dirErr := skillset.ManagedSkillDirForName(parsed.Name)
		if dirErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "skill must have a valid name in YAML frontmatter")
		}
		if err := client.Mkdir(ctx, dirPath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("mkdir failed: %v", err))
		}
		filePath := path.Join(dirPath, "SKILL.md")
		if err := client.WriteFile(ctx, filePath, []byte(raw)); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("write failed: %v", err))
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// DeleteSkills godoc
// @Summary Delete Memoh-managed skills
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsDeleteRequest true "Delete skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/skills [delete].
func (h *ContainerdHandler) DeleteSkills(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req SkillsDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.Names) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "names is required")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	for _, name := range req.Names {
		skillName := strings.TrimSpace(name)
		managedDir, dirErr := skillset.ManagedSkillDirForName(skillName)
		if dirErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
		}
		if _, statErr := client.Stat(ctx, managedDir); statErr != nil {
			return fsHTTPError(statErr)
		}
		if err := client.DeleteFile(ctx, managedDir, true); err != nil {
			return fsHTTPError(err)
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// ApplySkillAction godoc
// @Summary Apply an action to a discovered or managed skill source
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsActionRequest true "Skill action payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/skills/actions [post].
func (h *ContainerdHandler) ApplySkillAction(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req SkillsActionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}
	roots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := skillset.ApplyAction(ctx, client, roots, skillset.ActionRequest{
		Action:     req.Action,
		TargetPath: req.TargetPath,
	}); err != nil {
		return fsHTTPError(err)
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// LoadSkills loads the effective skills from the container for the given bot.
func (h *ContainerdHandler) LoadSkills(ctx context.Context, botID string) ([]SkillItem, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	roots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	items, err := skillset.LoadEffective(ctx, client, roots)
	if err != nil {
		return nil, err
	}
	return skillItemsFromEntries(items), nil
}

func (h *ContainerdHandler) listSkillsFromContainer(ctx context.Context, botID string) ([]SkillItem, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	roots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	items, err := skillset.List(ctx, client, roots)
	if err != nil {
		return nil, err
	}
	return skillItemsFromEntries(items), nil
}

func (h *ContainerdHandler) skillDiscoveryRoots(ctx context.Context, botID string) ([]string, error) {
	if h.botService != nil {
		bot, err := h.botService.Get(ctx, botID)
		if err == nil {
			return workspace.SkillDiscoveryRootsFromMetadata(bot.Metadata), nil
		}
	}
	if h.manager == nil {
		return nil, nil
	}
	return h.manager.ResolveWorkspaceSkillDiscoveryRoots(ctx, botID)
}

func skillItemsFromEntries(entries []skillset.Entry) []SkillItem {
	items := make([]SkillItem, len(entries))
	for i, entry := range entries {
		items[i] = SkillItem{
			Name:        entry.Name,
			Description: entry.Description,
			Content:     entry.Content,
			Metadata:    entry.Metadata,
			Raw:         entry.Raw,
			SourcePath:  entry.SourcePath,
			SourceRoot:  entry.SourceRoot,
			SourceKind:  entry.SourceKind,
			Managed:     entry.Managed,
			State:       entry.State,
			ShadowedBy:  entry.ShadowedBy,
		}
	}
	return items
}
