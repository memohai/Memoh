package mem0

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/mcp"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

const (
	Mem0Type = "mem0"

	mem0ToolSearchMemory = "search_memory"
	mem0DefaultLimit     = 8
	mem0MaxLimit         = 50
	mem0ContextMaxItems  = 8
	mem0ContextMaxChars  = 220

	mem0SyncMetadataKeySourceEntryID = "source_entry_id"
	mem0SyncMetadataKeySourceHash    = "source_hash"
	mem0SyncMetadataKeySourceBotID   = "source_bot_id"
	mem0SyncMetadataKeySourceManaged = "source_managed"
)

// Mem0Provider implements adapters.Provider by delegating to the Mem0 SaaS API.
type Mem0Provider struct {
	client *mem0Client
	logger *slog.Logger
	store  *storefs.Service
}

func NewMem0Provider(log *slog.Logger, config map[string]any, store *storefs.Service) (*Mem0Provider, error) {
	if log == nil {
		log = slog.Default()
	}
	c, err := newMem0Client(config)
	if err != nil {
		return nil, err
	}
	return &Mem0Provider{
		client: c,
		logger: log.With(slog.String("provider", Mem0Type)),
		store:  store,
	}, nil
}

func (*Mem0Provider) Type() string { return Mem0Type }

// --- Conversation Hooks ---

func (p *Mem0Provider) OnBeforeChat(ctx context.Context, req adapters.BeforeChatRequest) (*adapters.BeforeChatResult, error) {
	query := strings.TrimSpace(req.Query)
	botID := strings.TrimSpace(req.BotID)
	if query == "" || botID == "" {
		return nil, nil
	}
	memories, err := p.searchMemories(ctx, query, botID, mem0ContextMaxItems)
	if err != nil {
		p.logger.Warn("mem0 search for context failed", slog.Any("error", err))
		return nil, nil
	}
	if len(memories) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\nRelevant memory context (use when helpful):\n")
	for i, mem := range memories {
		if i >= mem0ContextMaxItems {
			break
		}
		text := strings.TrimSpace(mem.Memory)
		if text == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(adapters.TruncateSnippet(text, mem0ContextMaxChars))
		sb.WriteString("\n")
	}
	sb.WriteString("</memory-context>")
	return &adapters.BeforeChatResult{ContextText: sb.String()}, nil
}

func (p *Mem0Provider) OnAfterChat(ctx context.Context, req adapters.AfterChatRequest) error {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" || len(req.Messages) == 0 {
		return nil
	}
	_, err := p.client.Add(ctx, mem0AddRequest{
		Messages: req.Messages,
		AgentID:  botID,
	})
	if err != nil {
		p.logger.Warn("mem0 store memory failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (*Mem0Provider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return []mcp.ToolDescriptor{
		{
			Name:        mem0ToolSearchMemory,
			Description: "Search for memories relevant to the current chat",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The query to search memories",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of memory results",
					},
				},
				"required": []string{"query"},
			},
		},
	}, nil
}

func (p *Mem0Provider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != mem0ToolSearchMemory {
		return nil, mcp.ErrToolNotFound
	}
	query := mcp.StringArg(arguments, "query")
	if query == "" {
		return mcp.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcp.BuildToolErrorResult("bot_id is required"), nil
	}
	limit := mem0DefaultLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = mem0DefaultLimit
	}
	if limit > mem0MaxLimit {
		limit = mem0MaxLimit
	}

	memories, err := p.searchMemories(ctx, query, botID, limit)
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}

	results := make([]map[string]any, 0, len(memories))
	for _, mem := range memories {
		results = append(results, map[string]any{
			"id":     mem.ID,
			"memory": mem.Memory,
			"score":  mem.Score,
		})
	}
	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

// --- CRUD ---

func (p *Mem0Provider) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	agentID := mem0ScopeID(req.BotID, req.AgentID)
	if agentID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id or agent_id is required")
	}
	addReq := mem0AddRequest{
		AgentID: agentID,
		RunID:   req.RunID,
		Infer:   req.Infer,
	}
	if req.Message != "" {
		addReq.Messages = []adapters.Message{{Role: "user", Content: req.Message}}
	} else {
		addReq.Messages = req.Messages
	}
	if req.Metadata != nil {
		addReq.Metadata = req.Metadata
	}
	memories, err := p.client.Add(ctx, addReq)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: mem0ToItems(memories)}, nil
}

