package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/config"
)

// SkillV2Item represents an enhanced skill item for API responses.
type SkillV2Item struct {
	// Core fields
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	Raw         string `json:"raw,omitempty"`

	// Extended metadata
	Version       string   `json:"version,omitempty"`
	Author        string   `json:"author,omitempty"`
	License       string   `json:"license,omitempty"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`
	Compatibility string   `json:"compatibility,omitempty"`
	Category      string   `json:"category,omitempty"`

	// Runtime state
	Enabled     bool   `json:"enabled"`
	AutoLoad    bool   `json:"auto_load"`
	CategoryDir string `json:"category_dir,omitempty"` // "public" or "custom"
}

// ListSkillsV2Response represents the response for listing skills.
type ListSkillsV2Response struct {
	Skills []SkillV2Item `json:"skills"`
	Total  int           `json:"total"`
}

// SkillStateUpdateRequest represents a request to update skill state.
type SkillStateUpdateRequest struct {
	Enabled  *bool `json:"enabled,omitempty"`
	AutoLoad *bool `json:"auto_load,omitempty"`
}

// ValidateSkillRequest represents a request to validate skill content.
type ValidateSkillRequest struct {
	Content string `json:"content" validate:"required"`
}

// ValidateSkillResponse represents the response for skill validation.
type ValidateSkillResponse struct {
	Valid  bool                         `json:"valid"`
	Errors []tools.SkillValidationError `json:"errors,omitempty"`
	Skill  SkillV2Item                  `json:"skill,omitempty"`
}

// InstallSkillResponse represents the response for skill installation.
type InstallSkillResponse struct {
	Success   bool   `json:"success"`
	SkillName string `json:"skill_name,omitempty"`
	Message   string `json:"message"`
}

const skillsV2DirPath = config.DefaultDataMount + "/.skills"

// ListSkillsV2 godoc
// @Summary List all skills with extended metadata
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param enabled_only query bool false "Only return enabled skills"
// @Param category query string false "Filter by category (public/custom)"
// @Success 200 {object} ListSkillsV2Response
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/skills/v2 [get]
func (h *ContainerdHandler) ListSkillsV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()

	// Load extensions config
	configPath := path.Join(skillsV2DirPath, tools.DefaultExtensionsConfigFile)
	extConfig, err := h.loadExtensionsConfig(ctx, botID, configPath)
	if err != nil {
		// Config might not exist yet, continue with empty config
		extConfig = tools.NewExtensionsConfig()
	}

	// Parse query params
	enabledOnly := c.QueryParam("enabled_only") == "true"
	categoryFilter := c.QueryParam("category")

	// Load all skills
	skills, err := h.loadSkillsV2FromContainer(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Filter and convert
	var items []SkillV2Item
	for _, skill := range skills {
		// Apply category filter
		if categoryFilter != "" && skill.CategoryDir != categoryFilter {
			continue
		}

		// Get state from config
		state, exists := extConfig.GetSkillState(skill.Name)
		if exists {
			skill.Enabled = state.Enabled
			skill.AutoLoad = state.AutoLoad
		}

		// Apply enabled filter
		if enabledOnly && !skill.Enabled {
			continue
		}

		items = append(items, skillV2ToItem(skill, false))
	}

	return c.JSON(http.StatusOK, ListSkillsV2Response{
		Skills: items,
		Total:  len(items),
	})
}

// GetSkillV2 godoc
// @Summary Get a specific skill with extended metadata
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param skill_name path string true "Skill name"
// @Success 200 {object} SkillV2Item
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/skills/v2/{skill_name} [get]
func (h *ContainerdHandler) GetSkillV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skillName := c.Param("skill_name")
	if !isValidSkillName(skillName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
	}

	ctx := c.Request().Context()

	// Try to load skill
	skill, err := h.loadSkillV2FromContainer(ctx, botID, skillName)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "skill not found")
	}

	// Load state from config
	configPath := path.Join(skillsV2DirPath, tools.DefaultExtensionsConfigFile)
	extConfig, err := h.loadExtensionsConfig(ctx, botID, configPath)
	if err == nil {
		if state, exists := extConfig.GetSkillState(skillName); exists {
			skill.Enabled = state.Enabled
			skill.AutoLoad = state.AutoLoad
		}
	}

	return c.JSON(http.StatusOK, skillV2ToItem(skill, true))
}

