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

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/identity"
)

// ChatHandler handles chat CRUD, messaging, participants, settings, and routes.
type ChatHandler struct {
	resolver       *chat.Resolver
	chatService    *chat.Service
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

// NewChatHandler creates a ChatHandler.
func NewChatHandler(log *slog.Logger, resolver *chat.Resolver, chatService *chat.Service, botService *bots.Service, accountService *accounts.Service) *ChatHandler {
	return &ChatHandler{
		resolver:       resolver,
		chatService:    chatService,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "chat")),
	}
}

// Register registers all chat routes.
func (h *ChatHandler) Register(e *echo.Echo) {
	// Chat lifecycle (under bot).
	botGroup := e.Group("/bots/:bot_id/chats")
	botGroup.POST("", h.CreateChat)
	botGroup.GET("", h.ListChats)

	// Chat operations.
	chatGroup := e.Group("/chats/:chat_id")
	chatGroup.GET("", h.GetChat)
	chatGroup.DELETE("", h.DeleteChat)

	// Messages.
	chatGroup.POST("/messages", h.SendMessage)
	chatGroup.POST("/messages/stream", h.StreamMessage)
	chatGroup.GET("/messages", h.ListMessages)

	// Participants.
	chatGroup.GET("/participants", h.ListParticipants)
	chatGroup.POST("/participants", h.AddParticipant)
	chatGroup.DELETE("/participants/:user_id", h.RemoveParticipant)

	// Settings.
	chatGroup.GET("/settings", h.GetSettings)
	chatGroup.PUT("/settings", h.UpdateSettings)

	// Routes.
	chatGroup.GET("/routes", h.ListRoutes)
	chatGroup.POST("/routes", h.CreateRoute)
	chatGroup.DELETE("/routes/:route_id", h.DeleteRoute)

	// Threads.
	chatGroup.GET("/threads", h.ListThreads)
}

// --- Chat Lifecycle ---

// CreateChat creates a new chat for a bot.
func (h *ChatHandler) CreateChat(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}

	var req chat.CreateChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.chatService.Create(c.Request().Context(), botID, channelIdentityID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, result)
}

// ListChats lists chats for a bot where the user has participant or observed access.
func (h *ChatHandler) ListChats(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}

	chats, err := h.chatService.ListByBotAndChannelIdentity(c.Request().Context(), botID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": chats})
}

