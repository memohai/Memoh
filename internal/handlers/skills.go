package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/bots"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
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
	Editable    bool           `json:"editable"`
	Deletable   bool           `json:"deletable"`
	State       string         `json:"state,omitempty"`
	ShadowedBy  string         `json:"shadowed_by,omitempty"`
}

type SkillsResponse struct {
	Skills []SkillItem `json:"skills"`
}

type SafeSkillsResponse struct {
	Skills []skillset.SafeCatalogItem `json:"skills"`
}

type SkillsUpsertRequest struct {
	Skills []string `json:"skills"`
	// SourcePath is the existing SKILL.md being edited when saving a single skill.
	// Empty means create (or overwrite by frontmatter name under
	// /data/skills/user/personal/<name>/).
	SourcePath string `json:"source_path,omitempty"`
}

type SkillsDeleteRequest struct {
	// SourcePaths are SKILL.md paths reported in the skill list. Deleting by name
	// cannot address registry skills, which are nested by registry and package.
	SourcePaths []string `json:"source_paths"`
}

type SkillsActionRequest struct {
	Action     string `json:"action"`
	TargetPath string `json:"target_path"`
}

type skillsOpResponse struct {
	OK bool `json:"ok"`
}

type PluginInstallationLister interface {
	List(ctx context.Context, botID string) ([]pluginspkg.Installation, error)
}

func (h *ContainerdHandler) SetPluginService(service PluginInstallationLister) {
	h.pluginService = service
}

// ListSkills godoc
// @Summary List skills from the bot workspace
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/skills [get].
func (h *ContainerdHandler) ListSkills(c echo.Context) error {
	botID, err := h.requireBotAccessWithPermission(c, bots.PermissionManage)
	if err != nil {
		return err
	}

	skills, err := h.listSkillsFromContainer(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, SkillsResponse{Skills: skills})
}

// ListSafeSkills godoc
// @Summary List runtime-safe skills for chat-time skill selection
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} SafeSkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/skills/catalog [get].
func (h *ContainerdHandler) ListSafeSkills(c echo.Context) error {
	botID, err := h.requireBotAccessWithPermission(c, bots.PermissionChat)
	if err != nil {
		return err
	}
	catalog, err := h.buildSafeSkillCatalog(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, SafeSkillsResponse{Skills: catalog})
}