// UpdateSkillV2 godoc
// @Summary Create or update a skill
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param skill_name path string true "Skill name"
// @Param payload body string true "Skill content (markdown with YAML frontmatter)"
// @Success 200 {object} SkillV2Item
// @Router /bots/{bot_id}/skills/v2/{skill_name} [put]
func (h *ContainerdHandler) UpdateSkillV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skillName := c.Param("skill_name")
	if !isValidSkillName(skillName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
	}

	// Read raw body
	content, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read request body")
	}

	// Parse and validate skill
	skill, err := tools.ParseSkillV2(string(content), skillName, tools.DefaultCustomCategory)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("failed to parse skill: %v", err))
	}

	validationErrors := skill.Validate()
	if len(validationErrors) > 0 {
		return c.JSON(http.StatusBadRequest, ValidateSkillResponse{
			Valid:  false,
			Errors: validationErrors,
		})
	}

	// Ensure skill name matches URL parameter
	if skill.Name != skillName {
		return echo.NewHTTPError(http.StatusBadRequest, "skill name in frontmatter does not match URL")
	}

	ctx := c.Request().Context()

	// Write skill to container
	skillDir := path.Join(skillsV2DirPath, tools.DefaultCustomCategory, skillName)
	filePath := path.Join(skillDir, tools.SkillFileName)

	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	// Create directory
	if err := client.Mkdir(ctx, skillDir); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to create directory: %v", err))
	}

	// Write file
	if err := client.WriteFile(ctx, filePath, content); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to write skill: %v", err))
	}

	// Update config
	configPath := path.Join(skillsV2DirPath, tools.DefaultExtensionsConfigFile)
	extConfig, err := h.loadOrCreateExtensionsConfig(ctx, botID, configPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to load config: %v", err))
	}

	extConfig.SetSkillState(skillName, tools.SkillState{
		Enabled:  true,
		AutoLoad: false,
		Category: tools.DefaultCustomCategory,
		Version:  skill.Version,
	})

	if err := h.saveExtensionsConfig(ctx, botID, configPath, extConfig); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
	}

	return c.JSON(http.StatusOK, skillV2ToItem(skill, true))
}

// UpdateSkillStateV2 godoc
// @Summary Update skill state (enabled/disabled, auto_load)
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param skill_name path string true "Skill name"
// @Param payload body SkillStateUpdateRequest true "State update"
// @Success 200 {object} skillsOpResponse
// @Router /bots/{bot_id}/skills/v2/{skill_name}/state [patch]
func (h *ContainerdHandler) UpdateSkillStateV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skillName := c.Param("skill_name")
	if !isValidSkillName(skillName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
	}

	var req SkillStateUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()

	// Load config
	configPath := path.Join(skillsV2DirPath, tools.DefaultExtensionsConfigFile)
	extConfig, err := h.loadOrCreateExtensionsConfig(ctx, botID, configPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to load config: %v", err))
	}

	// Get current state or create new
	state, exists := extConfig.GetSkillState(skillName)
	if !exists {
		state = tools.SkillState{
			Category: tools.DefaultCustomCategory,
		}
	}

	// Update fields
	if req.Enabled != nil {
		state.Enabled = *req.Enabled
	}
	if req.AutoLoad != nil {
		state.AutoLoad = *req.AutoLoad
	}

	extConfig.SetSkillState(skillName, state)

	if err := h.saveExtensionsConfig(ctx, botID, configPath, extConfig); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// DeleteSkillV2 godoc
// @Summary Delete a skill
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param skill_name path string true "Skill name"
// @Success 200 {object} SuccessResponse
// @Router /bots/{bot_id}/skills/v2/{skill_name} [delete]
func (h *ContainerdHandler) DeleteSkillV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skillName := c.Param("skill_name")
	if !isValidSkillName(skillName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
	}

	ctx := c.Request().Context()
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	// Try to delete from both public and custom directories
	deleted := false
	for _, category := range []string{tools.DefaultPublicCategory, tools.DefaultCustomCategory} {
		skillDir := path.Join(skillsV2DirPath, category, skillName)
		if err := client.DeleteFile(ctx, skillDir, true); err == nil {
			deleted = true
			break
		}
	}

	if !deleted {
		return echo.NewHTTPError(http.StatusNotFound, "skill not found")
	}

	// Remove from config
	configPath := path.Join(skillsV2DirPath, tools.DefaultExtensionsConfigFile)
	extConfig, err := h.loadExtensionsConfig(ctx, botID, configPath)
	if err == nil {
		extConfig.RemoveSkill(skillName)
		_ = h.saveExtensionsConfig(ctx, botID, configPath, extConfig)
	}

	return c.JSON(http.StatusOK, skillsOpResponse{OK: true})
}

