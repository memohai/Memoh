package handlers

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

type ModelsHandler struct {
	service         *models.Service
	settingsService *settings.Service
	logger          *slog.Logger
}

func NewModelsHandler(log *slog.Logger, service *models.Service, settingsService *settings.Service) *ModelsHandler {
	return &ModelsHandler{
		service:         service,
		settingsService: settingsService,
		logger:          log.With(slog.String("handler", "models")),
	}
}

func (h *ModelsHandler) Register(e *echo.Echo) {
	group := e.Group("/models")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.GetByID)
	group.GET("/model/:modelId", h.GetByModelID)
	group.POST("/enable", h.Enable)
	group.PUT("/:id", h.UpdateByID)
	group.PUT("/model/:modelId", h.UpdateByModelID)
	group.DELETE("/:id", h.DeleteByID)
	group.DELETE("/model/:modelId", h.DeleteByModelID)
	group.GET("/count", h.Count)
}

// Create godoc
// @Summary Create a new model
// @Description Create a new model configuration
// @Tags models
// @Param payload body models.AddRequest true "Model configuration"
// @Success 201 {object} models.AddResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models [post]
func (h *ModelsHandler) Create(c echo.Context) error {
	var req models.AddRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	resp, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List all models
// @Description Get a list of all configured models, optionally filtered by type or client type
// @Tags models
// @Param type query string false "Model type (chat, embedding)"
// @Param client_type query string false "Client type (openai, anthropic, google)"
// @Success 200 {array} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models [get]
func (h *ModelsHandler) List(c echo.Context) error {
	modelType := c.QueryParam("type")
	clientType := c.QueryParam("client_type")

	var resp []models.GetResponse
	var err error

	if modelType != "" {
		resp, err = h.service.ListByType(c.Request().Context(), models.ModelType(modelType))
	} else if clientType != "" {
		resp, err = h.service.ListByClientType(c.Request().Context(), models.ClientType(clientType))
	} else {
		resp, err = h.service.List(c.Request().Context())
	}

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetByID godoc
// @Summary Get model by internal ID
// @Description Get a model configuration by its internal UUID
// @Tags models
// @Param id path string true "Model internal ID (UUID)"
// @Success 200 {object} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/{id} [get]
func (h *ModelsHandler) GetByID(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	resp, err := h.service.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetByModelID godoc
// @Summary Get model by model ID
// @Description Get a model configuration by its model_id field (e.g., gpt-4)
// @Tags models
// @Param modelId path string true "Model ID (e.g., gpt-4)"
// @Success 200 {object} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/model/{modelId} [get]
func (h *ModelsHandler) GetByModelID(c echo.Context) error {
	modelID := c.Param("modelId")
	if modelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "modelId is required")
	}
	if decoded, err := url.PathUnescape(modelID); err == nil {
		modelID = decoded
	} else {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid modelId")
	}

	resp, err := h.service.GetByModelID(c.Request().Context(), modelID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

type EnableModelRequest struct {
	As      string `json:"as"`
	ModelID string `json:"model_id"`
}

// Enable godoc
// @Summary Enable model for chat/memory/embedding
// @Description Update the current user's settings to use the selected model
// @Tags models
// @Param payload body handlers.EnableModelRequest true "Enable model payload"
// @Success 200 {object} settings.Settings
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/enable [post]
func (h *ModelsHandler) Enable(c echo.Context) error {
	if h.settingsService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "settings service not configured")
	}
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return err
	}
	var req EnableModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.As = strings.ToLower(strings.TrimSpace(req.As))
	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.As == "" || req.ModelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "as and model_id are required")
	}
	if req.As != "chat" && req.As != "memory" && req.As != "embedding" {
		return echo.NewHTTPError(http.StatusBadRequest, "as must be one of chat, memory, embedding")
	}
	model, err := h.service.GetByModelID(c.Request().Context(), req.ModelID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if req.As == "embedding" && model.Type != models.ModelTypeEmbedding {
		return echo.NewHTTPError(http.StatusBadRequest, "model is not an embedding model")
	}
	if (req.As == "chat" || req.As == "memory") && model.Type != models.ModelTypeChat {
		return echo.NewHTTPError(http.StatusBadRequest, "model is not a chat model")
	}
	upsert := settings.UpsertRequest{}
	switch req.As {
	case "chat":
		upsert.ChatModelID = req.ModelID
	case "memory":
		upsert.MemoryModelID = req.ModelID
	case "embedding":
		upsert.EmbeddingModelID = req.ModelID
	}
	resp, err := h.settingsService.Upsert(c.Request().Context(), userID, upsert)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateByID godoc
// @Summary Update model by internal ID
// @Description Update a model configuration by its internal UUID
// @Tags models
// @Param id path string true "Model internal ID (UUID)"
// @Param payload body models.UpdateRequest true "Updated model configuration"
// @Success 200 {object} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/{id} [put]
func (h *ModelsHandler) UpdateByID(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	var req models.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	resp, err := h.service.UpdateByID(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateByModelID godoc
// @Summary Update model by model ID
// @Description Update a model configuration by its model_id field (e.g., gpt-4)
// @Tags models
// @Param modelId path string true "Model ID (e.g., gpt-4)"
// @Param payload body models.UpdateRequest true "Updated model configuration"
// @Success 200 {object} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/model/{modelId} [put]
func (h *ModelsHandler) UpdateByModelID(c echo.Context) error {
	modelID := c.Param("modelId")
	if modelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "modelId is required")
	}
	if decoded, err := url.PathUnescape(modelID); err == nil {
		modelID = decoded
	} else {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid modelId")
	}

	var req models.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	resp, err := h.service.UpdateByModelID(c.Request().Context(), modelID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteByID godoc
// @Summary Delete model by internal ID
// @Description Delete a model configuration by its internal UUID
// @Tags models
// @Param id path string true "Model internal ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/{id} [delete]
func (h *ModelsHandler) DeleteByID(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	if err := h.service.DeleteByID(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteByModelID godoc
// @Summary Delete model by model ID
// @Description Delete a model configuration by its model_id field (e.g., gpt-4)
// @Tags models
// @Param modelId path string true "Model ID (e.g., gpt-4)"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/model/{modelId} [delete]
func (h *ModelsHandler) DeleteByModelID(c echo.Context) error {
	modelID := c.Param("modelId")
	if modelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "modelId is required")
	}
	if decoded, err := url.PathUnescape(modelID); err == nil {
		modelID = decoded
	} else {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid modelId")
	}

	if err := h.service.DeleteByModelID(c.Request().Context(), modelID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// Count godoc
// @Summary Get model count
// @Description Get the total count of models, optionally filtered by type
// @Tags models
// @Param type query string false "Model type (chat, embedding)"
// @Success 200 {object} models.CountResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /models/count [get]
func (h *ModelsHandler) Count(c echo.Context) error {
	modelType := c.QueryParam("type")

	var count int64
	var err error

	if modelType != "" {
		count, err = h.service.CountByType(c.Request().Context(), models.ModelType(modelType))
	} else {
		count, err = h.service.Count(c.Request().Context())
	}

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, models.CountResponse{Count: count})
}
