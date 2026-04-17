package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/models"
)

const denseEmbedTimeout = models.DefaultProviderRequestTimeout

type denseRuntime struct {
	qdrant     *qdrantclient.Client
	store      *storefs.Service
	embedModel *sdk.EmbeddingModel
	dimensions int
	collection string
}

type denseModelSpec struct {
	modelID    string
	clientType string
	baseURL    string
	apiKey     string
	dimensions int
}

func newDenseRuntime(providerConfig map[string]any, queries *dbsqlc.Queries, cfg config.Config, store *storefs.Service) (*denseRuntime, error) {
	if queries == nil {
		return nil, errors.New("dense runtime: queries are required")
	}
	if store == nil {
		return nil, errors.New("dense runtime: memory store is required")
	}

	modelRef := strings.TrimSpace(adapters.StringFromConfig(providerConfig, "embedding_model_id"))
	if modelRef == "" {
		return nil, errors.New("dense runtime: embedding_model_id is required")
	}

	spec, err := resolveDenseEmbeddingModel(context.Background(), queries, modelRef)
	if err != nil {
		return nil, err
	}

	host, port := parseQdrantHostPort(cfg.Qdrant.BaseURL)
	if host == "" {
		host = "localhost"
	}
	if port == 0 {
		port = 6334
	}
	collection := adapters.StringFromConfig(providerConfig, "qdrant_collection")
	if strings.TrimSpace(collection) == "" {
		collection = "memory_dense"
	}
	qClient, err := qdrantclient.NewClient(host, port, cfg.Qdrant.APIKey, collection)
	if err != nil {
		return nil, fmt.Errorf("dense runtime: %w", err)
	}

	embedModel := models.NewSDKEmbeddingModel(spec.clientType, spec.baseURL, spec.apiKey, spec.modelID, denseEmbedTimeout, nil)

	return &denseRuntime{
		qdrant:     qClient,
		store:      store,
		embedModel: embedModel,
		dimensions: spec.dimensions,
		collection: collection,
	}, nil
}

// --- embedder helpers using Twilight SDK ---

func (r *denseRuntime) embedQuery(ctx context.Context, text string) ([]float32, error) {
	client := sdk.NewClient()
	vec, err := client.Embed(ctx, text, sdk.WithEmbeddingModel(r.embedModel))
	if err != nil {
		return nil, fmt.Errorf("dense embed query: %w", err)
	}
	return float64sToFloat32s(vec), nil
}

func (r *denseRuntime) embedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	client := sdk.NewClient()
	result, err := client.EmbedMany(ctx, texts, sdk.WithEmbeddingModel(r.embedModel))
	if err != nil {
		return nil, fmt.Errorf("dense embed documents: %w", err)
	}
	out := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		out[i] = float64sToFloat32s(emb)
	}
	return out, nil
}

// embedHealth performs a minimal smoke-test embedding to verify that the
// configured embedding model is reachable and functional.
func (r *denseRuntime) embedHealth(ctx context.Context) error {
	client := sdk.NewClient()
	_, err := client.Embed(ctx, "health", sdk.WithEmbeddingModel(r.embedModel))
	if err != nil {
		return fmt.Errorf("dense embedding health check failed: %w", err)
	}
	return nil
}

