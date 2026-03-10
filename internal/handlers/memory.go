package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/mcp/mcpclient"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/settings"
)

// MemoryHandler handles memory CRUD operations scoped by bot.
type MemoryHandler struct {
	botService      *bots.Service
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

const (
	sharedMemoryNamespace    = "bot"
	defaultBuiltinProviderID = "__builtin_default__"
)

// NewMemoryHandler creates a MemoryHandler.
func NewMemoryHandler(log *slog.Logger, botService *bots.Service, accountService *accounts.Service) *MemoryHandler {
	return &MemoryHandler{
		botService:     botService,
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
	if h.memoryRegistry == nil {
		return nil
	}
	if h.settingsService != nil {
		botSettings, err := h.settingsService.GetBot(ctx, botID)
		if err == nil {
			providerID := strings.TrimSpace(botSettings.MemoryProviderID)
			if providerID != "" {
				p, getErr := h.memoryRegistry.Get(providerID)
				if getErr == nil {
					return p
				}
				h.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", getErr))
			}
		}
	}
	p, err := h.memoryRegistry.Get(defaultBuiltinProviderID)
	if err != nil {
		return nil
	}
	return p
}

// SetMCPClientProvider sets the gRPC client provider for filesystem persistence.
func (h *MemoryHandler) SetMCPClientProvider(p mcpclient.Provider) {
	if p == nil {
		h.memoryStore = nil
		return
	}
	h.memoryStore = storefs.New(p)
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

// --- Bot-level memory endpoints ---

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
// @Router /bots/{bot_id}/memory [post].
func (h *MemoryHandler) ChatAdd(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
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

	scopeID, resolvedBotID, err := h.resolveWriteScope(botID)
	if err != nil {
		return err
	}

	filters := buildNamespaceFilters(namespace, scopeID, payload.Filters)
	req := memprovider.AddRequest{
		Message:          payload.Message,
		Messages:         payload.Messages,
		BotID:            resolvedBotID,
		RunID:            payload.RunID,
		Metadata:         payload.Metadata,
		Filters:          filters,
		Infer:            payload.Infer,
		EmbeddingEnabled: payload.EmbeddingEnabled,
	}

	provider, checkErr := h.checkService(c.Request().Context(), resolvedBotID)
	if checkErr != nil {
		return checkErr
	}
	resp, err := provider.Add(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
// @Success 200 {object} provider.SearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/search [post].
func (h *MemoryHandler) ChatSearch(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var payload memorySearchPayload
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	scopes, err := h.resolveEnabledScopes(botID)
	if err != nil {
		return err
	}
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	results := make([]memprovider.MemoryItem, 0)
	for _, scope := range scopes {
		filters := buildNamespaceFilters(scope.Namespace, scope.ScopeID, payload.Filters)
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
		results = append(results, resp.Results...)
	}
	results = deduplicateMemoryItems(results)
	return c.JSON(http.StatusOK, memprovider.SearchResponse{Results: results})
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
// @Router /bots/{bot_id}/memory [get].
func (h *MemoryHandler) ChatGetAll(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	noStats := strings.EqualFold(c.QueryParam("no_stats"), "true")
	scopes, err := h.resolveEnabledScopes(botID)
	if err != nil {
		return err
	}
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
// @Router /bots/{bot_id}/memory [delete].
func (h *MemoryHandler) ChatDelete(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
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
		return c.JSON(http.StatusOK, resp)
	}

	scopes, err := h.resolveEnabledScopes(botID)
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
// @Router /bots/{bot_id}/memory/{id} [delete].
func (h *MemoryHandler) ChatDeleteOne(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
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
// @Router /bots/{bot_id}/memory/compact [post].
func (h *MemoryHandler) ChatCompact(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
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

	scopes, err := h.resolveEnabledScopes(botID)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no memory scopes found")
	}

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
// @Router /bots/{bot_id}/memory/usage [get].
func (h *MemoryHandler) ChatUsage(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}

	scopes, err := h.resolveEnabledScopes(botID)
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
// @Router /bots/{bot_id}/memory/rebuild [post].
func (h *MemoryHandler) ChatRebuild(c echo.Context) error {
	if h.memoryStore == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "memory filesystem not configured")
	}
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	fsItems, err := h.memoryStore.ReadAllMemoryFiles(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "read memory files failed: "+err.Error())
	}
	if err := h.memoryStore.SyncOverview(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "sync memory overview failed: "+err.Error())
	}

	return c.JSON(http.StatusOK, memprovider.RebuildResult{
		FsCount:       len(fsItems),
		StorageCount:  len(fsItems),
		MissingCount:  0,
		RestoredCount: 0,
	})
}

// --- helpers ---

// resolveEnabledScopes returns bot-shared namespace scope.
func (*MemoryHandler) resolveEnabledScopes(botID string) ([]namespaceScope, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "bot id is empty")
	}
	return []namespaceScope{{
		Namespace: sharedMemoryNamespace,
		ScopeID:   botID,
	}}, nil
}

// resolveWriteScope returns (scopeID, botID) for shared bot memory.
func (*MemoryHandler) resolveWriteScope(botID string) (string, string, error) {
	botID = strings.TrimSpace(botID)
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

func (*MemoryHandler) resolveBotID(c echo.Context) (string, error) {
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

func (*MemoryHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *MemoryHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID, err := h.resolveBotID(c)
	if err != nil {
		return "", err
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false}); err != nil {
		return "", err
	}
	return botID, nil
}

// NewBuiltinMemoryRuntime keeps provider architecture while using file memory backend.
func NewBuiltinMemoryRuntime(p mcpclient.Provider) any {
	if p == nil {
		return nil
	}
	return &fileMemoryRuntime{store: storefs.New(p)}
}

type fileMemoryRuntime struct {
	store *storefs.Service
}

func (r *fileMemoryRuntime) Add(ctx context.Context, req memprovider.AddRequest) (memprovider.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return memprovider.SearchResponse{}, err
	}
	text := strings.TrimSpace(req.Message)
	if text == "" && len(req.Messages) > 0 {
		parts := make([]string, 0, len(req.Messages))
		for _, m := range req.Messages {
			content := strings.TrimSpace(m.Content)
			if content == "" {
				continue
			}
			role := strings.ToUpper(strings.TrimSpace(m.Role))
			if role == "" {
				role = "MESSAGE"
			}
			parts = append(parts, "["+role+"] "+content)
		}
		text = strings.Join(parts, "\n")
	}
	if text == "" {
		return memprovider.SearchResponse{}, echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	item := memprovider.MemoryItem{
		ID:        botID + ":" + "mem_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
		Memory:    text,
		Hash:      runtimeHash(text),
		CreatedAt: now,
		UpdatedAt: now,
		BotID:     botID,
	}
	itemsToPersist := []storefs.MemoryItem{runtimeToStoreItem(item)}
	if err := r.store.PersistMemories(ctx, botID, itemsToPersist, req.Filters); err != nil {
		return memprovider.SearchResponse{}, err
	}
	return memprovider.SearchResponse{Results: []memprovider.MemoryItem{item}}, nil
}

func (r *fileMemoryRuntime) Search(ctx context.Context, req memprovider.SearchRequest) (memprovider.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return memprovider.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.SearchResponse{}, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	results := make([]memprovider.MemoryItem, 0, len(items))
	for _, item := range items {
		score := runtimeScore(query, item.Memory)
		if query != "" && score <= 0 {
			continue
		}
		item.BotID = botID
		item.Score = score
		results = append(results, runtimeFromStoreItem(item))
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].UpdatedAt > results[j].UpdatedAt
		}
		return results[i].Score > results[j].Score
	})
	if req.Limit > 0 && len(results) > req.Limit {
		results = results[:req.Limit]
	}
	return memprovider.SearchResponse{Results: results}, nil
}

