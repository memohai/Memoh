package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/memory"
)

// MemoryHandler handles memory CRUD operations scoped by chat.
type MemoryHandler struct {
	service        *memory.Service
	chatService    *chat.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

type memoryAddPayload struct {
	Message          string           `json:"message,omitempty"`
	Messages         []memory.Message `json:"messages,omitempty"`
	Namespace        string           `json:"namespace,omitempty"`
	RunID            string           `json:"run_id,omitempty"`
	Metadata         map[string]any   `json:"metadata,omitempty"`
	Filters          map[string]any   `json:"filters,omitempty"`
	Infer            *bool            `json:"infer,omitempty"`
	EmbeddingEnabled *bool            `json:"embedding_enabled,omitempty"`
}

type memorySearchPayload struct {
	Query            string         `json:"query"`
	RunID            string         `json:"run_id,omitempty"`
	Limit            int            `json:"limit,omitempty"`
	Filters          map[string]any `json:"filters,omitempty"`
	Sources          []string       `json:"sources,omitempty"`
	EmbeddingEnabled *bool          `json:"embedding_enabled,omitempty"`
}

// namespaceScope holds namespace + scopeId for a single memory scope.
type namespaceScope struct {
	Namespace string
	ScopeID   string
}

// NewMemoryHandler creates a MemoryHandler.
func NewMemoryHandler(log *slog.Logger, service *memory.Service, chatService *chat.Service, accountService *accounts.Service) *MemoryHandler {
	return &MemoryHandler{
		service:        service,
		chatService:    chatService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "memory")),
	}
}

// Register registers chat-level memory routes.
func (h *MemoryHandler) Register(e *echo.Echo) {
	chatGroup := e.Group("/chats/:chat_id/memory")
	chatGroup.POST("", h.ChatAdd)
	chatGroup.POST("/search", h.ChatSearch)
	chatGroup.GET("", h.ChatGetAll)
	chatGroup.DELETE("", h.ChatDeleteAll)
}

func (h *MemoryHandler) checkService() error {
	if h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory service not available")
	}
	return nil
}

// --- Chat-level memory endpoints ---

// ChatAdd adds memory to a specific namespace (validated against chat_settings).
func (h *MemoryHandler) ChatAdd(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "chat_id is required")
	}
	if err := h.requireChatParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	var payload memoryAddPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	namespace := strings.TrimSpace(payload.Namespace)
	if namespace == "" {
		namespace = "chat"
	}

	// Resolve correct scopeId/botId and validate namespace is enabled.
	scopeID, botID, err := h.resolveWriteScope(c.Request().Context(), chatID, channelIdentityID, namespace)
	if err != nil {
		return err
	}

	filters := buildNamespaceFilters(namespace, scopeID, payload.Filters)
	req := memory.AddRequest{
		Message:          payload.Message,
		Messages:         payload.Messages,
		BotID:            botID,
		RunID:            payload.RunID,
		Metadata:         payload.Metadata,
		Filters:          filters,
		Infer:            payload.Infer,
		EmbeddingEnabled: payload.EmbeddingEnabled,
	}
	resp, err := h.service.Add(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// ChatSearch searches memory across all enabled namespaces per chat_settings.
func (h *MemoryHandler) ChatSearch(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "chat_id is required")
	}
	if err := h.requireChatParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	var payload memorySearchPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), chatID, channelIdentityID)
	if err != nil {
		return err
	}

	// Search across all enabled namespaces and merge results.
	var allResults []memory.MemoryItem
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, payload.Filters)
		req := memory.SearchRequest{
			Query:            payload.Query,
			RunID:            payload.RunID,
			Limit:            payload.Limit,
			Filters:          filters,
			Sources:          payload.Sources,
			EmbeddingEnabled: payload.EmbeddingEnabled,
		}
		resp, err := h.service.Search(c.Request().Context(), req)
		if err != nil {
			h.logger.Warn("search namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", err))
			continue
		}
		allResults = append(allResults, resp.Results...)
	}

	// Deduplicate by ID and sort by score descending.
	allResults = deduplicateMemoryItems(allResults)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})
	if payload.Limit > 0 && len(allResults) > payload.Limit {
		allResults = allResults[:payload.Limit]
	}

	return c.JSON(http.StatusOK, memory.SearchResponse{Results: allResults})
}

