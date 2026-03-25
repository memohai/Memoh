package builtin

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	"github.com/memohai/memoh/internal/memory/sparse"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

type sparseEncoder interface {
	EncodeDocument(ctx context.Context, text string) (*sparse.SparseVector, error)
	EncodeDocuments(ctx context.Context, texts []string) ([]sparse.SparseVector, error)
	EncodeQuery(ctx context.Context, text string) (*sparse.SparseVector, error)
	Health(ctx context.Context) error
}

type sparseIndex interface {
	CollectionName() string
	CollectionExists(ctx context.Context) (bool, error)
	EnsureCollection(ctx context.Context) error
	Upsert(ctx context.Context, id string, vec qdrantclient.SparseVector, payload map[string]string) error
	Search(ctx context.Context, vec qdrantclient.SparseVector, botID string, limit int) ([]qdrantclient.SearchResult, error)
	Scroll(ctx context.Context, botID string, limit int) ([]qdrantclient.SearchResult, error)
	Count(ctx context.Context, botID string) (int, error)
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByBotID(ctx context.Context, botID string) error
}

type sparseMemoryStore interface {
	PersistMemories(ctx context.Context, botID string, items []storefs.MemoryItem, filters map[string]any) error
	ReadAllMemoryFiles(ctx context.Context, botID string) ([]storefs.MemoryItem, error)
	RemoveMemories(ctx context.Context, botID string, ids []string) error
	RemoveAllMemories(ctx context.Context, botID string) error
	RebuildFiles(ctx context.Context, botID string, items []storefs.MemoryItem, filters map[string]any) error
	SyncOverview(ctx context.Context, botID string) error
	CountMemoryFiles(ctx context.Context, botID string) (int, error)
}

// sparseRuntime implements memoryRuntime with markdown files as the source of
// truth and Qdrant as a derived sparse index used for retrieval.
type sparseRuntime struct {
	qdrant  sparseIndex
	encoder sparseEncoder
	store   sparseMemoryStore
}

const (
	sparseExplainTopKLimit = 24
)

func newSparseRuntime(qdrantHost string, qdrantPort int, qdrantAPIKey, collection, encoderBaseURL string, store *storefs.Service) (*sparseRuntime, error) {
	if strings.TrimSpace(qdrantHost) == "" {
		return nil, errors.New("sparse runtime: qdrant host is required")
	}
	if strings.TrimSpace(encoderBaseURL) == "" {
		return nil, errors.New("sparse runtime: sparse.base_url is required")
	}
	if store == nil {
		return nil, errors.New("sparse runtime: memory store is required")
	}
	qClient, err := qdrantclient.NewClient(qdrantHost, qdrantPort, qdrantAPIKey, collection)
	if err != nil {
		return nil, fmt.Errorf("sparse runtime: %w", err)
	}
	return &sparseRuntime{
		qdrant:  qClient,
		encoder: sparse.NewClient(encoderBaseURL),
		store:   store,
	}, nil
}

func (r *sparseRuntime) ensureCollection(ctx context.Context) error {
	return r.qdrant.EnsureCollection(ctx)
}

func (*sparseRuntime) Mode() string {
	return string(ModeSparse)
}

func (r *sparseRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	text := runtimeText(req.Message, req.Messages)
	if text == "" {
		return adapters.SearchResponse{}, errors.New("sparse runtime: message is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	item := adapters.MemoryItem{
		ID:        runtimeMemoryID(botID, time.Now().UTC()),
		Memory:    text,
		Hash:      runtimeHash(text),
		Metadata:  req.Metadata,
		BotID:     botID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.store.PersistMemories(ctx, botID, []storefs.MemoryItem{storeItemFromMemoryItem(item)}, req.Filters); err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.upsertSourceItems(ctx, botID, []storefs.MemoryItem{storeItemFromMemoryItem(item)}); err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{item}}, nil
}

func (r *sparseRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.SearchResponse{}, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	vec, err := r.encoder.EncodeQuery(ctx, req.Query)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("sparse encode query: %w", err)
	}
	results, err := r.qdrant.Search(ctx, qdrantclient.SparseVector{
		Indices: vec.Indices,
		Values:  vec.Values,
	}, botID, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := make([]adapters.MemoryItem, 0, len(results))
	for _, r := range results {
		items = append(items, resultToItem(r))
	}
	return adapters.SearchResponse{Results: items}, nil
}

func (r *sparseRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	result := make([]adapters.MemoryItem, 0, len(items))
	for _, item := range items {
		mem := memoryItemFromStore(item)
		mem.BotID = botID
		result = append(result, mem)
	}
	r.populateExplainStats(ctx, sparseMemoryItemPointers(result))
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	if req.Limit > 0 && len(result) > req.Limit {
		result = result[:req.Limit]
	}
	return adapters.SearchResponse{Results: result}, nil
}

func (r *sparseRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("sparse runtime: memory_id is required")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return adapters.MemoryItem{}, errors.New("sparse runtime: memory is required")
	}
	botID := runtimeBotIDFromMemoryID(memoryID)
	if botID == "" {
		return adapters.MemoryItem{}, errors.New("sparse runtime: invalid memory_id")
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
		return adapters.MemoryItem{}, errors.New("sparse runtime: memory not found")
	}
	existing.Memory = text
	existing.Hash = runtimeHash(text)
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := r.store.PersistMemories(ctx, botID, []storefs.MemoryItem{*existing}, nil); err != nil {
		return adapters.MemoryItem{}, err
	}
	if err := r.upsertSourceItems(ctx, botID, []storefs.MemoryItem{*existing}); err != nil {
		return adapters.MemoryItem{}, err
	}
	item := memoryItemFromStore(*existing)
	item.BotID = botID
	return item, nil
}

