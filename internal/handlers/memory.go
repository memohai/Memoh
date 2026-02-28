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
	fsops "github.com/memohai/memoh/internal/fs"
	memprovider "github.com/memohai/memoh/internal/memory/provider"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/settings"
)

// MemoryHandler handles memory CRUD operations scoped by conversation.
type MemoryHandler struct {
	chatService     *conversation.Service
	accountService  *accounts.Service
	settingsService *settings.Service
	memoryRegistry  *memprovider.Registry
	memoryStore     *storefs.Service
	logger          *slog.Logger
}

type memoryAddPayload struct {
	Message          string                `json:"message,omitempty"`
	Messages         []memprovider.Message `json:"messages,omitempty"`
	Namespace        string                `json:"namespace,omitempty"`
	RunID            string                `json:"run_id,omitempty"`
	Metadata         map[string]any        `json:"metadata,omitempty"`
	Filters          map[string]any        `json:"filters,omitempty"`
	Infer            *bool                 `json:"infer,omitempty"`
	EmbeddingEnabled *bool                 `json:"embedding_enabled,omitempty"`
}

type memorySearchPayload struct {
	Query            string         `json:"query"`
	RunID            string         `json:"run_id,omitempty"`
	Limit            int            `json:"limit,omitempty"`
	Filters          map[string]any `json:"filters,omitempty"`
	Sources          []string       `json:"sources,omitempty"`
	EmbeddingEnabled *bool          `json:"embedding_enabled,omitempty"`
	NoStats          bool           `json:"no_stats,omitempty"`
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
func NewMemoryHandler(log *slog.Logger, chatService *conversation.Service, accountService *accounts.Service) *MemoryHandler {
	return &MemoryHandler{
		chatService:    chatService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "memory")),
	}
}

// SetMemoryRegistry sets the provider registry for provider-based memory operations.
func (h *MemoryHandler) SetMemoryRegistry(registry *memprovider.Registry) {
	h.memoryRegistry = registry
}

// SetSettingsService sets the settings service for provider resolution.
func (h *MemoryHandler) SetSettingsService(svc *settings.Service) {
	h.settingsService = svc
}

// resolveProvider returns the memory provider for a bot, or nil if not configured.
func (h *MemoryHandler) resolveProvider(ctx context.Context, botID string) memprovider.Provider {
	if h.memoryRegistry == nil || h.settingsService == nil {
		return nil
	}
	botSettings, err := h.settingsService.GetBot(ctx, botID)
	if err != nil {
		return nil
	}
	providerID := strings.TrimSpace(botSettings.MemoryProviderID)
	if providerID == "" {
		return nil
	}
	p, err := h.memoryRegistry.Get(providerID)
	if err != nil {
		h.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", err))
		return nil
	}
	return p
}