func (p *Mem0Provider) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	agentID := mem0ScopeID(req.BotID, req.AgentID)
	if agentID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id or agent_id is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = mem0DefaultLimit
	} else if limit > mem0MaxLimit {
		limit = mem0MaxLimit
	}
	memories, err := p.client.Search(ctx, mem0SearchRequest{
		Query:   req.Query,
		TopK:    limit,
		Filters: mem0AgentFilter(agentID, req.RunID),
	})
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := mem0ToItems(memories)
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	return adapters.SearchResponse{Results: adapters.DeduplicateItems(items)}, nil
}

func (p *Mem0Provider) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	agentID := mem0ScopeID(req.BotID, req.AgentID)
	if agentID == "" {
		return adapters.SearchResponse{}, errors.New("bot_id or agent_id is required")
	}
	getReq := mem0GetAllRequest{
		Filters: mem0AgentFilter(agentID, req.RunID),
	}
	if req.Limit > 0 {
		getReq.PageSize = req.Limit
	}
	memories, err := p.client.GetAll(ctx, getReq)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := mem0ToItems(memories)
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return adapters.SearchResponse{Results: items}, nil
}

func (p *Mem0Provider) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("memory_id is required")
	}
	mem, err := p.client.Update(ctx, memoryID, req.Memory, nil)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	return mem0ToItem(*mem), nil
}

func (p *Mem0Provider) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	if err := p.client.Delete(ctx, strings.TrimSpace(memoryID)); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memory deleted successfully"}, nil
}

func (p *Mem0Provider) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	if err := p.client.BatchDelete(ctx, memoryIDs); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully"}, nil
}

func (p *Mem0Provider) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	agentID := mem0ScopeID(req.BotID, req.AgentID)
	if agentID == "" {
		return adapters.DeleteResponse{}, errors.New("bot_id or agent_id is required")
	}
	if err := p.client.DeleteAll(ctx, agentID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted"}, nil
}

// --- Lifecycle ---

func (*Mem0Provider) Compact(_ context.Context, _ map[string]any, _ float64, _ int) (adapters.CompactResult, error) {
	return adapters.CompactResult{}, errors.New("compact is not supported by mem0 provider")
}

func (*Mem0Provider) Usage(_ context.Context, _ map[string]any) (adapters.UsageResponse, error) {
	return adapters.UsageResponse{}, errors.New("usage is not supported by mem0 provider")
}

func (p *Mem0Provider) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
	status := adapters.MemoryStatusResponse{
		ProviderType:  Mem0Type,
		CanManualSync: p.store != nil,
		SourceDir:     path.Join(config.DefaultDataMount, "memory"),
		OverviewPath:  path.Join(config.DefaultDataMount, "MEMORY.md"),
	}
	if p.store == nil {
		return status, nil
	}
	fileCount, err := p.store.CountMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	items, err := p.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	status.MarkdownFileCount = fileCount
	status.SourceCount = len(mem0CanonicalStoreItems(items))
	remote, err := p.client.ListAllByAgent(ctx, strings.TrimSpace(botID))
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	status.IndexedCount = len(remote)
	return status, nil
}

func (p *Mem0Provider) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	if p.store == nil {
		return adapters.RebuildResult{}, errors.New("memory filesystem not configured")
	}
	items, err := p.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	if err := p.store.SyncOverview(ctx, botID); err != nil {
		return adapters.RebuildResult{}, err
	}
	return p.syncSourceItems(ctx, strings.TrimSpace(botID), items)
}

// --- helpers ---

func mem0ToItems(memories []mem0Memory) []adapters.MemoryItem {
	items := make([]adapters.MemoryItem, 0, len(memories))
	for _, m := range memories {
		items = append(items, mem0ToItem(m))
	}
	return items
}

func mem0ToItem(m mem0Memory) adapters.MemoryItem {
	return adapters.MemoryItem{
		ID:        m.ID,
		Memory:    m.Memory,
		Hash:      m.Hash,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		Score:     m.Score,
		Metadata:  m.Metadata,
		BotID:     m.AgentID,
		AgentID:   m.AgentID,
		RunID:     m.RunID,
	}
}

