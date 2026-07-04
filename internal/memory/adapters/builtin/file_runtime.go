package builtin

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

// fileRuntime implements a file-backed memory runtime. It serves markdown files
// directly as the source of truth with lexical search and no derived index. It
// is no longer a user-selectable mode: it survives as the graphRuntime's
// reliability fallback (graph_runtime.searchFileFallback) and as the
// __builtin_default__ provider when no database-backed wiki store is available
// (e.g. during bootstrap). Its lexical scorer fileRuntimeScore is also reused
// by the graph cache.
type fileRuntime struct {
	store memoryStore
}

// NewFileRuntime returns the file-only Runtime. Used for the bootstrap default
// provider when no wiki store is wired; not exposed as a memory_mode option.
func NewFileRuntime(store *storefs.Service) Runtime {
	return newFileRuntime(store)
}

func newFileRuntime(store memoryStore) *fileRuntime {
	if store == nil {
		return nil
	}
	return &fileRuntime{store: store}
}

func (r *fileRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	text := runtimeText(req.Message, req.Messages)
	if text == "" {
		return adapters.SearchResponse{}, errors.New("message is required")
	}
	now := time.Now().UTC()
	item := adapters.MemoryItem{
		ID:        runtimeMemoryID(botID, now),
		Memory:    text,
		Hash:      runtimeHash(text),
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
		Metadata:  req.Metadata,
		BotID:     botID,
	}
	itemsToPersist := []storefs.MemoryItem{storeItemFromMemoryItem(item)}
	if err := r.store.PersistMemories(ctx, botID, itemsToPersist, req.Filters); err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{item}, RetrievalMode: "file"}, nil
}

func (r *fileRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	results := make([]adapters.MemoryItem, 0, len(items))
	for _, item := range items {
		score := fileRuntimeScore(query, item.Memory)
		if query != "" && score <= 0 {
			continue
		}
		item.BotID = botID
		item.Score = score
		results = append(results, memoryItemFromStore(item))
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
	return adapters.SearchResponse{Results: results, RetrievalMode: "file"}, nil
}

func (r *fileRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	for i := range items {
		items[i].BotID = botID
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return adapters.SearchResponse{Results: memoryItemsFromStore(items), RetrievalMode: "file"}, nil
}

func (r *fileRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("memory_id is required")
	}
	botID := runtimeBotIDFromMemoryID(memoryID)
	if botID == "" {
		return adapters.MemoryItem{}, errors.New("invalid memory_id")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryItem{}, err
	}
	var existing *storefs.MemoryItem
	for i := range items {
		if strings.TrimSpace(items[i].ID) == memoryID {
			item := items[i]
			existing = &item
			break
		}
	}
	if existing == nil {
		return adapters.MemoryItem{}, errors.New("memory not found")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return adapters.MemoryItem{}, errors.New("memory is required")
	}
	if err := r.store.RemoveMemories(ctx, botID, []string{memoryID}); err != nil {
		return adapters.MemoryItem{}, err
	}
	existing.Memory = text
	existing.Hash = runtimeHash(text)
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	itemsToPersist := []storefs.MemoryItem{*existing}
	if err := r.store.PersistMemories(ctx, botID, itemsToPersist, nil); err != nil {
		return adapters.MemoryItem{}, err
	}
	item := memoryItemFromStore(*existing)
	item.BotID = botID
	return item, nil
}

func (r *fileRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *fileRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
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
			return adapters.DeleteResponse{}, err
		}
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *fileRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.store.RemoveAllMemories(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

func (r *fileRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (adapters.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return adapters.CompactResult{}, errors.New("ratio must be in range (0, 1]")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	before := len(items)
	if before == 0 {
		return adapters.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []adapters.MemoryItem{}}, nil
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
		return adapters.CompactResult{}, err
	}
	kept := memoryItemsFromStore(keptStore)
	return adapters.CompactResult{
		BeforeCount: before,
		AfterCount:  len(kept),
		Ratio:       ratio,
		Results:     kept,
	}, nil
}

func (r *fileRuntime) CompactWithLLM(ctx context.Context, filters map[string]any, ratio float64, decayDays int, llm adapters.LLM) (adapters.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return adapters.CompactResult{}, errors.New("ratio must be in range (0, 1]")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	before := len(items)
	if before == 0 {
		return adapters.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []adapters.MemoryItem{}}, nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	compactedStore, archivedStore, err := compactStoreItemsWithLLM(ctx, botID, items, ratio, decayDays, llm)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if err := r.store.ArchiveAndRebuildFiles(ctx, botID, compactedStore, archivedStore, filters); err != nil {
		return adapters.CompactResult{}, err
	}
	compacted := memoryItemsFromStore(compactedStore)
	return adapters.CompactResult{
		BeforeCount: before,
		AfterCount:  len(compacted),
		Ratio:       ratio,
		Results:     compacted,
	}, nil
}

func (r *fileRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.UsageResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.UsageResponse{}, err
	}
	var usage adapters.UsageResponse
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

// Mode returns the internal identifier for the file runtime. It is used as a
// fallback component (graphRuntime degrades to it when the wiki store is
// unavailable) and as the __builtin_default__ provider when no DB is wired.
// It is no longer a user-selectable memory_mode.
func (*fileRuntime) Mode() string {
	return "file"
}

func (r *fileRuntime) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
	fileCount, err := r.store.CountMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	return adapters.MemoryStatusResponse{
		ProviderType:      BuiltinType,
		MemoryMode:        "file",
		CanManualSync:     false,
		SourceDir:         path.Join(config.DefaultDataMount, "memory"),
		OverviewPath:      path.Join(config.DefaultDataMount, "MEMORY.md"),
		MarkdownFileCount: fileCount,
		SourceCount:       len(items),
	}, nil
}

func (r *fileRuntime) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	if err := r.store.SyncOverview(ctx, botID); err != nil {
		return adapters.RebuildResult{}, err
	}
	return adapters.RebuildResult{
		FsCount:      len(items),
		StorageCount: len(items),
	}, nil
}

func fileRuntimeScore(query, memory string) float64 {
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
	for _, token := range tokens {
		if strings.Contains(memory, token) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}