func (r *sparseRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *sparseRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	grouped := map[string][]string{}
	pointIDs := make([]string, 0, len(memoryIDs))
	for _, rawID := range memoryIDs {
		memoryID := strings.TrimSpace(rawID)
		if memoryID == "" {
			continue
		}
		botID := runtimeBotIDFromMemoryID(memoryID)
		if botID == "" {
			continue
		}
		grouped[botID] = append(grouped[botID], memoryID)
		pointIDs = append(pointIDs, runtimePointID(botID, memoryID))
	}
	for botID, ids := range grouped {
		if err := r.store.RemoveMemories(ctx, botID, ids); err != nil {
			return adapters.DeleteResponse{}, err
		}
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.qdrant.DeleteByIDs(ctx, pointIDs); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *sparseRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.store.RemoveAllMemories(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.qdrant.DeleteByBotID(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

func (r *sparseRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (adapters.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	all, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	before := len(all)
	if before == 0 {
		return adapters.CompactResult{Ratio: ratio}, nil
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt > all[j].UpdatedAt
	})
	target := int(float64(before) * ratio)
	if target < 1 {
		target = 1
	}
	if target > before {
		target = before
	}
	keptStore := append([]storefs.MemoryItem(nil), all[:target]...)
	if err := r.store.RebuildFiles(ctx, botID, keptStore, filters); err != nil {
		return adapters.CompactResult{}, err
	}
	if _, err := r.Rebuild(ctx, botID); err != nil {
		return adapters.CompactResult{}, err
	}
	kept := make([]adapters.MemoryItem, 0, len(keptStore))
	for _, item := range keptStore {
		kept = append(kept, memoryItemFromStore(item))
	}
	return adapters.CompactResult{
		BeforeCount: before,
		AfterCount:  len(kept),
		Ratio:       ratio,
		Results:     kept,
	}, nil
}

func (r *sparseRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
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

func (r *sparseRuntime) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
	fileCount, err := r.store.CountMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.MemoryStatusResponse{}, err
	}
	status := adapters.MemoryStatusResponse{
		ProviderType:      BuiltinType,
		MemoryMode:        string(ModeSparse),
		CanManualSync:     true,
		SourceDir:         path.Join(config.DefaultDataMount, "memory"),
		OverviewPath:      path.Join(config.DefaultDataMount, "MEMORY.md"),
		MarkdownFileCount: fileCount,
		SourceCount:       len(items),
		QdrantCollection:  r.qdrant.CollectionName(),
	}
	if err := r.encoder.Health(ctx); err != nil {
		status.Encoder.Error = err.Error()
	} else {
		status.Encoder.OK = true
	}
	exists, err := r.qdrant.CollectionExists(ctx)
	if err != nil {
		status.Qdrant.Error = err.Error()
		return status, nil
	}
	status.Qdrant.OK = true
	if exists {
		count, err := r.qdrant.Count(ctx, botID)
		if err != nil {
			status.Qdrant.OK = false
			status.Qdrant.Error = err.Error()
			return status, nil
		}
		status.IndexedCount = count
	}
	return status, nil
}

func (r *sparseRuntime) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	if err := r.store.SyncOverview(ctx, botID); err != nil {
		return adapters.RebuildResult{}, err
	}
	return r.syncSourceItems(ctx, botID, items)
}

// --- helpers ---

func (r *sparseRuntime) syncSourceItems(ctx context.Context, botID string, items []storefs.MemoryItem) (adapters.RebuildResult, error) {
	if err := r.ensureCollection(ctx); err != nil {
		return adapters.RebuildResult{}, err
	}
	existing, err := r.qdrant.Scroll(ctx, botID, 10000)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	existingBySource := make(map[string]qdrantclient.SearchResult, len(existing))
	for _, item := range existing {
		sourceID := strings.TrimSpace(item.Payload["source_entry_id"])
		if sourceID == "" {
			sourceID = strings.TrimSpace(item.ID)
		}
		if sourceID == "" {
			continue
		}
		existingBySource[sourceID] = item
	}
	canonical := make([]storefs.MemoryItem, 0, len(items))
	sourceIDs := make(map[string]struct{}, len(items))
	toUpsert := make([]storefs.MemoryItem, 0, len(items))
	missingCount := 0
	restoredCount := 0
	for _, item := range items {
		item = canonicalStoreItem(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		canonical = append(canonical, item)
		sourceIDs[item.ID] = struct{}{}
		payload := runtimePayload(botID, item)
		existingItem, ok := existingBySource[item.ID]
		if !ok {
			missingCount++
			restoredCount++
			toUpsert = append(toUpsert, item)
			continue
		}
		if !payloadMatches(existingItem.Payload, payload) {
			restoredCount++
			toUpsert = append(toUpsert, item)
		}
	}
	stalePointIDs := make([]string, 0)
	for _, item := range existing {
		sourceID := strings.TrimSpace(item.Payload["source_entry_id"])
		if sourceID == "" {
			sourceID = strings.TrimSpace(item.ID)
		}
		if _, ok := sourceIDs[sourceID]; ok {
			continue
		}
		if strings.TrimSpace(item.ID) != "" {
			stalePointIDs = append(stalePointIDs, item.ID)
		}
	}
	if len(stalePointIDs) > 0 {
		if err := r.qdrant.DeleteByIDs(ctx, stalePointIDs); err != nil {
			return adapters.RebuildResult{}, err
		}
	}
	if err := r.upsertSourceItems(ctx, botID, toUpsert); err != nil {
		return adapters.RebuildResult{}, err
	}
	count, err := r.qdrant.Count(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	return adapters.RebuildResult{
		FsCount:       len(canonical),
		StorageCount:  count,
		MissingCount:  missingCount,
		RestoredCount: restoredCount,
	}, nil
}

func (r *sparseRuntime) upsertSourceItems(ctx context.Context, botID string, items []storefs.MemoryItem) error {
	if len(items) == 0 {
		return nil
	}
	if err := r.ensureCollection(ctx); err != nil {
		return err
	}
	texts := make([]string, 0, len(items))
	canonical := make([]storefs.MemoryItem, 0, len(items))
	for _, item := range items {
		item = canonicalStoreItem(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		canonical = append(canonical, item)
		texts = append(texts, item.Memory)
	}
	if len(canonical) == 0 {
		return nil
	}
	vectors, err := r.encoder.EncodeDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("sparse encode documents: %w", err)
	}
	if len(vectors) != len(canonical) {
		return fmt.Errorf("sparse encode documents: expected %d vectors, got %d", len(canonical), len(vectors))
	}
	for i, item := range canonical {
		vec := vectors[i]
		if err := r.qdrant.Upsert(ctx, runtimePointID(botID, item.ID), qdrantclient.SparseVector{
			Indices: vec.Indices,
			Values:  vec.Values,
		}, runtimePayload(botID, item)); err != nil {
			return err
		}
	}
	return nil
}

func (r *sparseRuntime) populateExplainStats(ctx context.Context, items []*adapters.MemoryItem) {
	if len(items) == 0 {
		return
	}
	texts := make([]string, 0, len(items))
	targets := make([]*adapters.MemoryItem, 0, len(items))
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.Memory) == "" {
			continue
		}
		texts = append(texts, item.Memory)
		targets = append(targets, item)
	}
	if len(texts) == 0 {
		return
	}
	vectors, err := r.encoder.EncodeDocuments(ctx, texts)
	if err != nil || len(vectors) != len(targets) {
		return
	}
	for i := range targets {
		topK, cdf := sparseExplainStats(vectors[i])
		targets[i].TopKBuckets = topK
		targets[i].CDFCurve = cdf
	}
}

func sparseExplainStats(vec sparse.SparseVector) ([]adapters.TopKBucket, []adapters.CDFPoint) {
	type pair struct {
		index uint32
		value float32
	}
	pairs := make([]pair, 0, len(vec.Values))
	for i, value := range vec.Values {
		if i >= len(vec.Indices) || value <= 0 {
			continue
		}
		pairs = append(pairs, pair{index: vec.Indices[i], value: value})
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].value == pairs[j].value {
			return pairs[i].index < pairs[j].index
		}
		return pairs[i].value > pairs[j].value
	})
	topN := len(pairs)
	if topN > sparseExplainTopKLimit {
		topN = sparseExplainTopKLimit
	}
	topK := make([]adapters.TopKBucket, 0, topN)
	total := 0.0
	for _, pair := range pairs {
		total += float64(pair.value)
	}
	for _, pair := range pairs[:topN] {
		topK = append(topK, adapters.TopKBucket{
			Index: pair.index,
			Value: pair.value,
		})
	}
	cdf := make([]adapters.CDFPoint, 0, len(pairs))
	if total <= 0 {
		return topK, cdf
	}
	running := 0.0
	for i, pair := range pairs {
		running += float64(pair.value)
		cdf = append(cdf, adapters.CDFPoint{
			K:          i + 1,
			Cumulative: running / total,
		})
	}
	return topK, cdf
}

func sparseMemoryItemPointers(items []adapters.MemoryItem) []*adapters.MemoryItem {
	if len(items) == 0 {
		return nil
	}
	pointers := make([]*adapters.MemoryItem, 0, len(items))
	for i := range items {
		pointers = append(pointers, &items[i])
	}
	return pointers
}