// SetFSService sets the optional filesystem persistence layer.
func (h *MemoryHandler) SetFSService(fs *fsops.Service) {
	if fs == nil {
		h.memoryStore = nil
		return
	}
	h.memoryStore = storefs.New(fs)
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

func (h *MemoryHandler) checkService(ctx context.Context, botID string) (memprovider.Provider, error) {
	if p := h.resolveProvider(ctx, botID); p != nil {
		return p, nil
	}
	return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "memory service not available")
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
// @Success 200 {object} provider.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [post]
func (h *MemoryHandler) ChatAdd(c echo.Context) error {
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

	scopeID, botID, err := h.resolveWriteScope(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	filters := buildNamespaceFilters(namespace, scopeID, payload.Filters)
	req := memprovider.AddRequest{
		Message:          payload.Message,
		Messages:         payload.Messages,
		BotID:            botID,
		RunID:            payload.RunID,
		Metadata:         payload.Metadata,
		Filters:          filters,
		Infer:            payload.Infer,
		EmbeddingEnabled: payload.EmbeddingEnabled,
	}

	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}
	resp, err := provider.Add(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.persistAddedMemoriesAsync(botID, resp.Results, filters)

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
// @Success 200 {object} provider.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/search [post]
func (h *MemoryHandler) ChatSearch(c echo.Context) error {
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

	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	var allResults []memprovider.MemoryItem
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, payload.Filters)
		if botID != "" {
			filters["bot_id"] = botID
		}
		req := memprovider.SearchRequest{
			Query:            payload.Query,
			BotID:            botID,
			RunID:            payload.RunID,
			Limit:            payload.Limit,
			Filters:          filters,
			Sources:          payload.Sources,
			EmbeddingEnabled: payload.EmbeddingEnabled,
			NoStats:          payload.NoStats,
		}
		resp, searchErr := provider.Search(c.Request().Context(), req)
		if searchErr != nil {
			h.logger.Warn("search namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", searchErr))
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

	return c.JSON(http.StatusOK, memprovider.SearchResponse{Results: allResults})
}

// ChatGetAll godoc
// @Summary Get all memories
// @Description List all memories in the bot-shared namespace
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param no_stats query bool false "Skip optional stats in memory search response"
// @Success 200 {object} provider.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [get]
func (h *MemoryHandler) ChatGetAll(c echo.Context) error {
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

	noStats := strings.EqualFold(c.QueryParam("no_stats"), "true")
	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	var allResults []memprovider.MemoryItem
	for _, scope := range scopes {
		req := memprovider.GetAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
			NoStats: noStats,
		}
		resp, getAllErr := provider.GetAll(c.Request().Context(), req)
		if getAllErr != nil {
			h.logger.Warn("getall namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", getAllErr))
			continue
		}
		allResults = append(allResults, resp.Results...)
	}
	allResults = deduplicateMemoryItems(allResults)

	return c.JSON(http.StatusOK, memprovider.SearchResponse{Results: allResults})
}

// ChatDelete godoc
// @Summary Delete memories
// @Description Delete specific memories by IDs, or delete all memories if no IDs are provided
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memoryDeletePayload false "Optional: specify memory_ids to delete; if omitted, deletes all"
// @Success 200 {object} provider.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory [delete]
func (h *MemoryHandler) ChatDelete(c echo.Context) error {
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

	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	var payload memoryDeletePayload
	_ = c.Bind(&payload)

	if len(payload.MemoryIDs) > 0 {
		resp, delErr := provider.DeleteBatch(c.Request().Context(), payload.MemoryIDs)
		if delErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, delErr.Error())
		}
		h.deleteMemoryFiles(c.Request().Context(), containerID, payload.MemoryIDs)
		return c.JSON(http.StatusOK, resp)
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		req := memprovider.DeleteAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
		}
		if _, delErr := provider.DeleteAll(c.Request().Context(), req); delErr != nil {
			h.logger.Warn("deleteall namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", delErr))
		}
	}
	h.deleteAllMemoryFiles(c.Request().Context(), containerID)
	return c.JSON(http.StatusOK, memprovider.DeleteResponse{Message: "All memories deleted successfully!"})
}

