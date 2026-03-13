package builtin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

type denseRuntime struct {
	qdrant     *qdrantclient.Client
	store      *storefs.Service
	embedder   *denseEmbeddingClient
	collection string
}

type denseEmbeddingClient struct {
	baseURL    string
	apiKey     string
	modelID    string
	dimensions int
	httpClient *http.Client
}

type denseEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

type denseModelSpec struct {
	modelID    string
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

	modelSpec, err := resolveDenseEmbeddingModel(context.Background(), queries, modelRef)
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

	return &denseRuntime{
		qdrant: qClient,
		store:  store,
		embedder: &denseEmbeddingClient{
			baseURL:    strings.TrimRight(modelSpec.baseURL, "/"),
			apiKey:     modelSpec.apiKey,
			modelID:    modelSpec.modelID,
			dimensions: modelSpec.dimensions,
			httpClient: &http.Client{Timeout: 30 * time.Second},
		},
		collection: collection,
	}, nil
}

func (r *denseRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	botID, err := sparseRuntimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	text := sparseRuntimeText(req.Message, req.Messages)
	if text == "" {
		return adapters.SearchResponse{}, errors.New("dense runtime: message is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	item := adapters.MemoryItem{
		ID:        sparseRuntimeMemoryID(botID, time.Now().UTC()),
		Memory:    text,
		Hash:      denseRuntimeHash(text),
		Metadata:  req.Metadata,
		BotID:     botID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.store.PersistMemories(ctx, botID, []storefs.MemoryItem{denseStoreItemFromMemoryItem(item)}, req.Filters); err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.upsertSourceItems(ctx, botID, []storefs.MemoryItem{denseStoreItemFromMemoryItem(item)}); err != nil {
		return adapters.SearchResponse{}, err
	}
	return adapters.SearchResponse{Results: []adapters.MemoryItem{item}}, nil
}

func (r *denseRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	botID, err := sparseRuntimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	if err := r.qdrant.EnsureDenseCollection(ctx, r.embedder.dimensions); err != nil {
		return adapters.SearchResponse{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	vec, err := r.embedder.EmbedQuery(ctx, req.Query)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("dense embed query: %w", err)
	}
	results, err := r.qdrant.SearchDense(ctx, qdrantclient.DenseVector{Values: vec}, botID, limit)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items := make([]adapters.MemoryItem, 0, len(results))
	for _, result := range results {
		items = append(items, denseResultToItem(result))
	}
	return adapters.SearchResponse{Results: items}, nil
}

func (r *denseRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	botID, err := sparseRuntimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	result := make([]adapters.MemoryItem, 0, len(items))
	for _, item := range items {
		mem := denseMemoryItemFromStore(item)
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
	botID := sparseRuntimeBotIDFromMemoryID(memoryID)
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
	existing.Hash = denseRuntimeHash(text)
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := r.store.PersistMemories(ctx, botID, []storefs.MemoryItem{*existing}, nil); err != nil {
		return adapters.MemoryItem{}, err
	}
	if err := r.upsertSourceItems(ctx, botID, []storefs.MemoryItem{*existing}); err != nil {
		return adapters.MemoryItem{}, err
	}
	item := denseMemoryItemFromStore(*existing)
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
		botID := sparseRuntimeBotIDFromMemoryID(memoryID)
		if botID == "" {
			continue
		}
		grouped[botID] = append(grouped[botID], memoryID)
		pointIDs = append(pointIDs, sparsePointID(botID, memoryID))
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
	botID, err := sparseRuntimeBotID(req.BotID, req.Filters)
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
	botID, err := sparseRuntimeBotID("", filters)
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
		kept = append(kept, denseMemoryItemFromStore(item))
	}
	return adapters.CompactResult{
		BeforeCount: before,
		AfterCount:  len(kept),
		Ratio:       ratio,
		Results:     kept,
	}, nil
}

func (r *denseRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	botID, err := sparseRuntimeBotID("", filters)
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
	if err := r.embedder.Health(ctx); err != nil {
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
	if err := r.qdrant.EnsureDenseCollection(ctx, r.embedder.dimensions); err != nil {
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
		item = denseCanonicalStoreItem(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		sourceIDs[item.ID] = struct{}{}
		payload := densePayload(botID, item)
		existingItem, ok := existingBySource[item.ID]
		if !ok {
			missingCount++
			restoredCount++
			toUpsert = append(toUpsert, item)
			continue
		}
		if !densePayloadMatches(existingItem.Payload, payload) {
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
	if err := r.qdrant.EnsureDenseCollection(ctx, r.embedder.dimensions); err != nil {
		return err
	}
	canonical := make([]storefs.MemoryItem, 0, len(items))
	texts := make([]string, 0, len(items))
	for _, item := range items {
		item = denseCanonicalStoreItem(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		canonical = append(canonical, item)
		texts = append(texts, item.Memory)
	}
	if len(canonical) == 0 {
		return nil
	}
	vectors, err := r.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("dense embed documents: %w", err)
	}
	if len(vectors) != len(canonical) {
		return fmt.Errorf("dense embed documents: expected %d vectors, got %d", len(canonical), len(vectors))
	}
	for i, item := range canonical {
		if err := r.qdrant.UpsertDense(ctx, sparsePointID(botID, item.ID), qdrantclient.DenseVector{
			Values: vectors[i],
		}, densePayload(botID, item)); err != nil {
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
	if !row.LlmProviderID.Valid {
		return denseModelSpec{}, fmt.Errorf("dense runtime: model %s has no provider", modelRef)
	}
	provider, err := queries.GetLlmProviderByID(ctx, row.LlmProviderID)
	if err != nil {
		return denseModelSpec{}, fmt.Errorf("dense runtime: get embedding provider: %w", err)
	}
	if !row.Dimensions.Valid || row.Dimensions.Int32 <= 0 {
		return denseModelSpec{}, fmt.Errorf("dense runtime: embedding model %s missing dimensions", modelRef)
	}
	return denseModelSpec{
		modelID:    strings.TrimSpace(row.ModelID),
		baseURL:    strings.TrimSpace(provider.BaseUrl),
		apiKey:     strings.TrimSpace(provider.ApiKey),
		dimensions: int(row.Dimensions.Int32),
	}, nil
}

func joinDenseEmbeddingEndpointURL(baseURL, endpointPath string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("dense embedding base URL is required")
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid dense embedding base URL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("invalid dense embedding base URL scheme: %q", base.Scheme)
	}
	if base.Host == "" {
		return "", errors.New("invalid dense embedding base URL: host is required")
	}

	ref, err := url.Parse(endpointPath)
	if err != nil {
		return "", fmt.Errorf("invalid dense embedding path: %w", err)
	}
	return base.ResolveReference(ref).String(), nil
}

func (c *denseEmbeddingClient) Health(ctx context.Context) error {
	endpoint, err := joinDenseEmbeddingEndpointURL(c.baseURL, "/models")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is validated and derived from operator-configured embedding provider base URL
	if err != nil {
		return fmt.Errorf("dense embedding health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dense embedding health error (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *denseEmbeddingClient) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := c.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("dense embed query: empty embedding response")
	}
	return vectors[0], nil
}

func (c *denseEmbeddingClient) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": c.modelID,
		"input": texts,
	})
	if err != nil {
		return nil, err
	}
	endpoint, err := joinDenseEmbeddingEndpointURL(c.baseURL, "/embeddings")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is validated and derived from operator-configured embedding provider base URL
	if err != nil {
		return nil, fmt.Errorf("dense embed request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dense embed read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dense embed api error %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed denseEmbeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("dense embed decode response: %w", err)
	}
	vectors := make([][]float32, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}
	out := make([][]float32, 0, len(vectors))
	for _, vector := range vectors {
		if len(vector) > 0 {
			out = append(out, vector)
		}
	}
	return out, nil
}

func denseCanonicalStoreItem(item storefs.MemoryItem) storefs.MemoryItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Memory = strings.TrimSpace(item.Memory)
	if item.Memory != "" && strings.TrimSpace(item.Hash) == "" {
		item.Hash = denseRuntimeHash(item.Memory)
	}
	return item
}

func densePayload(botID string, item storefs.MemoryItem) map[string]string {
	item = denseCanonicalStoreItem(item)
	payload := map[string]string{
		"memory":          item.Memory,
		"bot_id":          strings.TrimSpace(botID),
		"source_entry_id": item.ID,
		"hash":            item.Hash,
	}
	if item.CreatedAt != "" {
		payload["created_at"] = item.CreatedAt
	}
	if item.UpdatedAt != "" {
		payload["updated_at"] = item.UpdatedAt
	}
	return payload
}

func densePayloadMatches(existing, expected map[string]string) bool {
	for key, value := range expected {
		if strings.TrimSpace(existing[key]) != strings.TrimSpace(value) {
			return false
		}
	}
	return true
}

func denseStoreItemFromMemoryItem(item adapters.MemoryItem) storefs.MemoryItem {
	return denseCanonicalStoreItem(storefs.MemoryItem{
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
	})
}

func denseMemoryItemFromStore(item storefs.MemoryItem) adapters.MemoryItem {
	item = denseCanonicalStoreItem(item)
	return adapters.MemoryItem{
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

func denseResultToItem(r qdrantclient.SearchResult) adapters.MemoryItem {
	item := adapters.MemoryItem{
		ID:    r.ID,
		Score: r.Score,
	}
	if r.Payload != nil {
		if sourceID := strings.TrimSpace(r.Payload["source_entry_id"]); sourceID != "" {
			item.ID = sourceID
		}
		item.Memory = r.Payload["memory"]
		item.Hash = r.Payload["hash"]
		item.BotID = r.Payload["bot_id"]
		item.CreatedAt = r.Payload["created_at"]
		item.UpdatedAt = r.Payload["updated_at"]
	}
	return item
}

func denseRuntimeHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
