package handlers

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

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
// @Router /bots/{bot_id}/container/skills [get]
func (h *ContainerdHandler) ListSkills(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	skillsDir, err := h.ensureSkillsDirHost(botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	entries, err := listSkillEntries(skillsDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	skills := make([]SkillItem, 0, len(entries))
	for _, entry := range entries {
		skillPath, name := skillPathForEntry(entry)
		if skillPath == "" {
			continue
		}
		raw, err := h.readSkillFile(skillsDir, skillPath)
		if err != nil {
			continue
		}
		parsed := parseSkillFile(raw, name)
		skills = append(skills, SkillItem{
			Name:        parsed.Name,
			Description: parsed.Description,
			Content:     parsed.Content,
			Metadata:    parsed.Metadata,
			Raw:         raw,
		})
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
// @Router /bots/{bot_id}/container/skills [post]
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

	skillsDir, err := h.ensureSkillsDirHost(botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, raw := range req.Skills {
		parsed := parseSkillFile(raw, "")
		if !isValidSkillName(parsed.Name) {
			return echo.NewHTTPError(http.StatusBadRequest, "skill must have a valid name in YAML frontmatter")
		}
		dirPath := filepath.Join(skillsDir, parsed.Name)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		filePath := filepath.Join(dirPath, "SKILL.md")
		if err := os.WriteFile(filePath, []byte(raw), 0o644); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
// @Router /bots/{bot_id}/container/skills [delete]
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

	skillsDir, err := h.ensureSkillsDirHost(botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	for _, name := range req.Names {
		skillName := strings.TrimSpace(name)
		if !isValidSkillName(skillName) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
		}
		deletePath := filepath.Join(skillsDir, skillName)
		if err := os.RemoveAll(deletePath); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// LoadSkills loads all skills from the container for the given bot.
// This implements chat.SkillLoader.
func (h *ContainerdHandler) LoadSkills(ctx context.Context, botID string) ([]SkillItem, error) {
	skillsDir, err := h.ensureSkillsDirHost(botID)
	if err != nil {
		return nil, err
	}

	entries, err := listSkillEntries(skillsDir)
	if err != nil {
		return nil, err
	}

	skills := make([]SkillItem, 0, len(entries))
	for _, entry := range entries {
		skillPath, name := skillPathForEntry(entry)
		if skillPath == "" {
			continue
		}
		raw, err := h.readSkillFile(skillsDir, skillPath)
		if err != nil {
			continue
		}
		parsed := parseSkillFile(raw, name)
		skills = append(skills, SkillItem{
			Name:        parsed.Name,
			Description: parsed.Description,
			Content:     parsed.Content,
			Metadata:    parsed.Metadata,
		})
	}
	return skills, nil
}

func (h *ContainerdHandler) ensureSkillsDirHost(botID string) (string, error) {
	root, err := h.ensureBotDataRoot(botID)
	if err != nil {
		return "", err
	}
	skillsDir := filepath.Join(root, ".skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return "", err
	}
	return skillsDir, nil
}

func (h *ContainerdHandler) readSkillFile(skillsDir, filePath string) (string, error) {
	safeRel := strings.TrimPrefix(strings.TrimPrefix(filePath, ".skills/"), "./.skills/")
	if safeRel == "" {
		return "", os.ErrInvalid
	}
	target := filepath.Join(skillsDir, filepath.FromSlash(safeRel))
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func listSkillEntries(skillsDir string) ([]skillEntry, error) {
	dirEntries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	entries := make([]skillEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		name := entry.Name()
		if name == "" {
			continue
		}
		if entry.IsDir() {
			entries = append(entries, skillEntry{
				Path:  path.Join(".skills", name),
				IsDir: true,
			})
			continue
		}
		if name == "SKILL.md" {
			entries = append(entries, skillEntry{
				Path:  path.Join(".skills", name),
				IsDir: false,
			})
		}
	}
	return entries, nil
}

type skillEntry struct {
	Path  string
	IsDir bool
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

// parsedSkill holds the result of parsing a SKILL.md file with YAML frontmatter.
type parsedSkill struct {
	Name        string
	Description string
	Content     string         // body after frontmatter
	Metadata    map[string]any // "metadata" key from frontmatter
}

// parseSkillFile parses a SKILL.md file with YAML frontmatter delimited by "---".
// Format:
//
//	---
//	name: your-skill-name
//	description: Brief description
//	metadata:
//	  key: value
//	---
//	# Body content ...
func parseSkillFile(raw string, fallbackName string) parsedSkill {
	trimmed := strings.TrimSpace(raw)
	result := parsedSkill{
		Name:    strings.TrimSpace(fallbackName),
		Content: trimmed,
	}
	if !strings.HasPrefix(trimmed, "---") {
		return normalizeParsedSkill(result)
	}

	// Find closing "---".
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