// ChatDeleteOne godoc
// @Summary Delete a single memory
// @Description Delete a single memory by its ID
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Memory ID"
// @Success 200 {object} provider.DeleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/{id} [delete]
func (h *MemoryHandler) ChatDeleteOne(c echo.Context) error {
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

	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	memoryID := strings.TrimSpace(c.Param("memory_id"))
	if memoryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "memory_id is required")
	}
	resp, err := provider.Delete(c.Request().Context(), memoryID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.deleteMemoryFiles(c.Request().Context(), containerID, []string{memoryID})
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
// @Success 200 {object} provider.CompactResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/compact [post]
func (h *MemoryHandler) ChatCompact(c echo.Context) error {
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

	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	scope := scopes[0]
	filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil)
	result, err := provider.Compact(c.Request().Context(), filters, ratio, decayDays)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if h.memoryStore != nil {
		if err := h.memoryStore.RebuildFiles(c.Request().Context(), containerID, result.Results, filters); err != nil {
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
// @Success 200 {object} provider.UsageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/usage [get]
func (h *MemoryHandler) ChatUsage(c echo.Context) error {
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

	chatObj, err := h.chatService.Get(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "chat not found")
	}
	botID := strings.TrimSpace(chatObj.BotID)
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	var totalUsage memprovider.UsageResponse
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil)
		usage, usageErr := provider.Usage(c.Request().Context(), filters)
		if usageErr != nil {
			h.logger.Warn("usage namespace failed", slog.String("namespace", scope.Namespace), slog.Any("error", usageErr))
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
// @Description Read memory files from the container filesystem (source of truth) and restore missing entries to memory storage
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} provider.RebuildResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/rebuild [post]
func (h *MemoryHandler) ChatRebuild(c echo.Context) error {
	if h.memoryStore == nil {
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

	botID := c.Param("bot_id")
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	fsItems, err := h.memoryStore.ReadAllMemoryFiles(c.Request().Context(), containerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "read memory files failed: "+err.Error())
	}

	manifest, _ := h.memoryStore.ReadManifest(c.Request().Context(), containerID)

	scopes, err := h.resolveEnabledScopes(c.Request().Context(), containerID)
	if err != nil {
		return err
	}

	existingIDs := map[string]struct{}{}
	for _, scope := range scopes {
		req := memprovider.GetAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
		}
		resp, getAllErr := provider.GetAll(c.Request().Context(), req)
		if getAllErr != nil {
			h.logger.Warn("rebuild getall failed", slog.String("namespace", scope.Namespace), slog.Any("error", getAllErr))
			continue
		}
		for _, item := range resp.Results {
			existingIDs[item.ID] = struct{}{}
		}
	}

	infer := false
	var restoredCount int
	for _, fsItem := range fsItems {
		if _, exists := existingIDs[fsItem.ID]; exists {
			continue
		}
		var filters map[string]any
		if manifest != nil {
			if entry, ok := manifest.Entries[fsItem.ID]; ok && len(entry.Filters) > 0 {
				filters = entry.Filters
			}
		}
		if len(filters) == 0 && len(scopes) > 0 {
			filters = buildNamespaceFilters(scopes[0].Namespace, scopes[0].ScopeID, nil)
		}

		if _, addErr := provider.Add(c.Request().Context(), memprovider.AddRequest{
			Message: fsItem.Memory,
			BotID:   botID,
			Filters: filters,
			Infer:   &infer,
		}); addErr != nil {
			h.logger.Warn("rebuild add failed", slog.String("id", fsItem.ID), slog.Any("error", addErr))
			continue
		}
		restoredCount++
	}

	return c.JSON(http.StatusOK, memprovider.RebuildResult{
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

func deduplicateMemoryItems(items []memprovider.MemoryItem) []memprovider.MemoryItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]memprovider.MemoryItem, 0, len(items))
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

func (h *MemoryHandler) persistAddedMemoriesAsync(botID string, items []memprovider.MemoryItem, filters map[string]any) {
	if h.memoryStore == nil || len(items) == 0 {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.memoryStore.PersistMemories(bgCtx, botID, items, filters); err != nil {
			h.logger.Warn("async memory persist failed", slog.Any("error", err))
		}
	}()
}

func (h *MemoryHandler) deleteMemoryFiles(ctx context.Context, botID string, ids []string) {
	if h.memoryStore == nil || len(ids) == 0 {
		return
	}
	if err := h.memoryStore.RemoveMemories(ctx, botID, ids); err != nil {
		h.logger.Warn("memory fs remove failed", slog.Any("error", err))
	}
}

func (h *MemoryHandler) deleteAllMemoryFiles(ctx context.Context, botID string) {
	if h.memoryStore == nil {
		return
	}
	if err := h.memoryStore.RemoveAllMemories(ctx, botID); err != nil {
		h.logger.Warn("memory fs delete all failed", slog.Any("error", err))
	}
}
