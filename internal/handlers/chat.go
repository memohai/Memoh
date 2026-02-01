package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/identity"
)

type ChatHandler struct {
	resolver *chat.Resolver
	logger   *slog.Logger
}

func NewChatHandler(log *slog.Logger, resolver *chat.Resolver) *ChatHandler {
	return &ChatHandler{
		resolver: resolver,
		logger:   log.With(slog.String("handler", "chat")),
	}
}

func (h *ChatHandler) Register(e *echo.Echo) {
	group := e.Group("/chat")
	group.POST("", h.Chat)
	group.POST("/stream", h.StreamChat)
}

// Chat godoc
// @Summary Chat with AI
// @Description Send a chat message and get a response. The system will automatically select an appropriate chat model from the database.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body chat.ChatRequest true "Chat request"
// @Success 200 {object} chat.ChatResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat [post]
func (h *ChatHandler) Chat(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.UserID = userID
	req.Token = c.Request().Header.Get("Authorization")
	req.Token = c.Request().Header.Get("Authorization")

	resp, err := h.resolver.Chat(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// StreamChat godoc
// @Summary Stream chat with AI
// @Description Send a chat message and get a streaming response. The system will automatically select an appropriate chat model from the database.
// @Tags chat
// @Accept json
// @Produce text/event-stream
// @Param request body chat.ChatRequest true "Chat request"
// @Success 200 {object} chat.StreamChunk
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /chat/stream [post]
func (h *ChatHandler) StreamChat(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.UserID = userID
	req.Token = c.Request().Header.Get("Authorization")
	req.Token = c.Request().Header.Get("Authorization")

	// Set headers for SSE
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	// Get streaming channels
	chunkChan, errChan := h.resolver.StreamChat(c.Request().Context(), req)

	// Create a flusher
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	writer := bufio.NewWriter(c.Response().Writer)

	// Stream chunks
	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Channel closed, send done message
				writer.WriteString("data: [DONE]\n\n")
				writer.Flush()
				flusher.Flush()
				return nil
			}

			// Marshal chunk to JSON
			data, err := json.Marshal(chunk)
			if err != nil {
				continue
			}

			// Write SSE format
			writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
			writer.Flush()
			flusher.Flush()

		case err := <-errChan:
			if err != nil {
				// Send error as SSE event
				errData := map[string]string{"error": err.Error()}
				data, _ := json.Marshal(errData)
				writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
				writer.Flush()
				flusher.Flush()
				return nil
			}
		}
	}
}

func (h *ChatHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}
