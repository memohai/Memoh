package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/memory"
)

// MemoryHandler handles memory CRUD operations scoped by conversation.
type MemoryHandler struct {
	service        *memory.Service
	chatService    *conversation.Service
	accountService *accounts.Service
	memoryFS       *memory.MemoryFS
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

type memoryDeletePayload struct {
	MemoryIDs []string `json:"memory_ids,omitempty"`
}

type memoryCompactPayload struct {
	Ratio     float64 `json:"ratio"`
	DecayDays *int    `json:"decay_days,omitempty"`
}

// namespaceScope holds namespace + scopeId for a single memory scope.
type namespaceScope struct {
	Namespace string
	ScopeID   string
}

const sharedMemoryNamespace = "bot"

// NewMemoryHandler creates a MemoryHandler.
func NewMemoryHandler(log *slog.Logger, service *memory.Service, chatService *conversation.Service, accountService *accounts.Service) *MemoryHandler {
	return &MemoryHandler{
		service:        service,
		chatService:    chatService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "memory")),
	}
}

// SetMemoryFS sets the optional filesystem persistence layer.
func (h *MemoryHandler) SetMemoryFS(fs *memory.MemoryFS) {
	h.memoryFS = fs
}

// Register registers chat-level memory routes.
func (h *MemoryHandler) Register(e *echo.Echo) {
	chatGroup := e.Group("/bots/:bot_id/memory")
	chatGroup.POST("", h.ChatAdd)
	chatGroup.POST("/search", h.ChatSearch)
	chatGroup.POST("/compact", h.ChatCompact)
	chatGroup.POST("/rebuild", h.ChatRebuild)
	chatGroup.GET("", h.ChatGetAll)
	chatGroup.GET("/usage", h.ChatUsage)
	chatGroup.DELETE("", h.ChatDelete)
	chatGroup.DELETE("/:memory_id", h.ChatDeleteOne)
}

func (h *MemoryHandler) checkService() error {
	if h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory service not available")
	}
	return nil
}

// --- Chat-level memory endpoints ---

// ChatAdd godoc
// @Summary Add memory
// @Description Add memory into the bot-shared namespace
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memoryAddPayload true "Memory add payload"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [post]
func (h *MemoryHandler) ChatAdd(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	var payload memoryAddPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	namespace, err := normalizeSharedMemoryNamespace(payload.Namespace)
	if err != nil {
		return err
	}

	// Resolve bot scope for shared memory.
	scopeID, botID, err := h.resolveWriteScope(c.Request().Context(), containerID)
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

	// Async persist to filesystem.
	if h.memoryFS != nil && len(resp.Results) > 0 {
		items := resp.Results
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.memoryFS.PersistMemories(bgCtx, botID, items, filters); err != nil {
				h.logger.Warn("async memory persist failed", slog.Any("error", err))
			}
		}()
	}

	return c.JSON(http.StatusOK, resp)
}