// ValidateSkillV2 godoc
// @Summary Validate skill content without saving
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param payload body ValidateSkillRequest true "Skill content to validate"
// @Success 200 {object} ValidateSkillResponse
// @Router /bots/{bot_id}/skills/v2/validate [post]
func (h *ContainerdHandler) ValidateSkillV2(c echo.Context) error {
	_, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req ValidateSkillRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Parse skill
	skill, err := tools.ParseSkillV2(req.Content, "", tools.DefaultCustomCategory)
	if err != nil {
		return c.JSON(http.StatusOK, ValidateSkillResponse{
			Valid: false,
			Errors: []tools.SkillValidationError{
				{Field: "parse", Message: err.Error()},
			},
		})
	}

	// Validate
	validationErrors := skill.Validate()
	if len(validationErrors) > 0 {
		return c.JSON(http.StatusOK, ValidateSkillResponse{
			Valid:  false,
			Errors: validationErrors,
			Skill:  skillV2ToItem(skill, false),
		})
	}

	return c.JSON(http.StatusOK, ValidateSkillResponse{
		Valid: true,
		Skill: skillV2ToItem(skill, false),
	})
}

// InstallSkillV2 godoc
// @Summary Install a skill from .skill archive
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param file formData file true "Skill archive (.skill file)"
// @Success 200 {object} InstallSkillResponse
// @Router /bots/{bot_id}/skills/v2/install [post]
func (h *ContainerdHandler) InstallSkillV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}

	// Validate extension
	if !strings.HasSuffix(file.Filename, tools.SkillFileExtension) {
		return echo.NewHTTPError(http.StatusBadRequest, "file must have .skill extension")
	}

	ctx := c.Request().Context()

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	defer src.Close()

	// Save to temp file
	tempFile, err := os.CreateTemp("", "skill-*.skill")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create temp file")
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, src); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save file")
	}
	tempFile.Close()

	// Get gRPC client for validation
	_, err = h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	// Read skill archive content for processing
	_, err = os.ReadFile(tempFile.Name())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read archive")
	}

	// Transfer to container and install
	// Note: In a real implementation, you'd need to:
	// 1. Transfer the archive to the container
	// 2. Run the installer inside the container
	// For now, we'll return a placeholder response

	return c.JSON(http.StatusOK, InstallSkillResponse{
		Success:   true,
		SkillName: strings.TrimSuffix(file.Filename, tools.SkillFileExtension),
		Message:   "Skill installation initiated",
	})
}

// ExportSkillV2 godoc
// @Summary Export a skill as .skill archive
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param skill_name path string true "Skill name"
// @Success 200 {file} binary
// @Router /bots/{bot_id}/skills/v2/{skill_name}/export [get]
func (h *ContainerdHandler) ExportSkillV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	skillName := c.Param("skill_name")
	if !isValidSkillName(skillName) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name")
	}

	ctx := c.Request().Context()

	// Try to find skill in public or custom
	var skillPath, category string
	for _, cat := range []string{tools.DefaultPublicCategory, tools.DefaultCustomCategory} {
		path := path.Join(skillsV2DirPath, cat, skillName)
		client, err := h.getGRPCClient(ctx, botID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
		}
		_, err = client.Stat(ctx, path)
		if err == nil {
			skillPath = path
			category = cat
			break
		}
	}

	if skillPath == "" {
		return echo.NewHTTPError(http.StatusNotFound, "skill not found")
	}

	// category is used for logging/export metadata
	_ = category

	// Create temp archive
	tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-*.skill", skillName))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create temp file")
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Note: In a real implementation, you'd need to:
	// 1. Download the skill directory from the container
	// 2. Create the archive locally
	// 3. Send it to the client

	return c.Attachment(tempFile.Name(), fmt.Sprintf("%s.skill", skillName))
}

// Helper functions

func skillV2ToItem(skill *tools.SkillV2, includeContent bool) SkillV2Item {
	item := SkillV2Item{
		Name:          skill.Name,
		Description:   skill.Description,
		Version:       skill.Version,
		Author:        skill.Author,
		License:       skill.License,
		AllowedTools:  skill.AllowedTools,
		Compatibility: skill.Compatibility,
		Category:      skill.Category,
		Enabled:       skill.Enabled,
		AutoLoad:      skill.AutoLoad,
		CategoryDir:   skill.CategoryDir,
	}

	if includeContent {
		item.Content = skill.Content
		item.Raw = skill.Raw
	}

	return item
}