func float64sToFloat32s(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

// --- memoryRuntime interface ---

func (r *denseRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	text := runtimeText(req.Message, req.Messages)
	if text == "" {
		return adapters.SearchResponse{}, errors.New("dense runtime: message is required")
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

func (r *denseRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.qdrant.EnsureDenseCollection(ctx, r.dimensions); err != nil {
		return adapters.SearchResponse{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	vec, err := r.embedQuery(ctx, req.Query)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	results, err := r.qdrant.SearchDense(ctx, qdrantclient.DenseVector{Values: vec}, botID, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := make([]adapters.MemoryItem, 0, len(results))
	for _, result := range results {
		items = append(items, resultToItem(result))
	}
	return adapters.SearchResponse{Results: items}, nil
}

func (r *denseRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
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
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	if req.Limit > 0 && len(result) > req.Limit {
		result = result[:req.Limit]
	}
	return adapters.SearchResponse{Results: result}, nil
}

func (r *denseRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("dense runtime: memory_id is required")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return adapters.MemoryItem{}, errors.New("dense runtime: memory is required")
	}
	botID := runtimeBotIDFromMemoryID(memoryID)
	if botID == "" {
		return adapters.MemoryItem{}, errors.New("dense runtime: invalid memory_id")
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
		return adapters.MemoryItem{}, errors.New("dense runtime: memory not found")
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

func (r *denseRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *denseRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
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
	if err := r.qdrant.DeleteByIDs(ctx, pointIDs); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *denseRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.store.RemoveAllMemories(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.qdrant.DeleteByBotID(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, err
	}
	return adapters.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

func (r *denseRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (adapters.CompactResult, error) {
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

func (r *denseRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
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

func (*denseRuntime) Mode() string {
	return string(ModeDense)
}

func (r *denseRuntime) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
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
		MemoryMode:        string(ModeDense),
		CanManualSync:     true,
		SourceDir:         path.Join(config.DefaultDataMount, "memory"),
		OverviewPath:      path.Join(config.DefaultDataMount, "MEMORY.md"),
		MarkdownFileCount: fileCount,
		SourceCount:       len(items),
		QdrantCollection:  r.collection,
	}
	if err := r.embedHealth(ctx); err != nil {
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

func (r *denseRuntime) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, err
	}
	if err := r.store.SyncOverview(ctx, botID); err != nil {
		return adapters.RebuildResult{}, err
	}
	return r.syncSourceItems(ctx, botID, items)
}

func (r *denseRuntime) syncSourceItems(ctx context.Context, botID string, items []storefs.MemoryItem) (adapters.RebuildResult, error) {
	if err := r.qdrant.EnsureDenseCollection(ctx, r.dimensions); err != nil {
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
		if sourceID != "" {
			existingBySource[sourceID] = item
		}
	}
	sourceIDs := make(map[string]struct{}, len(items))
	toUpsert := make([]storefs.MemoryItem, 0, len(items))
	missingCount := 0
	restoredCount := 0
	for _, item := range items {
		item = canonicalStoreItem(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
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
	stale := make([]string, 0)
	for _, item := range existing {
		sourceID := strings.TrimSpace(item.Payload["source_entry_id"])
		if sourceID == "" {
			sourceID = strings.TrimSpace(item.ID)
		}
		if _, ok := sourceIDs[sourceID]; ok {
			continue
		}
		if strings.TrimSpace(item.ID) != "" {
			stale = append(stale, item.ID)
		}
	}
	if len(stale) > 0 {
		if err := r.qdrant.DeleteByIDs(ctx, stale); err != nil {
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
		FsCount:       len(items),
		StorageCount:  count,
		MissingCount:  missingCount,
		RestoredCount: restoredCount,
	}, nil
}

func (r *denseRuntime) upsertSourceItems(ctx context.Context, botID string, items []storefs.MemoryItem) error {
	if len(items) == 0 {
		return nil
	}
	if err := r.qdrant.EnsureDenseCollection(ctx, r.dimensions); err != nil {
		return err
	}
	canonical := make([]storefs.MemoryItem, 0, len(items))
	texts := make([]string, 0, len(items))
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
	vectors, err := r.embedDocuments(ctx, texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(canonical) {
		return fmt.Errorf("dense embed documents: expected %d vectors, got %d", len(canonical), len(vectors))
	}
	for i, item := range canonical {
		if err := r.qdrant.UpsertDense(ctx, runtimePointID(botID, item.ID), qdrantclient.DenseVector{
			Values: vectors[i],
		}, runtimePayload(botID, item)); err != nil {
			return err
		}
	}
	return nil
}

func resolveDenseEmbeddingModel(ctx context.Context, queries *dbsqlc.Queries, modelRef string) (denseModelSpec, error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return denseModelSpec{}, errors.New("dense runtime: embedding_model_id is required")
	}
	var row dbsqlc.Model
	if parsed, err := db.ParseUUID(modelRef); err == nil {
		dbModel, err := queries.GetModelByID(ctx, parsed)
		if err == nil {
			row = dbModel
		}
	}
	if !row.ID.Valid {
		rows, err := queries.ListModelsByModelID(ctx, modelRef)
		if err != nil || len(rows) == 0 {
			return denseModelSpec{}, fmt.Errorf("dense runtime: embedding model not found: %s", modelRef)
		}
		row = rows[0]
	}
	if row.Type != "embedding" {
		return denseModelSpec{}, fmt.Errorf("dense runtime: model %s is not an embedding model", modelRef)
	}
	if !row.ProviderID.Valid {
		return denseModelSpec{}, fmt.Errorf("dense runtime: model %s has no provider", modelRef)
	}
	provider, err := queries.GetProviderByID(ctx, row.ProviderID)
	if err != nil {
		return denseModelSpec{}, fmt.Errorf("dense runtime: get embedding provider: %w", err)
	}
	var cfg struct {
		Dimensions *int `json:"dimensions"`
	}
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	if cfg.Dimensions == nil || *cfg.Dimensions <= 0 {
		return denseModelSpec{}, fmt.Errorf("dense runtime: embedding model %s missing dimensions", modelRef)
	}
	var providerCfg map[string]any
	if len(provider.Config) > 0 {
		_ = json.Unmarshal(provider.Config, &providerCfg)
	}
	baseURL, _ := providerCfg["base_url"].(string)
	apiKey, _ := providerCfg["api_key"].(string)
	return denseModelSpec{
		modelID:    strings.TrimSpace(row.ModelID),
		clientType: strings.TrimSpace(provider.ClientType),
		baseURL:    strings.TrimSpace(baseURL),
		apiKey:     strings.TrimSpace(apiKey),
		dimensions: *cfg.Dimensions,
	}, nil
}

// --- shared helpers (used by both dense and sparse runtimes) ---

func runtimeHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
