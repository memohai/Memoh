package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/users"
)

type ChatHandler struct {
	resolver    *chat.Resolver
	botService  *bots.Service
	userService *users.Service
	logger      *slog.Logger
}

func NewChatHandler(log *slog.Logger, resolver *chat.Resolver, botService *bots.Service, userService *users.Service) *ChatHandler {
	return &ChatHandler{
		resolver:    resolver,
		botService:  botService,
		userService: userService,
		logger:      log.With(slog.String("handler", "chat")),
	}
}

func (h *ChatHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/chat")
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
// @Router /bots/{bot_id}/chat [post]
func (h *ChatHandler) Chat(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = botID
	req.SessionID = sessionID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = userID
	if strings.TrimSpace(req.ContactID) == "" {
		req.ContactID = userID
	}
	if strings.TrimSpace(req.ContactName) == "" {
		req.ContactName = "User"
	}

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
// @Success 200 {string} string
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/chat/stream [post]
func (h *ChatHandler) StreamChat(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = botID
	req.SessionID = sessionID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = userID
	if strings.TrimSpace(req.ContactID) == "" {
		req.ContactID = userID
	}
	if strings.TrimSpace(req.ContactName) == "" {
		req.ContactName = "User"
	}

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

func (h *ChatHandler) authorizeBotAccess(ctx context.Context, actorID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.userService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.userService.IsAdmin(ctx, actorID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, actorID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: true})
	if err != nil {
		if errors.Is(err, bots.ErrBotNotFound) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		if errors.Is(err, bots.ErrBotAccessDenied) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusForbidden, "bot access denied")
		}
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return bot, nil
}
