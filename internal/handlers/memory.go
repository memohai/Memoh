package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/settings"
)

// MemoryHandler handles memory CRUD operations scoped by bot.
type MemoryHandler struct {
	botService      *bots.Service
	accountService  *accounts.Service
	settingsService *settings.Service
	memoryRegistry  *memprovider.Registry
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

// resolveProvider returns the memory provider for a bot. An explicitly selected
// provider must be available; only bots without a selected provider may fall
// back to the builtin default.
func (h *MemoryHandler) resolveProvider(ctx context.Context, botID string) (memprovider.Provider, error) {
	if h.memoryRegistry == nil {
		return nil, nil
	}
	if h.settingsService != nil {
		botSettings, err := h.settingsService.GetBot(ctx, botID)
		if err == nil {
			providerID := strings.TrimSpace(botSettings.MemoryProviderID)
			if providerID != "" {
				p, getErr := h.memoryRegistry.Get(providerID)
				if getErr == nil {
					return p, nil
				}
				h.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", getErr))
				return nil, fmt.Errorf("configured memory provider is unavailable: %w", getErr)
			}
		}
	}
	p, err := h.memoryRegistry.Get(defaultBuiltinProviderID)
	if err != nil {
		return nil, nil
	}
	return p, nil
}

// Register registers chat-level memory routes.
func (h *MemoryHandler) Register(e *echo.Echo) {
	chatGroup := e.Group("/bots/:bot_id/memory")
	chatGroup.POST("", h.ChatAdd)
	chatGroup.POST("/search", h.ChatSearch)
	chatGroup.POST("/compact", h.ChatCompact)
	chatGroup.POST("/rebuild", h.ChatRebuild)
	chatGroup.GET("/status", h.ChatStatus)
	chatGroup.GET("", h.ChatGetAll)
	chatGroup.GET("/usage", h.ChatUsage)
	chatGroup.GET("/graph", h.ChatGraph)
	chatGroup.DELETE("", h.ChatDelete)
	chatGroup.DELETE("/:memory_id", h.ChatDeleteOne)
}

func (h *MemoryHandler) checkService(ctx context.Context, botID string) (memprovider.Provider, error) {
	p, err := h.resolveProvider(ctx, botID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	}
	if p != nil {
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
// @Success 200 {object} adapters.SearchResponse
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
	channelIdentityID, identityErr := h.requireChannelIdentityID(c)
	if identityErr != nil {
		return identityErr
	}
	req := memprovider.AddRequest{
		Message:          payload.Message,
		Messages:         payload.Messages,
		BotID:            resolvedBotID,
		RunID:            payload.RunID,
		Metadata:         memprovider.MergeMetadata(payload.Metadata, memprovider.BuildProfileMetadata("", channelIdentityID, "")),
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
// @Success 200 {object} adapters.SearchResponse
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
// @Success 200 {object} adapters.SearchResponse
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

// graphNode is one node in the memory graph view.
type graphNode struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Memory   string         `json:"memory"`
	Subject  string         `json:"subject,omitempty"`
	Topic    string         `json:"topic,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// graphEdge is one edge in the memory graph view.
type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Rel    string `json:"rel"`
}

// graphResponse is the payload for the memory graph view.
type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

// ChatGraph returns the memory graph (nodes + cross-reference edges) for the
// wiki visualization. Edges are derived from markdown links in memory bodies
// that reference another memory's subject (slug) — e.g. [[alice-profile]] or
// [background](alice-profile). This mirrors the LLM Wiki pattern: connections
// are explicit, human/LLM-readable cross-references, authored with short
// semantic slugs rather than database IDs.
//
// @Summary Get memory graph
// @Tags memory
// @Router /bots/{bot_id}/memory/graph [get].
func (h *MemoryHandler) ChatGraph(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
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
		resp, getAllErr := provider.GetAll(c.Request().Context(), memprovider.GetAllRequest{
			Filters: buildNamespaceFilters(scope.Namespace, scope.ScopeID, nil),
			NoStats: true,
		})
		if getAllErr != nil {
			continue
		}
		allResults = append(allResults, resp.Results...)
	}
	allResults = deduplicateMemoryItems(allResults)

	// Build nodes with slug = subject (or fallback). Build slug→id lookup so
	// [[slug]] references in bodies can be resolved to target node IDs.
	nodes := make([]graphNode, 0, len(allResults))
	slugToID := make(map[string]string, len(allResults))
	for _, item := range allResults {
		label := item.Memory
		if len(label) > 40 {
			label = label[:37] + "..."
		}
		subject := ""
		topic := ""
		if item.Metadata != nil {
			if s, ok := item.Metadata["subject"].(string); ok {
				subject = s
			}
			if t, ok := item.Metadata["topic"].(string); ok {
				topic = t
			}
		}
		slug := memorySlug(item.ID, subject, topic)
		slugToID[slug] = item.ID
		nodes = append(nodes, graphNode{
			ID:       item.ID,
			Label:    label,
			Memory:   item.Memory,
			Subject:  slug,
			Topic:    topic,
			Metadata: item.Metadata,
		})
	}

	// Derive edges from markdown cross-references in node bodies. A [[slug]]
	// or [label](slug) resolves to a target node via the slug→id map.
	edges := make([]graphEdge, 0)
	seen := map[string]bool{}
	for _, src := range nodes {
		for _, slugRef := range parseMemoryLinks(src.Memory) {
			targetID, ok := slugToID[strings.TrimSpace(slugRef)]
			if !ok || targetID == src.ID {
				continue
			}
			key := src.ID + "\x00" + targetID
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, graphEdge{Source: src.ID, Target: targetID, Rel: "refs"})
		}
	}

	return c.JSON(http.StatusOK, graphResponse{Nodes: nodes, Edges: edges})
}

// memorySlug returns a short, human/LLM-friendly slug for a memory node. The
// priority is: explicit subject > topic > a short hash of the id. Slugs are
// what LLMs use when writing [[slug]] cross-references.
func memorySlug(id, subject, topic string) string {
	if s := strings.TrimSpace(subject); s != "" {
		return s
	}
	if t := strings.TrimSpace(topic); t != "" {
		return t
	}
	// Fallback: last meaningful fragment of the id (e.g. "mem_2").
	if idx := strings.LastIndex(id, ":"); idx >= 0 && idx+1 < len(id) {
		return id[idx+1:]
	}
	return id
}

// Link patterns for parsing [[slug]] and [label](slug) from memory bodies.
var (
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	mdLinkRe   = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
)

// parseMemoryLinks extracts referenced slugs from a memory body. Supports
// [[slug]] and [label](slug) formats. Returns deduplicated, trimmed slugs.
func parseMemoryLinks(body string) []string {
	var slugs []string
	seen := map[string]bool{}
	collect := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		slugs = append(slugs, raw)
	}
	for _, m := range wikiLinkRe.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			collect(m[1])
		}
	}
	for _, m := range mdLinkRe.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			// Only treat as a memory link if the href is NOT a URL.
			href := strings.TrimSpace(m[1])
			if !strings.HasPrefix(href, "http") {
				collect(href)
			}
		}
	}
	return slugs
}

// @Summary Delete memories
// @Description Delete specific memories by IDs, or delete all memories if no IDs are provided
// @Tags memory
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body memoryDeletePayload false "Optional: specify memory_ids to delete; if omitted, deletes all"
// @Success 200 {object} adapters.DeleteResponse
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
// @Success 200 {object} adapters.DeleteResponse
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
// @Success 200 {object} adapters.CompactResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 501 {object} ErrorResponse
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
	capability := semanticCompactCapability(provider)
	if !capability.Semantic {
		reason := strings.TrimSpace(capability.Reason)
		if reason == "" {
			reason = "selected memory provider does not support semantic compact"
		}
		return echo.NewHTTPError(http.StatusNotImplemented, reason)
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
// @Success 200 {object} adapters.UsageResponse
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
// @Success 200 {object} adapters.RebuildResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/rebuild [post].
func (h *MemoryHandler) ChatRebuild(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}
	syncProvider, ok := provider.(memprovider.SourceSyncProvider)
	if !ok {
		return echo.NewHTTPError(http.StatusConflict, "selected memory provider does not support rebuild from markdown source")
	}
	status, err := syncProvider.Status(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !status.CanManualSync {
		return echo.NewHTTPError(http.StatusConflict, "manual sync is not available for the selected memory provider")
	}
	result, err := syncProvider.Rebuild(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

// ChatStatus godoc
// @Summary Get memory runtime status
// @Description Get the resolved memory runtime status for a bot, including index health and source counts
// @Tags memory
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} adapters.MemoryStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/memory/status [get].
func (h *MemoryHandler) ChatStatus(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	provider, checkErr := h.checkService(c.Request().Context(), botID)
	if checkErr != nil {
		return checkErr
	}
	syncProvider, ok := provider.(memprovider.SourceSyncProvider)
	if !ok {
		return echo.NewHTTPError(http.StatusConflict, "selected memory provider does not expose runtime status")
	}
	status, err := syncProvider.Status(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	status.Compact = semanticCompactCapability(provider)
	return c.JSON(http.StatusOK, status)
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

func semanticCompactCapability(provider memprovider.Provider) memprovider.MemoryCompactCapability {
	if provider == nil {
		return memprovider.MemoryCompactCapability{Reason: "memory service not available"}
	}
	semanticProvider, ok := provider.(memprovider.SemanticCompactProvider)
	if !ok {
		return memprovider.MemoryCompactCapability{Reason: "selected memory provider does not support semantic compact"}
	}
	capability := semanticProvider.SemanticCompactCapability()
	if !capability.Semantic && strings.TrimSpace(capability.Reason) == "" {
		capability.Reason = "selected memory provider does not support semantic compact"
	}
	return capability
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
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}