// ChatSearch godoc
// @Summary Search memory
// @Description Search memory in the bot-shared namespace
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memorySearchPayload true "Memory search payload"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/search [post]
func (h *MemoryHandler) ChatSearch(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	var payload memorySearchPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}
	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)

	// Search shared namespace and merge results.
	var allResults []memory.MemoryItem
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, payload.Filters)
		if botID != "" {
			filters["bot_id"] = botID
		}
		req := memory.SearchRequest{
			Query:            payload.Query,
			BotID:            botID,
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

// ChatGetAll godoc
// @Summary Get all memories
// @Description List all memories in the bot-shared namespace
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} memory.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [get]
func (h *MemoryHandler) ChatGetAll(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
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

// ChatDelete godoc
// @Summary Delete memories
// @Description Delete specific memories by IDs, or delete all memories if no IDs are provided
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memoryDeletePayload false "Optional: specify memory_ids to delete; if omitted, deletes all"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [delete]
func (h *MemoryHandler) ChatDelete(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	var payload memoryDeletePayload
	// Body is optional; ignore bind errors for empty body.
	_ = c.Bind(&payload)

	// If memory_ids provided, delete specific memories.
	if len(payload.MemoryIDs) > 0 {
		resp, err := h.service.DeleteBatch(c.Request().Context(), payload.MemoryIDs)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		// Sync remove from filesystem.
		if h.memoryFS != nil {
			if err := h.memoryFS.RemoveMemories(c.Request().Context(), containerID, payload.MemoryIDs); err != nil {
				h.logger.Warn("delete memory fs remove failed", slog.Any("error", err))
			}
		}
		return c.JSON(http.StatusOK, resp)
	}

	// Otherwise delete all memories in the bot-shared namespace.
	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
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
	// Sync remove all from filesystem.
	if h.memoryFS != nil {
		if err := h.memoryFS.RemoveAllMemories(c.Request().Context(), containerID); err != nil {
			h.logger.Warn("deleteall memory fs remove failed", slog.Any("error", err))
		}
	}
	return c.JSON(http.StatusOK, memory.DeleteResponse{Message: "All memories deleted successfully!"})
}

// ChatDeleteOne godoc
// @Summary Delete a single memory
// @Description Delete a single memory by its ID
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Memory ID"
// @Success 200 {object} memory.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/{id} [delete]
func (h *MemoryHandler) ChatDeleteOne(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	memoryID := strings.TrimSpace(c.Param("memory_id"))
	if memoryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "memory_id is required")
	}
	resp, err := h.service.Delete(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Sync remove from filesystem.
	if h.memoryFS != nil {
		if err := h.memoryFS.RemoveMemories(c.Request().Context(), containerID, []string{memoryID}); err != nil {
			h.logger.Warn("delete one memory fs remove failed", slog.Any("error", err))
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// ChatCompact godoc
// @Summary Compact memories
// @Description Consolidate memories by merging similar/redundant entries using LLM.
// @Description
// @Description **ratio** (required, range (0,1]):
// @Description - 0.8 = light compression, mostly dedup, keep ~80% of entries
// @Description - 0.5 = moderate compression, merge similar facts, keep ~50%
// @Description - 0.3 = aggressive compression, heavily consolidate, keep ~30%
// @Description
// @Description **decay_days** (optional): enable time decay — memories older than N days are treated as low priority and more likely to be merged/dropped.
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memoryCompactPayload true "ratio (0,1] required; decay_days optional"
// @Success 200 {object} memory.CompactResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/compact [post]
func (h *MemoryHandler) ChatCompact(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	var payload memoryCompactPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if payload.Ratio <= 0 || payload.Ratio > 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "ratio is required and must be in range (0, 1]")
	}
	ratio := payload.Ratio
	var decayDays int
	if payload.DecayDays != nil && *payload.DecayDays > 0 {
		decayDays = *payload.DecayDays
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no memory scopes found")
	}

	// Compact the first (primary) scope.
	scope := scopes[0]
	filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil)
	result, err := h.service.Compact(c.Request().Context(), filters, ratio, decayDays)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Sync rebuild filesystem.
	if h.memoryFS != nil {
		if err := h.memoryFS.RebuildFiles(c.Request().Context(), containerID, result.Results, filters); err != nil {
			h.logger.Warn("compact memory fs rebuild failed", slog.Any("error", err))
		}
	}

	return c.JSON(http.StatusOK, result)
}

// ChatUsage godoc
// @Summary Get memory usage
// @Description Query the estimated storage usage of current memories
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} memory.UsageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/usage [get]
func (h *MemoryHandler) ChatUsage(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	var totalUsage memory.UsageResponse
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil)
		usage, err := h.service.Usage(c.Request().Context(), filters)
		if err != nil {
			h.logger.Warn("usage namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", err))
			continue
		}
		totalUsage.Count += usage.Count
		totalUsage.TotalTextBytes += usage.TotalTextBytes
		totalUsage.EstimatedStorageBytes += usage.EstimatedStorageBytes
	}
	if totalUsage.Count > 0 {
		totalUsage.AvgTextBytes = totalUsage.TotalTextBytes / int64(totalUsage.Count)
	}
	return c.JSON(http.StatusOK, totalUsage)
}

