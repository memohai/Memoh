package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/config"
	mcptools "github.com/memohai/memoh/internal/mcp"
)

type SkillItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type SkillsResponse struct {
	Skills []SkillItem `json:"skills"`
}

type SkillsUpsertRequest struct {
	Skills []SkillItem `json:"skills"`
}

type SkillsDeleteRequest struct {
	Names []string `json:"names"`
}

type skillsOpResponse struct {
	OK bool `json:"ok"`
}

// ListSkills godoc
// @Summary List skills from container
// @Tags containerd
// @Success 200 {object} SkillsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /container/skills [get]
func (h *ContainerdHandler) ListSkills(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return err
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.ensureSkillsDirHost(botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	listPayload, err := h.callMCPTool(ctx, containerID, "fs.list", map[string]any{
		"path":      ".skills",
		"recursive": false,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	entries, err := extractListEntries(listPayload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	skills := make([]SkillItem, 0, len(entries))
	for _, entry := range entries {
		skillPath, name := skillPathForEntry(entry)
		if skillPath == "" {
			continue
		}
		content, err := h.readSkillFile(ctx, containerID, skillPath)
		if err != nil {
			continue
		}
		skills = append(skills, SkillItem{
			Name:        name,
			Description: skillDescription(content),
			Content:     content,
		})
	}

	return c.JSON(http.StatusOK, SkillsResponse{Skills: skills})
}

// UpsertSkills godoc
// @Summary Upload skills into container
// @Tags containerd
// @Param payload body SkillsUpsertRequest true "Skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /container/skills [post]
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
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return err
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, skill := range req.Skills {
		name := strings.TrimSpace(skill.Name)
		if !isValidSkillName(name) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
		}
		content := strings.TrimSpace(skill.Content)
		if content == "" {
			content = buildSkillContent(name, strings.TrimSpace(skill.Description))
		}
		filePath := path.Join(".skills", name, "SKILL.md")
		if _, err := h.callMCPTool(ctx, containerID, "fs.write", map[string]any{
			"path":    filePath,
			"content": content,
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// DeleteSkills godoc
// @Summary Delete skills from container
// @Tags containerd
// @Param payload body SkillsDeleteRequest true "Delete skills payload"
// @Success 200 {object} skillsOpResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /container/skills [delete]
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
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return err
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	for _, name := range req.Names {
		skillName := strings.TrimSpace(name)
		if !isValidSkillName(skillName) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
		}
		deletePath := path.Join(".skills", skillName)
		if _, err := h.callMCPTool(ctx, containerID, "fs.delete", map[string]any{
			"path": deletePath,
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

func (h *ContainerdHandler) ensureSkillsDirHost(botID string) error {
	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	skillsDir := path.Join(dataRoot, "bots", botID, ".skills")
	return os.MkdirAll(skillsDir, 0o755)
}

func (h *ContainerdHandler) readSkillFile(ctx context.Context, containerID, filePath string) (string, error) {
	payload, err := h.callMCPTool(ctx, containerID, "fs.read", map[string]any{
		"path": filePath,
	})
	if err != nil {
		return "", err
	}
	content, err := extractContentString(payload)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (h *ContainerdHandler) callMCPTool(ctx context.Context, containerID, toolName string, args map[string]any) (map[string]any, error) {
	id := "skills-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	req, err := mcptools.NewToolCallRequest(id, toolName, args)
	if err != nil {
		return nil, err
	}
	payload, err := h.callMCPServer(ctx, containerID, req)
	if err != nil {
		return nil, err
	}
	if err := mcptools.PayloadError(payload); err != nil {
		return nil, err
	}
	if err := mcptools.ResultError(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func extractListEntries(payload map[string]any) ([]skillEntry, error) {
	result, err := mcptools.StructuredContent(payload)
	if err != nil {
		return nil, err
	}
	rawEntries, ok := result["entries"].([]any)
	if !ok {
		return nil, errors.New("invalid list response")
	}
	entries := make([]skillEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		entryMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entryPath, _ := entryMap["path"].(string)
		if entryPath == "" {
			continue
		}
		isDir, _ := entryMap["is_dir"].(bool)
		entries = append(entries, skillEntry{Path: entryPath, IsDir: isDir})
	}
	return entries, nil
}

type skillEntry struct {
	Path  string
	IsDir bool
}

func extractContentString(payload map[string]any) (string, error) {
	result, err := mcptools.StructuredContent(payload)
	if err != nil {
		return "", err
	}
	content, _ := result["content"].(string)
	if content == "" {
		return "", errors.New("empty content")
	}
	return content, nil
}

func skillNameFromPath(rel string) string {
	if rel == "" || rel == "SKILL.md" {
		return "default"
	}
	parent := path.Dir(rel)
	if parent == "." {
		return "default"
	}
	return path.Base(parent)
}

func skillPathForEntry(entry skillEntry) (string, string) {
	rel := strings.TrimPrefix(entry.Path, ".skills/")
	if rel == entry.Path {
		rel = strings.TrimPrefix(entry.Path, "./.skills/")
	}
	if entry.IsDir {
		name := path.Base(rel)
		if name == "." || name == "" {
			return "", ""
		}
		return path.Join(".skills", name, "SKILL.md"), name
	}
	if path.Base(rel) == "SKILL.md" {
		return path.Join(".skills", "SKILL.md"), skillNameFromPath(rel)
	}
	return "", ""
}

func skillDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		return line
	}
	return ""
}

func buildSkillContent(name, description string) string {
	if description == "" {
		return "# " + name
	}
	return "# " + name + "\n\n" + description
}

func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return true
}