// GetChat returns a chat by ID.
func (h *ChatHandler) GetChat(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireReadable(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	result, err := h.chatService.Get(c.Request().Context(), chatID)
	if err != nil {
		if errors.Is(err, chat.ErrChatNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "chat not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

// DeleteChat deletes a chat (owner only).
func (h *ChatHandler) DeleteChat(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleOwner); err != nil {
		return err
	}

	if err := h.chatService.Delete(c.Request().Context(), chatID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Messages ---

// SendMessage sends a synchronous chat message.
func (h *ChatHandler) SendMessage(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	chatObj, err := h.chatService.Get(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = chatObj.BotID
	req.ChatID = chatID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = channelIdentityID
	req.SourceChannelIdentityID = channelIdentityID

	resp, err := h.resolver.Chat(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// StreamMessage sends a streaming chat message.
func (h *ChatHandler) StreamMessage(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	chatObj, err := h.chatService.Get(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}

	var req chat.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = chatObj.BotID
	req.ChatID = chatID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = channelIdentityID
	req.SourceChannelIdentityID = channelIdentityID

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	chunkChan, errChan := h.resolver.StreamChat(c.Request().Context(), req)
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	writer := bufio.NewWriter(c.Response().Writer)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				writer.WriteString("data: [DONE]\n\n")
				writer.Flush()
				flusher.Flush()
				return nil
			}
			data, err := json.Marshal(chunk)
			if err != nil {
				continue
			}
			writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
			writer.Flush()
			flusher.Flush()
		case err := <-errChan:
			if err != nil {
				h.logger.Error("chat stream failed", slog.Any("error", err))
				errData := map[string]string{"error": err.Error()}
				data, marshalErr := json.Marshal(errData)
				if marshalErr != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, marshalErr.Error())
				}
				writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
				writer.Flush()
				flusher.Flush()
				return nil
			}
		}
	}
}

// ListMessages lists messages for a chat.
func (h *ChatHandler) ListMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireReadable(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	messages, err := h.chatService.ListMessages(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": messages})
}

// --- Participants ---

// ListParticipants lists participants for a chat.
func (h *ChatHandler) ListParticipants(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	participants, err := h.chatService.ListParticipants(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": participants})
}

// AddParticipant adds a participant to a chat (owner/admin only).
func (h *ChatHandler) AddParticipant(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleAdmin); err != nil {
		return err
	}

	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(body.UserID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id is required")
	}

	p, err := h.chatService.AddParticipant(c.Request().Context(), chatID, body.UserID, body.Role)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, p)
}

// RemoveParticipant removes a participant from a chat.
func (h *ChatHandler) RemoveParticipant(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleAdmin); err != nil {
		return err
	}

	targetUserID := strings.TrimSpace(c.Param("user_id"))
	if targetUserID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_id is required")
	}

	if err := h.chatService.RemoveParticipant(c.Request().Context(), chatID, targetUserID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Settings ---

// GetSettings returns settings for a chat.
func (h *ChatHandler) GetSettings(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	settings, err := h.chatService.GetSettings(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, settings)
}

// UpdateSettings updates settings for a chat (owner/admin only).
func (h *ChatHandler) UpdateSettings(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	chatObj, err := h.chatService.Get(c.Request().Context(), chatID)
	if err != nil {
		if errors.Is(err, chat.ErrChatNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "chat not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if chatObj.Kind == chat.KindGroup {
		if _, err := h.authorizeBotManage(c.Request().Context(), channelIdentityID, chatObj.BotID); err != nil {
			return err
		}
	} else {
		if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleAdmin); err != nil {
			return err
		}
	}

	var req chat.UpdateSettingsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	settings, err := h.chatService.UpdateSettings(c.Request().Context(), chatID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, settings)
}

// --- Routes ---

// ListRoutes lists routes for a chat.
func (h *ChatHandler) ListRoutes(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	routes, err := h.chatService.ListRoutes(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": routes})
}

// CreateRoute creates a new route for a chat (cross-channel).
func (h *ChatHandler) CreateRoute(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleAdmin); err != nil {
		return err
	}

	var route chat.Route
	if err := c.Bind(&route); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	result, err := h.chatService.CreateRoute(c.Request().Context(), chatID, route)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, result)
}

// DeleteRoute deletes a route.
func (h *ChatHandler) DeleteRoute(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireRole(c.Request().Context(), chatID, channelIdentityID, chat.RoleAdmin); err != nil {
		return err
	}

	routeID := strings.TrimSpace(c.Param("route_id"))
	if routeID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "route_id is required")
	}
	if err := h.chatService.DeleteRoute(c.Request().Context(), routeID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Threads ---

// ListThreads lists threads for a parent chat.
func (h *ChatHandler) ListThreads(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if err := h.requireParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	threads, err := h.chatService.ListThreads(c.Request().Context(), chatID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": threads})
}

// --- helpers ---

func (h *ChatHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	channelIdentityID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateChannelIdentityID(channelIdentityID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return channelIdentityID, nil
}

func (h *ChatHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.accountService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, channelIdentityID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: true})
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

func (h *ChatHandler) authorizeBotManage(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.accountService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, channelIdentityID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: false})
	if err != nil {
		if errors.Is(err, bots.ErrBotNotFound) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		if errors.Is(err, bots.ErrBotAccessDenied) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusForbidden, "bot management access denied")
		}
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return bot, nil
}

func (h *ChatHandler) requireParticipant(ctx context.Context, chatID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	// Admin bypass.
	if h.accountService != nil {
		isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if isAdmin {
			return nil
		}
	}
	ok, err := h.chatService.IsParticipant(ctx, chatID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, "not a participant")
	}
	return nil
}

func (h *ChatHandler) requireReadable(ctx context.Context, chatID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	// Admin bypass.
	if h.accountService != nil {
		isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if isAdmin {
			return nil
		}
	}
	_, err := h.chatService.GetReadAccess(ctx, chatID, channelIdentityID)
	if err != nil {
		if errors.Is(err, chat.ErrPermissionDenied) {
			return echo.NewHTTPError(http.StatusForbidden, "not allowed to read chat")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

func (h *ChatHandler) requireRole(ctx context.Context, chatID, channelIdentityID, minRole string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	// Admin bypass.
	if h.accountService != nil {
		isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if isAdmin {
			return nil
		}
	}
	p, err := h.chatService.GetParticipant(ctx, chatID, channelIdentityID)
	if err != nil {
		if errors.Is(err, chat.ErrNotParticipant) {
			return echo.NewHTTPError(http.StatusForbidden, "not a participant")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !roleAtLeast(p.Role, minRole) {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
	}
	return nil
}

func roleAtLeast(actual, required string) bool {
	roleLevel := map[string]int{
		chat.RoleOwner:  3,
		chat.RoleAdmin:  2,
		chat.RoleMember: 1,
	}
	return roleLevel[actual] >= roleLevel[required]
}