func (r *fileMemoryRuntime) GetAll(ctx context.Context, req memprovider.GetAllRequest) (memprovider.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return memprovider.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.SearchResponse{}, err
	}
	for i := range items {
		items[i].BotID = botID
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return memprovider.SearchResponse{Results: runtimeFromStoreItems(items)}, nil
}

func (r *fileMemoryRuntime) Update(ctx context.Context, req memprovider.UpdateRequest) (memprovider.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return memprovider.MemoryItem{}, echo.NewHTTPError(http.StatusBadRequest, "memory_id is required")
	}
	botID := runtimeBotIDFromMemoryID(memoryID)
	if botID == "" {
		return memprovider.MemoryItem{}, echo.NewHTTPError(http.StatusBadRequest, "invalid memory_id")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.MemoryItem{}, err
	}
	var existing *memprovider.MemoryItem
	for i := range items {
		if strings.TrimSpace(items[i].ID) == memoryID {
			item := runtimeFromStoreItem(items[i])
			existing = &item
			break
		}
	}
	if existing == nil {
		return memprovider.MemoryItem{}, echo.NewHTTPError(http.StatusNotFound, "memory not found")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return memprovider.MemoryItem{}, echo.NewHTTPError(http.StatusBadRequest, "memory is required")
	}
	if err := r.store.RemoveMemories(ctx, botID, []string{memoryID}); err != nil {
		return memprovider.MemoryItem{}, err
	}
	existing.Memory = text
	existing.Hash = runtimeHash(text)
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	itemsToPersist := []storefs.MemoryItem{runtimeToStoreItem(*existing)}
	if err := r.store.PersistMemories(ctx, botID, itemsToPersist, nil); err != nil {
		return memprovider.MemoryItem{}, err
	}
	return *existing, nil
}

