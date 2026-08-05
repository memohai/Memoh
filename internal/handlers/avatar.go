package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	avatarpkg "github.com/memohai/memoh/internal/avatar"
	"github.com/memohai/memoh/internal/media"
)

// AvatarHandler proxies authenticated immutable avatar objects from private storage.
type AvatarHandler struct {
	logger  *slog.Logger
	service *avatarpkg.Service
}

func NewAvatarHandler(log *slog.Logger, service *avatarpkg.Service) *AvatarHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AvatarHandler{
		logger:  log.With(slog.String("handler", "avatar")),
		service: service,
	}
}

func (h *AvatarHandler) Register(e *echo.Echo) {
	e.GET("/avatars/:content_hash", h.Serve)
	e.HEAD("/avatars/:content_hash", h.Serve)
}

// Serve godoc
// @Summary Read an avatar image
// @Description Stream an immutable content-addressed avatar through the authenticated Memoh API
// @Tags avatars
// @Produce image/png
// @Produce image/jpeg
// @Produce image/gif
// @Produce image/webp
// @Param content_hash path string true "SHA-256 content hash"
// @Success 200 {file} binary
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /avatars/{content_hash} [get].
func (h *AvatarHandler) Serve(c echo.Context) error {
	contentHash := media.NormalizeContentHash(c.Param("content_hash"))
	if !media.IsContentHash(contentHash) {
		return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
	}
	if h == nil || h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "avatar service unavailable")
	}
	reader, asset, err := h.service.Open(c.Request().Context(), contentHash)
	if err != nil {
		if errors.Is(err, media.ErrAssetNotFound) || errors.Is(err, avatarpkg.ErrInvalidImage) {
			return echo.NewHTTPError(http.StatusNotFound, "avatar not found")
		}
		h.logger.Error("open avatar failed", slog.Any("error", err), slog.String("content_hash", contentHash))
		return echo.NewHTTPError(http.StatusInternalServerError, "avatar unavailable")
	}
	defer func() { _ = reader.Close() }()

	contentType := strings.TrimSpace(asset.Mime)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Response().Header().Set(echo.HeaderContentType, contentType)
	c.Response().Header().Set(echo.HeaderCacheControl, "private, max-age=31536000, immutable")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	if c.Request().Method == http.MethodHead {
		return c.NoContent(http.StatusOK)
	}
	c.Response().WriteHeader(http.StatusOK)
	if _, err := io.Copy(c.Response().Writer, reader); err != nil {
		h.logger.Warn("serve avatar stream failed", slog.Any("error", err), slog.String("content_hash", contentHash))
	}
	return nil
}