// UpsertSkills godoc
// @Summary Upload skills into Memoh-managed directory
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsUpsertRequest true "Skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} apperror.Problem
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} apperror.Problem
// @Failure 503 {object} apperror.Problem
// @Router /bots/{bot_id}/container/skills [post].
func (h *ContainerdHandler) UpsertSkills(c echo.Context) error {
	botID, err := h.requireBotAccessWithPermission(c, bots.PermissionWorkspaceWrite)
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
	sourcePath := strings.TrimSpace(req.SourcePath)
	if sourcePath != "" && len(req.Skills) != 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "source_path requires exactly one skill")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return workspaceUnavailableError(err)
	}

	for i, raw := range req.Skills {
		editPath := ""
		if i == 0 {
			editPath = sourcePath
		}
		plan, planErr := skillset.PlanUpsert(raw, editPath)
		if planErr != nil {
			if errors.Is(planErr, skillset.ErrBuiltinSkillReadOnly) {
				return apperror.New(apperror.CodeSkillBuiltinReadOnly, nil)
			}
			return echo.NewHTTPError(http.StatusBadRequest, "skill must have a valid name in YAML frontmatter")
		}
		dirPath := path.Dir(plan.WritePath)
		if plan.RenameFromDir != "" {
			if _, statErr := client.Stat(ctx, dirPath); statErr == nil {
				return apperror.New(apperror.CodeSkillNameTaken, nil)
			} else if !errors.Is(statErr, bridge.ErrNotFound) {
				return skillSaveHTTPError(fmt.Errorf("inspect renamed skill destination: %w", statErr))
			}
			if err := client.Rename(ctx, plan.RenameFromDir, dirPath); err != nil {
				return skillSaveHTTPError(fmt.Errorf("rename skill dir: %w", err))
			}
			if err := client.WriteFile(ctx, plan.WritePath, []byte(raw)); err != nil {
				rollbackErr := client.Rename(context.WithoutCancel(ctx), dirPath, plan.RenameFromDir)
				if rollbackErr != nil {
					return apperror.Wrap(
						apperror.CodeSkillSaveFailed,
						fmt.Errorf("write renamed skill file: %w; restore original skill directory: %w", err, rollbackErr),
						nil,
					)
				}
				return skillSaveHTTPError(fmt.Errorf("write renamed skill file: %w", err))
			}
			continue
		}
		// A pooled client can pass getGRPCClient and still fail on first use
		// when the workspace just stopped; classify per-op errors so the dial
		// diagnostics never reach the response body.
		if err := client.Mkdir(ctx, dirPath); err != nil {
			return skillSaveHTTPError(fmt.Errorf("mkdir skill dir: %w", err))
		}
		if err := client.WriteFile(ctx, plan.WritePath, []byte(raw)); err != nil {
			return skillSaveHTTPError(fmt.Errorf("write skill file: %w", err))
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

func skillSaveHTTPError(err error) error {
	switch {
	case errors.Is(err, bridge.ErrNotFound),
		errors.Is(err, bridge.ErrBadRequest),
		errors.Is(err, bridge.ErrForbidden):
		return fsHTTPError(err)
	case errors.Is(err, bridge.ErrUnavailable):
		return workspaceUnavailableError(err)
	default:
		return apperror.Wrap(apperror.CodeSkillSaveFailed, err, nil)
	}
}

// DeleteSkills godoc
// @Summary Delete Memoh-managed skills
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsDeleteRequest true "Delete skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} apperror.Problem
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} apperror.Problem
// @Router /bots/{bot_id}/container/skills [delete].
func (h *ContainerdHandler) DeleteSkills(c echo.Context) error {
	botID, err := h.requireBotAccessWithPermission(c, bots.PermissionWorkspaceWrite)
	if err != nil {
		return err
	}

	var req SkillsDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.SourcePaths) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "source_paths is required")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return workspaceUnavailableError(err)
	}

	targets := make([]string, 0, len(req.SourcePaths))
	for _, sourcePath := range req.SourcePaths {
		skillDir, dirErr := skillset.DeletableSkillDirForSourcePath(sourcePath)
		if dirErr != nil {
			if errors.Is(dirErr, skillset.ErrBuiltinSkillReadOnly) {
				return apperror.New(apperror.CodeSkillBuiltinReadOnly, nil)
			}
			return echo.NewHTTPError(http.StatusBadRequest, "only Memoh-managed skills can be deleted")
		}
		targets = append(targets, skillDir)
	}

	for _, skillDir := range targets {
		if _, statErr := client.Stat(ctx, skillDir); statErr != nil {
			return fsHTTPError(statErr)
		}
		if err := client.DeleteFile(ctx, skillDir, true); err != nil {
			return fsHTTPError(err)
		}
		pruneEmptySkillNamespaceDirs(ctx, client, skillDir)
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// pruneEmptySkillNamespaceDirs drops the package and namespace directories left
// behind by the last deleted Skill. Best effort: a concurrent install may refill
// them, and a stale empty directory is harmless to discovery.
func pruneEmptySkillNamespaceDirs(ctx context.Context, client *bridge.Client, skillDir string) {
	for _, dir := range skillset.PrunableSkillNamespaceDirs(skillDir) {
		entries, err := client.ListDirAll(ctx, dir, false)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := client.DeleteFile(ctx, dir, false); err != nil {
			return
		}
	}
}

// ApplySkillAction godoc
// @Summary Apply an action to a discovered or managed skill source
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body SkillsActionRequest true "Skill action payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} apperror.Problem
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} apperror.Problem
// @Router /bots/{bot_id}/container/skills/actions [post].
func (h *ContainerdHandler) ApplySkillAction(c echo.Context) error {
	botID, err := h.requireBotAccessWithPermission(c, bots.PermissionWorkspaceWrite)
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
		return workspaceUnavailableError(err)
	}
	roots, pluginRoots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := skillset.ApplyActionWithPluginRoots(ctx, client, roots, pluginRoots, skillset.ActionRequest{
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
	roots, pluginRoots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	items, err := skillset.LoadEffectiveWithPluginRoots(ctx, client, roots, pluginRoots)
	if err != nil {
		return nil, err
	}
	return skillItemsFromEntries(items), nil
}

func (h *ContainerdHandler) ListSafeSkillCatalog(ctx context.Context, botID string) ([]skillset.SafeCatalogItem, error) {
	return h.buildSafeSkillCatalog(ctx, botID)
}

func (h *ContainerdHandler) buildSafeSkillCatalog(ctx context.Context, botID string) ([]skillset.SafeCatalogItem, error) {
	entries, err := h.listSkillEntriesFromContainer(ctx, botID)
	if err != nil {
		return nil, err
	}
	return skillset.BuildSafeCatalog(entries)
}

func (h *ContainerdHandler) ResolveTextRequestedSkills(ctx context.Context, botID string, names []string) ([]skillset.ResolvedSkill, error) {
	entries, err := h.listSkillEntriesFromContainer(ctx, botID)
	if err != nil {
		return nil, err
	}
	return skillset.ResolveTextRequestedSkills(entries, names, skillset.ResolveLimits{})
}

func (h *ContainerdHandler) listSkillsFromContainer(ctx context.Context, botID string) ([]SkillItem, error) {
	items, err := h.listSkillEntriesFromContainer(ctx, botID)
	if err != nil {
		return nil, err
	}
	return skillItemsFromEntries(items), nil
}

func (h *ContainerdHandler) listSkillEntriesFromContainer(ctx context.Context, botID string) ([]skillset.Entry, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	roots, pluginRoots, err := h.skillDiscoveryRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	items, err := skillset.ListWithPluginRoots(ctx, client, roots, pluginRoots)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (h *ContainerdHandler) skillDiscoveryRoots(ctx context.Context, botID string) ([]string, []string, error) {
	var roots []string
	if h.botService != nil {
		bot, err := h.botService.Get(ctx, botID)
		if err == nil {
			roots = workspace.SkillDiscoveryRootsFromMetadata(bot.Metadata)
			pluginRoots, err := h.pluginSkillRoots(ctx, botID)
			return roots, pluginRoots, err
		}
	}
	if h.manager == nil {
		return nil, nil, nil
	}
	var err error
	roots, err = h.manager.ResolveWorkspaceSkillDiscoveryRoots(ctx, botID)
	if err != nil {
		return nil, nil, err
	}
	pluginRoots, err := h.pluginSkillRoots(ctx, botID)
	return roots, pluginRoots, err
}

func (h *ContainerdHandler) pluginSkillRoots(ctx context.Context, botID string) ([]string, error) {
	if h.pluginService == nil {
		return nil, nil
	}
	installations, err := h.pluginService.List(ctx, botID)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(installations))
	seen := make(map[string]struct{}, len(installations))
	for _, installation := range installations {
		if !installation.Enabled || installation.Status == pluginspkg.StatusUninstalled {
			continue
		}
		root, err := skillset.PluginSkillsDirForID(installation.PluginID)
		if err != nil {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots, nil
}

func skillItemsFromEntries(entries []skillset.Entry) []SkillItem {
	items := make([]SkillItem, len(entries))
	for i, entry := range entries {
		_, builtin := skillset.BuiltinSkillName(entry.SourcePath)
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
			Editable:    !builtin,
			Deletable:   entry.Managed && !builtin,
			State:       entry.State,
			ShadowedBy:  entry.ShadowedBy,
		}
	}
	return items
}