// ChatGetAll lists all memories across enabled namespaces.
func (h *MemoryHandler) ChatGetAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "chat_id is required")
	}
	if err := h.requireChatParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), chatID, channelIdentityID)
	if err != nil {
		return err
	}

	var allResults []memory.MemoryItem
	for _, scope := range scopes {
		req := memory.GetAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
		}
		resp, err := h.service.GetAll(c.Request().Context(), req)
		if err != nil {
			h.logger.Warn("getall namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", err))
			continue
		}
		allResults = append(allResults, resp.Results...)
	}
	allResults = deduplicateMemoryItems(allResults)

	return c.JSON(http.StatusOK, memory.SearchResponse{Results: allResults})
}

// ChatDeleteAll deletes all memories across enabled namespaces.
func (h *MemoryHandler) ChatDeleteAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	chatID := strings.TrimSpace(c.Param("chat_id"))
	if chatID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "chat_id is required")
	}
	if err := h.requireChatParticipant(c.Request().Context(), chatID, channelIdentityID); err != nil {
		return err
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), chatID, channelIdentityID)
	if err != nil {
		return err
	}

	for _, scope := range scopes {
		req := memory.DeleteAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
		}
		if _, err := h.service.DeleteAll(c.Request().Context(), req); err != nil {
			h.logger.Warn("deleteall namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", err))
		}
	}
	return c.JSON(http.StatusOK, memory.DeleteResponse{Message: "Memory deleted successfully!"})
}

// --- helpers ---

// resolveEnabledScopes returns all namespace scopes enabled by chat_settings.
func (h *MemoryHandler) resolveEnabledScopes(ctx context.Context, chatID, channelIdentityID string) ([]namespaceScope, error) {
	if h.chatService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	chatObj, err := h.chatService.Get(ctx, chatID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	settings, err := h.chatService.GetSettings(ctx, chatID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var scopes []namespaceScope
	if settings.EnableChatMemory {
		scopes = append(scopes, namespaceScope{Namespace: "chat", ScopeID: chatID})
	}
	if settings.EnablePrivateMemory && strings.TrimSpace(channelIdentityID) != "" {
		scopes = append(scopes, namespaceScope{Namespace: "private", ScopeID: channelIdentityID})
	}
	if settings.EnablePublicMemory {
		scopes = append(scopes, namespaceScope{Namespace: "public", ScopeID: chatObj.BotID})
	}
	if len(scopes) == 0 {
		scopes = append(scopes, namespaceScope{Namespace: "chat", ScopeID: chatID})
	}
	return scopes, nil
}

// resolveWriteScope validates namespace and returns (scopeId, botId).
func (h *MemoryHandler) resolveWriteScope(ctx context.Context, chatID, channelIdentityID, namespace string) (string, string, error) {
	if h.chatService == nil {
		return "", "", echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	chatObj, err := h.chatService.Get(ctx, chatID)
	if err != nil {
		return "", "", echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	settings, err := h.chatService.GetSettings(ctx, chatID)
	if err != nil {
		return "", "", echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	switch namespace {
	case "chat":
		if !settings.EnableChatMemory {
			return "", "", echo.NewHTTPError(http.StatusForbidden, "chat memory is disabled for this chat")
		}
		return chatID, chatObj.BotID, nil
	case "private":
		if !settings.EnablePrivateMemory {
			return "", "", echo.NewHTTPError(http.StatusForbidden, "private memory is disabled for this chat")
		}
		if strings.TrimSpace(channelIdentityID) == "" {
			return "", "", echo.NewHTTPError(http.StatusBadRequest, "channel_identity_id required for private namespace")
		}
		return channelIdentityID, chatObj.BotID, nil
	case "public":
		if !settings.EnablePublicMemory {
			return "", "", echo.NewHTTPError(http.StatusForbidden, "public memory is disabled for this chat")
		}
		return chatObj.BotID, chatObj.BotID, nil
	default:
		return "", "", echo.NewHTTPError(http.StatusBadRequest, "invalid namespace: "+namespace)
	}
}

func buildNamespaceFilters(namespace, scopeID string, extra map[string]any) map[string]any {
	filters := map[string]any{
		"namespace": namespace,
		"scopeId":   scopeID,
	}
	for k, v := range extra {
		if k != "namespace" && k != "scopeId" {
			filters[k] = v
		}
	}
	return filters
}

func deduplicateMemoryItems(items []memory.MemoryItem) []memory.MemoryItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]memory.MemoryItem, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (h *MemoryHandler) requireChatParticipant(ctx context.Context, chatID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
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
		return echo.NewHTTPError(http.StatusForbidden, "not a chat participant")
	}
	return nil
}

func (h *MemoryHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	channelIdentityID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateChannelIdentityID(channelIdentityID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return channelIdentityID, nil
}