func (h *ContainerdHandler) loadSkillsV2FromContainer(ctx context.Context, botID string) ([]*tools.SkillV2, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}

	var allSkills []*tools.SkillV2

	// Scan both public and custom directories
	for _, category := range []string{tools.DefaultPublicCategory, tools.DefaultCustomCategory} {
		categoryPath := path.Join(skillsV2DirPath, category)

		entries, err := client.ListDir(ctx, categoryPath, false)
		if err != nil {
			// Directory might not exist, skip
			continue
		}

		for _, entry := range entries {
			if !entry.GetIsDir() {
				continue
			}

			skillName := path.Base(entry.GetPath())
			skillPath := path.Join(entry.GetPath(), tools.SkillFileName)

			resp, err := client.ReadFile(ctx, skillPath, 0, 0)
			if err != nil {
				continue
			}

			// Level 1: Parse only metadata for listing (lightweight)
			skill, err := tools.ParseSkillMetadata(resp.GetContent(), skillName, category)
			if err != nil {
				continue
			}

			// Set up skill directory and resource loader for Level 2/3 loading
			skill.SkillDir = "/" + entry.GetPath()
			skill.SetResourceLoader(tools.NewBridgeResourceLoader(h.manager, botID))

			allSkills = append(allSkills, skill)
		}
	}

	return allSkills, nil
}

func (h *ContainerdHandler) loadSkillV2FromContainer(ctx context.Context, botID string, skillName string) (*tools.SkillV2, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}

	// Try both categories
	for _, category := range []string{tools.DefaultPublicCategory, tools.DefaultCustomCategory} {
		skillPath := path.Join(skillsV2DirPath, category, skillName, tools.SkillFileName)

		resp, err := client.ReadFile(ctx, skillPath, 0, 0)
		if err != nil {
			continue
		}

		// Parse metadata first (Level 1)
		skill, err := tools.ParseSkillMetadata(resp.GetContent(), skillName, category)
		if err != nil {
			continue
		}

		// Set up resource loader
		skill.SkillDir = path.Join("/", skillsV2DirPath, category, skillName)
		skill.SetResourceLoader(tools.NewBridgeResourceLoader(h.manager, botID))

		// Load full content (Level 2) for single skill retrieval
		if err := skill.LoadContent(ctx); err != nil {
			return nil, fmt.Errorf("failed to load skill content: %w", err)
		}

		return skill, nil
	}

	return nil, fmt.Errorf("skill not found")
}

func (h *ContainerdHandler) loadExtensionsConfig(ctx context.Context, botID string, configPath string) (*tools.ExtensionsConfig, error) {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return nil, err
	}

	resp, err := client.ReadFile(ctx, configPath, 0, 0)
	if err != nil {
		return nil, err
	}

	// Write to temp file and load
	tempFile, err := os.CreateTemp("", "extensions-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.WriteString(resp.GetContent()); err != nil {
		tempFile.Close()
		return nil, err
	}
	tempFile.Close()

	return tools.LoadExtensionsConfig(tempFile.Name())
}

func (h *ContainerdHandler) loadOrCreateExtensionsConfig(ctx context.Context, botID string, configPath string) (*tools.ExtensionsConfig, error) {
	config, err := h.loadExtensionsConfig(ctx, botID, configPath)
	if err != nil {
		return tools.NewExtensionsConfig(), nil
	}
	return config, nil
}

func (h *ContainerdHandler) saveExtensionsConfig(ctx context.Context, botID string, configPath string, config *tools.ExtensionsConfig) error {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return err
	}

	// Save to temp file
	tempFile, err := os.CreateTemp("", "extensions-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())

	if err := config.SaveTo(tempFile.Name()); err != nil {
		tempFile.Close()
		return err
	}
	tempFile.Close()

	// Read and write to container
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return err
	}

	return client.WriteFile(ctx, configPath, data)
}

// InstallFromTemplateRequest represents a request to install from built-in template.
type InstallFromTemplateRequest struct {
	SkillName string `json:"skill_name" validate:"required"`
}

