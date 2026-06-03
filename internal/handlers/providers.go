package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	"github.com/memohai/memoh/internal/providers"
)

type ProvidersHandler struct {
	service       *providers.Service
	modelsService *models.Service
	logger        *slog.Logger
}

func NewProvidersHandler(log *slog.Logger, service *providers.Service, modelsService *models.Service) *ProvidersHandler {
	return &ProvidersHandler{
		service:       service,
		modelsService: modelsService,
		logger:        log.With(slog.String("handler", "providers")),
	}
}

func (h *ProvidersHandler) Register(e *echo.Echo) {
	group := e.Group("/providers")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.GET("/:id/models", h.ListModelsByProvider)
	group.GET("/name/:name", h.GetByName)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
	group.GET("/count", h.Count)
	group.POST("/:id/test", h.Test)
	group.POST("/:id/import-models", h.ImportModels)
}

// Create godoc
// @Summary Create a new LLM provider
// @Description Create a new LLM provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param request body providers.CreateRequest true "Provider configuration"
// @Success 201 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers [post].
func (h *ProvidersHandler) Create(c echo.Context) error {
	var req providers.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate required fields
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	resp, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List all LLM providers
// @Description Get a list of all configured LLM providers
// @Tags providers
// @Accept json
// @Produce json
// @Success 200 {array} providers.GetResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers [get].
func (h *ProvidersHandler) List(c echo.Context) error {
	resp, err := h.service.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Get godoc
// @Summary Get provider by ID
// @Description Get a provider configuration by its ID
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [get].
func (h *ProvidersHandler) Get(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	resp, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// ListModelsByProvider godoc
// @Summary List provider models
// @Description Get models for a provider by id, optionally filtered by type
// @Tags providers
// @Param id path string true "Provider ID (UUID)"
// @Param type query string false "Model type (chat, embedding)"
// @Success 200 {array} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/models [get].
func (h *ProvidersHandler) ListModelsByProvider(c echo.Context) error {
	if h.modelsService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "models service not configured")
	}
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	modelType := strings.TrimSpace(c.QueryParam("type"))
	var (
		resp []models.GetResponse
		err  error
	)
	if modelType == "" {
		resp, err = h.modelsService.ListByProviderID(c.Request().Context(), id)
	} else {
		resp, err = h.modelsService.ListByProviderIDAndType(c.Request().Context(), id, models.ModelType(modelType))
	}
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetByName godoc
// @Summary Get provider by name
// @Description Get a provider configuration by its name
// @Tags providers
// @Accept json
// @Produce json
// @Param name path string true "Provider name"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/name/{name} [get].
func (h *ProvidersHandler) GetByName(c echo.Context) error {
	name := c.Param("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	resp, err := h.service.GetByName(c.Request().Context(), name)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update provider
// @Description Update an existing provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Param request body providers.UpdateRequest true "Updated provider configuration"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [put].
func (h *ProvidersHandler) Update(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	var req providers.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	resp, err := h.service.Update(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete provider
// @Description Delete a provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [delete].
func (h *ProvidersHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// Count godoc
// @Summary Count providers
// @Description Get the total count of providers
// @Tags providers
// @Accept json
// @Produce json
// @Success 200 {object} providers.CountResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/count [get].
func (h *ProvidersHandler) Count(c echo.Context) error {
	count, err := h.service.Count(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, providers.CountResponse{Count: count})
}

// Test godoc
// @Summary Test provider connectivity
// @Description Probe a provider's base URL to check reachability, supported client types, and embedding support
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.TestResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/test [post].
func (h *ProvidersHandler) Test(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	ctx := c.Request().Context()
	if userID, err := auth.UserIDFromContext(c); err == nil {
		ctx = oauthctx.WithUserID(ctx, userID)
	}

	resp, err := h.service.Test(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// ImportModels godoc
// @Summary Import models from provider
// @Description Fetch models from provider and import them
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.ImportModelsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/import-models [post].
func (h *ProvidersHandler) ImportModels(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	ctx := c.Request().Context()
	if userID, err := auth.UserIDFromContext(c); err == nil {
		ctx = oauthctx.WithUserID(ctx, userID)
	}

	provider, err := h.service.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("provider not found: %v", err))
	}
	if !models.IsLLMClientType(models.ClientType(provider.ClientType)) {
		return echo.NewHTTPError(http.StatusBadRequest, "import models is not supported for speech providers")
	}

	remoteModels, err := h.service.FetchRemoteModels(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("fetch remote models: %v", err))
	}

	resp := providers.ImportModelsResponse{
		Models: make([]string, 0),
	}

	for _, m := range remoteModels {
		modelType := models.ModelTypeChat
		if strings.TrimSpace(m.Type) == string(models.ModelTypeEmbedding) {
			modelType = models.ModelTypeEmbedding
		}
		compatibilities := m.Compatibilities
		if len(compatibilities) == 0 && modelType == models.ModelTypeChat {
			// No capability info at all (no upstream claim, no registry match):
			// fall back to a permissive default, but respect an explicit
			// "no reasoning" discovery so we don't advertise thinking falsely.
			compatibilities = []string{models.CompatVision, models.CompatToolCall}
			if m.ThinkingMode != models.ThinkingModeNone {
				compatibilities = append(compatibilities, models.CompatReasoning)
			}
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = m.ID
		}
		cfg := models.ModelConfig{
			Compatibilities:  compatibilities,
			ReasoningEfforts: m.ReasoningEfforts,
			ThinkingMode:     m.ThinkingMode,
			ContextWindow:    m.ContextWindow,
			Dimensions:       m.Dimensions,
		}
		_, err := h.modelsService.Create(ctx, models.AddRequest{
			ModelID:    m.ID,
			Name:       name,
			ProviderID: id,
			Type:       modelType,
			Config:     cfg,
		})
		if err != nil {
			if errors.Is(err, models.ErrModelIDAlreadyExists) {
				// Upsert/assert: re-importing fills in newly discovered
				// capabilities on existing models without clobbering user config.
				if h.fillExistingModel(ctx, id, m.ID, cfg) {
					resp.Updated++
				} else {
					resp.Skipped++
				}
				continue
			}
			h.logger.Warn("failed to import model", slog.String("model_id", m.ID), slog.Any("error", err))
			continue
		}

		resp.Created++
		resp.Models = append(resp.Models, m.ID)
	}

	return c.JSON(http.StatusOK, resp)
}

// fillExistingModel refreshes an existing model's capability-discovery fields
// from the latest trusted discovery (upstream + litellm registry) and adds
// missing compatibility tokens. Re-import is a refresh: newer discovery
// overwrites stale capability values (thinking_mode / effort tiers / context
// window) rather than only filling blanks, so an old, less-accurate import
// (e.g. an effort list missing xhigh/max) cannot survive. Returns true if the
// model was changed and persisted.
//
// The lookup is provider-scoped because model_id is only unique per provider;
// same-named models under other providers must not affect this refresh.
func (h *ProvidersHandler) fillExistingModel(ctx context.Context, providerID, modelID string, discovered models.ModelConfig) bool {
	existing, err := h.modelsService.GetByProviderAndModelID(ctx, providerID, modelID)
	if err != nil {
		return false
	}
	merged, changed := mergeDiscoveredConfig(existing.Config, discovered)
	if !changed {
		return false
	}
	if _, err := h.modelsService.UpdateByProviderAndModelID(ctx, providerID, modelID, models.UpdateRequest{
		ModelID:    existing.ModelID,
		Name:       existing.Name,
		ProviderID: existing.ProviderID,
		Type:       existing.Type,
		Config:     merged,
	}); err != nil {
		h.logger.Warn("failed to fill model capabilities", slog.String("model_id", modelID), slog.Any("error", err))
		return false
	}
	return true
}

func mergeDiscoveredConfig(existing, discovered models.ModelConfig) (models.ModelConfig, bool) {
	out := existing
	changed := false
	// Capability-discovery fields: a present discovery wins. The fetch layer
	// (applyCapabilities) has already let an explicit upstream claim take
	// precedence over the registry, so whatever arrives here is the freshest
	// trusted value and should replace the stored one. We only skip when the
	// discovery is empty (nothing learned this round → keep what we have).
	if discovered.ThinkingMode != "" && discovered.ThinkingMode != out.ThinkingMode {
		out.ThinkingMode = discovered.ThinkingMode
		changed = true
	}
	if len(discovered.ReasoningEfforts) > 0 && !slices.Equal(discovered.ReasoningEfforts, out.ReasoningEfforts) {
		out.ReasoningEfforts = append([]string(nil), discovered.ReasoningEfforts...)
		changed = true
	}
	if discovered.ContextWindow != nil && (out.ContextWindow == nil || *discovered.ContextWindow != *out.ContextWindow) {
		out.ContextWindow = discovered.ContextWindow
		changed = true
	}
	// Compatibilities are additive: keep anything already present and add the
	// newly discovered tokens.
	for _, c := range discovered.Compatibilities {
		if !slices.Contains(out.Compatibilities, c) {
			out.Compatibilities = append(out.Compatibilities, c)
			changed = true
		}
	}
	return out, changed
}