func (p *Mem0Provider) syncSourceItems(ctx context.Context, botID string, items []storefs.MemoryItem) (adapters.RebuildResult, error) {
	canonical := mem0CanonicalStoreItems(items)
	existing, err := p.client.ListAllByAgent(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	existingBySource := make(map[string]mem0Memory, len(existing))
	for _, memory := range existing {
		sourceID := mem0SourceEntryID(memory.Metadata)
		if sourceID == "" {
			continue
		}
		existingBySource[sourceID] = memory
	}
	sourceIDs := make(map[string]struct{}, len(canonical))
	missingCount := 0
	restoredCount := 0
	for _, item := range canonical {
		sourceIDs[item.ID] = struct{}{}
		metadata := mem0SourceMetadata(botID, item)
		existingMemory, ok := existingBySource[item.ID]
		if !ok {
			missingCount++
			restoredCount++
			if _, err := p.client.Add(ctx, mem0AddRequest{
				Messages:  []adapters.Message{{Role: "system", Content: item.Memory}},
				AgentID:   botID,
				RunID:     item.RunID,
				Metadata:  metadata,
				Infer:     mem0BoolPtr(false),
				AsyncMode: mem0BoolPtr(false),
			}); err != nil {
				return adapters.RebuildResult{}, err
			}
			continue
		}
		if mem0SourceMemoryMatches(existingMemory, item, botID) {
			continue
		}
		restoredCount++
		if _, err := p.client.Update(ctx, existingMemory.ID, item.Memory, metadata); err != nil {
			return adapters.RebuildResult{}, err
		}
	}
	staleIDs := make([]string, 0)
	for _, memory := range existing {
		sourceID := mem0SourceEntryID(memory.Metadata)
		if sourceID == "" {
			continue
		}
		if _, ok := sourceIDs[sourceID]; ok {
			continue
		}
		if strings.TrimSpace(memory.ID) != "" {
			staleIDs = append(staleIDs, memory.ID)
		}
	}
	for len(staleIDs) > 0 {
		chunkSize := mem0BatchDeleteMaxSize
		if len(staleIDs) < chunkSize {
			chunkSize = len(staleIDs)
		}
		if err := p.client.BatchDelete(ctx, staleIDs[:chunkSize]); err != nil {
			return adapters.RebuildResult{}, err
		}
		staleIDs = staleIDs[chunkSize:]
	}
	remote, err := p.client.ListAllByAgent(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	return adapters.RebuildResult{
		FsCount:       len(canonical),
		StorageCount:  len(remote),
		MissingCount:  missingCount,
		RestoredCount: restoredCount,
	}, nil
}

func (p *Mem0Provider) searchMemories(ctx context.Context, query, agentID string, limit int) ([]adapters.MemoryItem, error) {
	memories, err := p.client.Search(ctx, mem0SearchRequest{
		Query:   query,
		TopK:    limit,
		Filters: mem0AgentFilter(agentID, ""),
	})
	if err != nil {
		return nil, err
	}
	items := mem0ToItems(memories)
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	return adapters.DeduplicateItems(items), nil
}

func mem0ScopeID(botID, agentID string) string {
	if value := strings.TrimSpace(botID); value != "" {
		return value
	}
	return strings.TrimSpace(agentID)
}

func mem0AgentFilter(agentID, runID string) map[string]any {
	filter := map[string]any{
		"agent_id": strings.TrimSpace(agentID),
	}
	if strings.TrimSpace(runID) != "" {
		filter["run_id"] = strings.TrimSpace(runID)
	}
	return filter
}

func mem0CanonicalStoreItems(items []storefs.MemoryItem) []storefs.MemoryItem {
	result := make([]storefs.MemoryItem, 0, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Memory = strings.TrimSpace(item.Memory)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func mem0SourceMetadata(botID string, item storefs.MemoryItem) map[string]any {
	metadata := make(map[string]any, len(item.Metadata)+4)
	for key, value := range item.Metadata {
		metadata[key] = value
	}
	metadata[mem0SyncMetadataKeySourceEntryID] = item.ID
	metadata[mem0SyncMetadataKeySourceHash] = strings.TrimSpace(item.Hash)
	metadata[mem0SyncMetadataKeySourceBotID] = strings.TrimSpace(botID)
	metadata[mem0SyncMetadataKeySourceManaged] = true
	return metadata
}

func mem0SourceEntryID(metadata map[string]any) string {
	return strings.TrimSpace(mem0MetadataString(metadata, mem0SyncMetadataKeySourceEntryID))
}

func mem0SourceMemoryMatches(memory mem0Memory, item storefs.MemoryItem, botID string) bool {
	if strings.TrimSpace(memory.Memory) != strings.TrimSpace(item.Memory) {
		return false
	}
	metadata := memory.Metadata
	if mem0SourceEntryID(metadata) != strings.TrimSpace(item.ID) {
		return false
	}
	if mem0MetadataString(metadata, mem0SyncMetadataKeySourceHash) != strings.TrimSpace(item.Hash) {
		return false
	}
	if mem0MetadataString(metadata, mem0SyncMetadataKeySourceBotID) != strings.TrimSpace(botID) {
		return false
	}
	return mem0MetadataBool(metadata, mem0SyncMetadataKeySourceManaged)
}

func mem0MetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(strings.Trim(fmt.Sprintf("%v", raw), "\""))
}

func mem0MetadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return false
	}
	value, ok := raw.(bool)
	return ok && value
}

func mem0BoolPtr(v bool) *bool {
	return &v
}
