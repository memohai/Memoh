package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const skillsDirPath = config.DefaultDataMount + "/.skills"

type SkillItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Raw         string         `json:"raw"`
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

type skillsOpResponse struct {
	OK bool `json:"ok"`
}

// ListSkills godoc
// @Summary List skills from data directory
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
	skills, err := h.loadSkillsFromContainer(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for i := range skills {
		skills[i].Raw = skills[i].Content
	}
	return c.JSON(http.StatusOK, SkillsResponse{Skills: skills})
}

// UpsertSkills godoc
// @Summary Upload skills into data directory
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
		parsed := parseSkillFile(raw, "")
		if !isValidSkillName(parsed.Name) {
			return echo.NewHTTPError(http.StatusBadRequest, "skill must have a valid name in YAML frontmatter")
		}
		dirPath := path.Join(skillsDirPath, parsed.Name)
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
// @Summary Delete skills from data directory
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
		if !isValidSkillName(skillName) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
		}
		_ = client.DeleteFile(ctx, path.Join(skillsDirPath, skillName), true)
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// LoadSkills loads all skills from the container for the given bot.
func (h *ContainerdHandler) LoadSkills(ctx context.Context, botID string) ([]SkillItem, error) {
	return h.loadSkillsFromContainer(ctx, botID)
}

func (h *ContainerdHandler) loadSkillsFromContainer(ctx context.Context, botID string) ([]SkillItem, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}

	entries, err := client.ListDir(ctx, skillsDirPath, false)
	if err != nil {
		return []SkillItem{}, nil
	}

	var skills []SkillItem
	for _, entry := range entries {
		if !entry.GetIsDir() {
			if path.Base(entry.GetPath()) == "SKILL.md" {
				filePath := path.Join(skillsDirPath, "SKILL.md")
				raw, readErr := readContainerSkillFile(ctx, client, filePath)
				if readErr != nil {
					continue
				}
				parsed := parseSkillFile(raw, "default")
				skills = append(skills, skillItemFromParsed(parsed, raw))
			}
			continue
		}
		name := path.Base(entry.GetPath())
		if name == "" || name == "." {
			continue
		}
		filePath := path.Join(skillsDirPath, name, "SKILL.md")
		raw, readErr := readContainerSkillFile(ctx, client, filePath)
		if readErr != nil {
			continue
		}
		parsed := parseSkillFile(raw, name)
		skills = append(skills, skillItemFromParsed(parsed, raw))
	}
	return skills, nil
}

func readContainerSkillFile(ctx context.Context, client *bridge.Client, filePath string) (string, error) {
	resp, err := client.ReadFile(ctx, filePath, 0, 0)
	if err != nil {
		return "", err
	}
	return resp.GetContent(), nil
}

func skillItemFromParsed(parsed parsedSkill, raw string) SkillItem {
	return SkillItem{
		Name:        parsed.Name,
		Description: parsed.Description,
		Content:     parsed.Content,
		Metadata:    parsed.Metadata,
		Raw:         raw,
	}
}

// --- parsing logic (unchanged) ---

type parsedSkill struct {
	Name        string
	Description string
	Content     string
	Metadata    map[string]any
}

// parseSkillFile parses a SKILL.md file with YAML frontmatter delimited by "---".
func parseSkillFile(raw string, fallbackName string) parsedSkill {
	trimmed := strings.TrimSpace(raw)
	result := parsedSkill{
		Name:    strings.TrimSpace(fallbackName),
		Content: trimmed,
	}
	if !strings.HasPrefix(trimmed, "---") {
		return normalizeParsedSkill(result)
	}

	rest := trimmed[3:]
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}
	closingIdx := strings.Index(rest, "\n---")
	if closingIdx < 0 {
		return normalizeParsedSkill(result)
	}

	frontmatterRaw := rest[:closingIdx]
	body := rest[closingIdx+4:]
	body = strings.TrimLeft(body, "\r\n")
	result.Content = body

	var fm struct {
		Name        string         `yaml:"name"`
		Description string         `yaml:"description"`
		Metadata    map[string]any `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &fm); err != nil {
		return normalizeParsedSkill(result)
	}

	if strings.TrimSpace(fm.Name) != "" {
		result.Name = strings.TrimSpace(fm.Name)
	}
	result.Description = strings.TrimSpace(fm.Description)
	result.Metadata = fm.Metadata

	return normalizeParsedSkill(result)
}

func normalizeParsedSkill(skill parsedSkill) parsedSkill {
	if strings.TrimSpace(skill.Name) == "" {
		skill.Name = "default"
	}
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Description = strings.TrimSpace(skill.Description)
	skill.Content = strings.TrimSpace(skill.Content)

	if skill.Description == "" {
		skill.Description = skill.Name
	}
	if skill.Content == "" {
		skill.Content = skill.Description
	}

	return skill
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