// ChatRebuild godoc
// @Summary Rebuild memories from filesystem
// @Description Read memory files from the container filesystem (source of truth) and restore missing entries to Qdrant
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} memory.RebuildResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/rebuild [post]
func (h *MemoryHandler) ChatRebuild(c echo.Context) error {
	if err := h.checkService(); err != nil {
		return err
	}
	if h.memoryFS == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory filesystem not configured")
	}
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	containerID, err := h.resolveBotContainerID(c)
	if err != nil {
		return err
	}
	if err := h.requireChatParticipant(c.Request().Context(), containerID, channelIdentityID); err != nil {
		return err
	}

	// Read filesystem entries.
	fsItems, err := h.memoryFS.ReadAllMemoryFiles(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "read memory files failed: "+err.Error())
	}

	// Read manifest for filters.
	manifest, _ := h.memoryFS.ReadManifest(c.Request().Context(), containerID)

	// Get current Qdrant entries.
	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	existingIDs := map[string]struct{}{}
	for _, scope := range scopes {
		req := memory.GetAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
		}
		resp, err := h.service.GetAll(c.Request().Context(), req)
		if err != nil {
			h.logger.Warn("rebuild getall failed", slog.String("namespace", scope.Namespace), slog.Any("error", err))
			continue
		}
		for _, item := range resp.Results {
			existingIDs[item.ID] = struct{}{}
		}
	}

	// Find and restore missing entries.
	var restoredCount int
	for _, fsItem := range fsItems {
		if _, exists := existingIDs[fsItem.ID]; exists {
			continue
		}
		// Resolve filters from manifest, fallback to first scope.
		var filters map[string]any
		if manifest != nil {
			if entry, ok := manifest.Entries[fsItem.ID]; ok && len(entry.Filters) > 0 {
				filters = entry.Filters
			}
		}
		if len(filters) == 0 && len(scopes) > 0 {
			filters = buildNamespaceFilters(scopes[0].Namespace, scopes[0].ScopeID, nil)
		}

		if _, err := h.service.RebuildAdd(c.Request().Context(), fsItem.ID, fsItem.Memory, filters); err != nil {
			h.logger.Warn("rebuild add failed", slog.String("id", fsItem.ID), slog.Any("error", err))
			continue
		}
		restoredCount++
	}

	return c.JSON(http.StatusOK, memory.RebuildResult{
		FsCount:       len(fsItems),
		QdrantCount:   len(existingIDs),
		MissingCount:  len(fsItems) - len(existingIDs),
		RestoredCount: restoredCount,
	})
}

// --- helpers ---

// resolveEnabledScopes returns the bot-shared namespace scope for the conversation.
func (h *MemoryHandler) resolveEnabledScopes(ctx context.Context, chatID string) ([]namespaceScope, error) {
	if h.chatService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	chatObj, err := h.chatService.Get(ctx, chatID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	if botID == "" {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "chat bot id is empty")
	}
	return []namespaceScope{{
		Namespace: sharedMemoryNamespace,
		ScopeID:   botID,
	}}, nil
}

// resolveWriteScope returns (scopeID, botID) for shared bot memory.
func (h *MemoryHandler) resolveWriteScope(ctx context.Context, chatID string) (string, string, error) {
	if h.chatService == nil {
		return "", "", echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	chatObj, err := h.chatService.Get(ctx, chatID)
	if err != nil {
		return "", "", echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	if botID == "" {
		return "", "", echo.NewHTTPError(http.StatusInternalServerError, "bot id is empty")
	}
	return botID, botID, nil
}

func normalizeSharedMemoryNamespace(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", sharedMemoryNamespace:
		return sharedMemoryNamespace, nil
	default:
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid namespace: "+raw)
	}
}

func (h *MemoryHandler) resolveBotContainerID(c echo.Context) (string, error) {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	return botID, nil
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
	return RequireChannelIdentityID(c)
}
