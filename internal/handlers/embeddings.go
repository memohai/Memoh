package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/embeddings"
	"github.com/memohai/memoh/internal/models"
)

// DefaultEmbeddingTimeout is the default HTTP timeout for embedding requests.
const DefaultEmbeddingTimeout = 10 * time.Second

// EmbeddingsHandler serves POST /embeddings for text or multimodal embedding.
type EmbeddingsHandler struct {
	resolver *embeddings.Resolver
	logger   *slog.Logger
}

// EmbeddingsRequest is the body for POST /embeddings (type, provider, model, dimensions, input).
type EmbeddingsRequest struct {
	Type       string          `json:"type"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Dimensions int             `json:"dimensions,omitempty"`
	Input      EmbeddingsInput `json:"input"`
}

// EmbeddingsInput holds text and optional image/video URL.
type EmbeddingsInput struct {
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	VideoURL string `json:"video_url,omitempty"`
}

// EmbeddingsResponse is the success body (type, provider, model, dimensions, embedding, usage).
type EmbeddingsResponse struct {
	Type       string          `json:"type"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Dimensions int             `json:"dimensions"`
	Embedding  []float32       `json:"embedding"`
	Usage      EmbeddingsUsage `json:"usage,omitzero"`
	Message    string          `json:"message,omitempty"`
}

// EmbeddingsUsage holds token and duration usage from the embedding API.
type EmbeddingsUsage struct {
	InputTokens int `json:"input_tokens,omitempty"`
	ImageTokens int `json:"image_tokens,omitempty"`
	Duration    int `json:"duration,omitempty"`
}

// NewEmbeddingsHandler creates an embeddings handler with a resolver built from models service and queries.
func NewEmbeddingsHandler(log *slog.Logger, modelsService *models.Service, queries *sqlc.Queries) *EmbeddingsHandler {
	return &EmbeddingsHandler{
		resolver: embeddings.NewResolver(log, modelsService, queries, DefaultEmbeddingTimeout),
		logger:   log.With(slog.String("handler", "embeddings")),
	}
}

// Register mounts POST /embeddings on the Echo instance.
func (h *EmbeddingsHandler) Register(e *echo.Echo) {
	e.POST("/embeddings", h.Embed)
}

// Embed godoc
// @Summary Create embeddings
// @Description Create text or multimodal embeddings
// @Tags embeddings
// @Param payload body EmbeddingsRequest true "Embeddings request"
// @Success 200 {object} EmbeddingsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 501 {object} EmbeddingsResponse
// @Failure 500 {object} ErrorResponse
// @Router /embeddings [post].
func (h *EmbeddingsHandler) Embed(c echo.Context) error {
	var req EmbeddingsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.Type = normalizeEmbeddingValue(req.Type)
	req.Provider = normalizeEmbeddingValue(req.Provider)
	req.Model = strings.TrimSpace(req.Model)
	req.Input.Text = strings.TrimSpace(req.Input.Text)
	req.Input.ImageURL = strings.TrimSpace(req.Input.ImageURL)
	req.Input.VideoURL = strings.TrimSpace(req.Input.VideoURL)

	result, err := h.resolver.Embed(c.Request().Context(), embeddings.Request{
		Type:       req.Type,
		Provider:   req.Provider,
		Model:      req.Model,
		Dimensions: req.Dimensions,
		Input: embeddings.Input{
			Text:     req.Input.Text,
			ImageURL: req.Input.ImageURL,
			VideoURL: req.Input.VideoURL,
		},
	})
	if err != nil {
		message := err.Error()
		switch message {
		case "no embedding models available":
			return echo.NewHTTPError(http.StatusNotFound, message)
		case "embedding model not found":
			return echo.NewHTTPError(http.StatusBadRequest, message)
		case "provider not implemented":
			resp := EmbeddingsResponse{
				Type:       req.Type,
				Provider:   req.Provider,
				Model:      req.Model,
				Dimensions: req.Dimensions,
				Embedding:  []float32{},
				Message:    "embeddings provider not implemented",
			}
			return c.JSON(http.StatusNotImplemented, resp)
		default:
			if strings.Contains(message, "required") || strings.Contains(message, "invalid") {
				return echo.NewHTTPError(http.StatusBadRequest, message)
			}
			return echo.NewHTTPError(http.StatusInternalServerError, message)
		}
	}

	return c.JSON(http.StatusOK, EmbeddingsResponse{
		Type:       result.Type,
		Provider:   result.Provider,
		Model:      result.Model,
		Dimensions: result.Dimensions,
		Embedding:  result.Embedding,
		Usage: EmbeddingsUsage{
			InputTokens: result.Usage.InputTokens,
			ImageTokens: result.Usage.ImageTokens,
			Duration:    result.Usage.Duration,
		},
	})
}

func normalizeEmbeddingValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