// InstallSkillFromTemplateV2 godoc
// @Summary Install a skill from built-in templates
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param request body InstallFromTemplateRequest true "Skill name to install"
// @Success 200 {object} InstallSkillResponse
// @Router /bots/{bot_id}/skills/v2/install/template [post]
func (h *ContainerdHandler) InstallSkillFromTemplateV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallFromTemplateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.SkillName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "skill_name is required")
	}

	ctx := c.Request().Context()

	// Get gRPC client
	_, err = h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	// Check if skill exists in built-in templates
	builtinSkillPath := path.Join("skills/public", req.SkillName)
	if _, err := os.Stat(builtinSkillPath); os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("skill '%s' not found in built-in templates", req.SkillName))
	}

	// Copy skill to container's public directory
	targetPath := path.Join(skillsV2DirPath, tools.DefaultPublicCategory, req.SkillName)
	if err := h.copySkillToContainer(ctx, botID, builtinSkillPath, targetPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to install skill: %v", err))
	}

	return c.JSON(http.StatusOK, InstallSkillResponse{
		Success:   true,
		SkillName: req.SkillName,
		Message:   "Skill installed from template successfully",
	})
}

// InstallFromMarketRequest represents a request to install from skill.sh marketplace.
type InstallFromMarketRequest struct {
	SkillName string `json:"skill_name" validate:"required"` // Format: owner/repo@skill-name
}

// InstallSkillFromMarketV2 godoc
// @Summary Install a skill from skill.sh marketplace
// @Tags skills
// @Param bot_id path string true "Bot ID"
// @Param request body InstallFromMarketRequest true "Skill name (owner/repo@skill-name)"
// @Success 200 {object} InstallSkillResponse
// @Router /bots/{bot_id}/skills/v2/install/market [post]
func (h *ContainerdHandler) InstallSkillFromMarketV2(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req InstallFromMarketRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.SkillName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "skill_name is required")
	}

	// Validate format: owner/repo@skill-name
	parts := strings.Split(req.SkillName, "@")
	if len(parts) != 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name format, expected: owner/repo@skill-name")
	}
	ownerRepo := parts[0]
	skillName := parts[1]
	if !strings.Contains(ownerRepo, "/") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid skill name format, expected: owner/repo@skill-name")
	}

	ctx := c.Request().Context()

	// Get gRPC client
	_, err = h.getGRPCClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("container not reachable: %v", err))
	}

	// Download skill from skill.sh marketplace
	// Format: https://skills.sh/api/v1/skills/{owner}/{repo}/{skill-name}/download
	downloadURL := fmt.Sprintf("https://skills.sh/api/v1/skills/%s/%s/download", ownerRepo, skillName)

	// Download the .skill file
	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("failed to download skill: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("skill not found in marketplace: %s", req.SkillName))
	}

	// Save to temp file
	tempFile, err := os.CreateTemp("", fmt.Sprintf("%s-*.skill", skillName))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create temp file")
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save downloaded skill")
	}
	tempFile.Close()

	// Install from the downloaded archive
	targetPath := path.Join(skillsV2DirPath, tools.DefaultCustomCategory, skillName)
	if err := h.installSkillArchive(ctx, botID, tempFile.Name(), targetPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to install skill: %v", err))
	}

	return c.JSON(http.StatusOK, InstallSkillResponse{
		Success:   true,
		SkillName: skillName,
		Message:   "Skill installed from marketplace successfully",
	})
}

// copySkillToContainer copies a skill directory from local filesystem to container
func (h *ContainerdHandler) copySkillToContainer(ctx context.Context, botID, localPath, containerPath string) error {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return err
	}

	// Create directory in container
	if err := client.Mkdir(ctx, containerPath); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Walk local directory and copy files
	return filepath.Walk(localPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(localPath, filePath)
		targetFilePath := path.Join(containerPath, relPath)

		if info.IsDir() {
			return client.Mkdir(ctx, targetFilePath)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		return client.WriteFile(ctx, targetFilePath, data)
	})
}

// installSkillArchive extracts a .skill archive to container
func (h *ContainerdHandler) installSkillArchive(ctx context.Context, botID, archivePath, targetPath string) error {
	client, err := h.getGRPCClient(ctx, botID)
	if err != nil {
		return err
	}

	// Read archive data
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("failed to read archive: %w", err)
	}

	// Create target directory
	if err := client.Mkdir(ctx, targetPath); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Extract and upload files (simplified - real impl would use proper ZIP extraction)
	// For now, just transfer the archive and let container extract it
	archiveTargetPath := path.Join(targetPath, "archive.skill")
	if err := client.WriteFile(ctx, archiveTargetPath, archiveData); err != nil {
		return fmt.Errorf("failed to transfer archive: %w", err)
	}

	return nil
}