func (r *fileMemoryRuntime) Delete(ctx context.Context, memoryID string) (memprovider.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *fileMemoryRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (memprovider.DeleteResponse, error) {
	grouped := map[string][]string{}
	for _, id := range memoryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		botID := runtimeBotIDFromMemoryID(id)
		if botID == "" {
			continue
		}
		grouped[botID] = append(grouped[botID], id)
	}
	for botID, ids := range grouped {
		if err := r.store.RemoveMemories(ctx, botID, ids); err != nil {
			return memprovider.DeleteResponse{}, err
		}
	}
	return memprovider.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *fileMemoryRuntime) DeleteAll(ctx context.Context, req memprovider.DeleteAllRequest) (memprovider.DeleteResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return memprovider.DeleteResponse{}, err
	}
	if err := r.store.RemoveAllMemories(ctx, botID); err != nil {
		return memprovider.DeleteResponse{}, err
	}
	return memprovider.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

func (r *fileMemoryRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (memprovider.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return memprovider.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return memprovider.CompactResult{}, echo.NewHTTPError(http.StatusBadRequest, "ratio must be in range (0, 1]")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.CompactResult{}, err
	}
	before := len(items)
	if before == 0 {
		return memprovider.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []memprovider.MemoryItem{}}, nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	target := int(float64(before) * ratio)
	if target < 1 {
		target = 1
	}
	if target > before {
		target = before
	}
	keptStore := append([]storefs.MemoryItem(nil), items[:target]...)
	if err := r.store.RebuildFiles(ctx, botID, keptStore, filters); err != nil {
		return memprovider.CompactResult{}, err
	}
	kept := runtimeFromStoreItems(keptStore)
	return memprovider.CompactResult{
		BeforeCount: before,
		AfterCount:  len(kept),
		Ratio:       ratio,
		Results:     kept,
	}, nil
}

func (r *fileMemoryRuntime) Usage(ctx context.Context, filters map[string]any) (memprovider.UsageResponse, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return memprovider.UsageResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.UsageResponse{}, err
	}
	var usage memprovider.UsageResponse
	usage.Count = len(items)
	for _, item := range items {
		usage.TotalTextBytes += int64(len(item.Memory))
	}
	if usage.Count > 0 {
		usage.AvgTextBytes = usage.TotalTextBytes / int64(usage.Count)
	}
	usage.EstimatedStorageBytes = usage.TotalTextBytes
	return usage, nil
}

func runtimeBotID(botID string, filters map[string]any) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		botID = strings.TrimSpace(runtimeAny(filters, "bot_id"))
	}
	if botID == "" {
		botID = strings.TrimSpace(runtimeAny(filters, "scopeId"))
	}
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	return botID, nil
}

func runtimeBotIDFromMemoryID(memoryID string) string {
	parts := strings.SplitN(strings.TrimSpace(memoryID), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func runtimeAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func runtimeHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}

func runtimeScore(query, memory string) float64 {
	if query == "" {
		return 1
	}
	memory = strings.ToLower(memory)
	if strings.Contains(memory, query) {
		return 1
	}
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return 0
	}
	hits := 0
	for _, t := range tokens {
		if strings.Contains(memory, t) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

func runtimeToStoreItem(item memprovider.MemoryItem) storefs.MemoryItem {
	return storefs.MemoryItem{
		ID:        item.ID,
		Memory:    item.Memory,
		Hash:      item.Hash,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		Score:     item.Score,
		Metadata:  item.Metadata,
		BotID:     item.BotID,
		AgentID:   item.AgentID,
		RunID:     item.RunID,
	}
}

func runtimeFromStoreItem(item storefs.MemoryItem) memprovider.MemoryItem {
	return memprovider.MemoryItem{
		ID:        item.ID,
		Memory:    item.Memory,
		Hash:      item.Hash,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
		Score:     item.Score,
		Metadata:  item.Metadata,
		BotID:     item.BotID,
		AgentID:   item.AgentID,
		RunID:     item.RunID,
	}
}

func runtimeFromStoreItems(items []storefs.MemoryItem) []memprovider.MemoryItem {
	if len(items) == 0 {
		return []memprovider.MemoryItem{}
	}
	out := make([]memprovider.MemoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, runtimeFromStoreItem(item))
	}
	return out
}
