package memory

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/memohai/memoh/internal/embeddings"
)

type Service struct {
	llm                      LLM
	embedder                 embeddings.Embedder
	store                    *QdrantStore
	resolver                 *embeddings.Resolver
	bm25                     *BM25Indexer
	logger                   *slog.Logger
	defaultTextModelID       string
	defaultMultimodalModelID string
}

func NewService(log *slog.Logger, llm LLM, embedder embeddings.Embedder, store *QdrantStore, resolver *embeddings.Resolver, bm25 *BM25Indexer, defaultTextModelID, defaultMultimodalModelID string) *Service {
	return &Service{
		llm:                      llm,
		embedder:                 embedder,
		store:                    store,
		resolver:                 resolver,
		bm25:                     bm25,
		logger:                   log.With(slog.String("service", "memory")),
		defaultTextModelID:       defaultTextModelID,
		defaultMultimodalModelID: defaultMultimodalModelID,
	}
}

func (s *Service) Add(ctx context.Context, req AddRequest) (SearchResponse, error) {
	if req.Message == "" && len(req.Messages) == 0 {
		return SearchResponse{}, fmt.Errorf("message or messages is required")
	}
	if req.BotID == "" && req.AgentID == "" && req.RunID == "" {
		return SearchResponse{}, fmt.Errorf("bot_id, agent_id or run_id is required")
	}

	messages := normalizeMessages(req)
	filters := buildFilters(req)

	embeddingEnabled := req.EmbeddingEnabled != nil && *req.EmbeddingEnabled
	if req.Infer != nil && !*req.Infer {
		return s.addRawMessages(ctx, messages, filters, req.Metadata, embeddingEnabled)
	}

	extractResp, err := s.llm.Extract(ctx, ExtractRequest{
		Messages: messages,
		Filters:  filters,
		Metadata: req.Metadata,
	})
	if err != nil {
		return SearchResponse{}, err
	}
	if len(extractResp.Facts) == 0 {
		return SearchResponse{Results: []MemoryItem{}}, nil
	}

	candidates, err := s.collectCandidates(ctx, extractResp.Facts, filters)
	if err != nil {
		return SearchResponse{}, err
	}

	decideResp, err := s.llm.Decide(ctx, DecideRequest{
		Facts:      extractResp.Facts,
		Candidates: candidates,
		Filters:    filters,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return SearchResponse{}, err
	}

	actions := decideResp.Actions
	if len(actions) == 0 && len(extractResp.Facts) > 0 {
		actions = make([]DecisionAction, 0, len(extractResp.Facts))
		for _, fact := range extractResp.Facts {
			actions = append(actions, DecisionAction{
				Event: "ADD",
				Text:  fact,
			})
		}
	}

	results := make([]MemoryItem, 0, len(actions))
	for _, action := range actions {
		switch strings.ToUpper(action.Event) {
		case "ADD":
			item, err := s.applyAdd(ctx, action.Text, filters, req.Metadata, embeddingEnabled)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]any{
				"event": "ADD",
			})
			results = append(results, item)
		case "UPDATE":
			item, err := s.applyUpdate(ctx, action.ID, action.Text, filters, req.Metadata, embeddingEnabled)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]any{
				"event":           "UPDATE",
				"previous_memory": action.OldMemory,
			})
			results = append(results, item)
		case "DELETE":
			item, err := s.applyDelete(ctx, action.ID)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]any{
				"event": "DELETE",
			})
			results = append(results, item)
		default:
			return SearchResponse{}, fmt.Errorf("unknown action: %s", action.Event)
		}
	}

	return SearchResponse{Results: results}, nil
}

func (s *Service) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return SearchResponse{}, fmt.Errorf("query is required")
	}
	if s.store == nil {
		return SearchResponse{}, fmt.Errorf("qdrant store not configured")
	}
	filters := buildSearchFilters(req)
	modality := ""
	if raw, ok := filters["modality"].(string); ok {
		modality = strings.ToLower(strings.TrimSpace(raw))
	}
	embeddingEnabled := req.EmbeddingEnabled != nil && *req.EmbeddingEnabled
	if modality == embeddings.TypeMultimodal {
		if !embeddingEnabled {
			return SearchResponse{}, fmt.Errorf("embedding is disabled")
		}
		if s.resolver == nil {
			return SearchResponse{}, fmt.Errorf("embeddings resolver not configured")
		}
		result, err := s.resolver.Embed(ctx, embeddings.Request{
			Type: embeddings.TypeMultimodal,
			Input: embeddings.Input{
				Text: req.Query,
			},
		})
		if err != nil {
			return SearchResponse{}, err
		}
		vectorName := s.vectorNameForMultimodal()
		if len(req.Sources) == 0 {
			points, scores, err := s.store.Search(ctx, result.Embedding, req.Limit, filters, vectorName)
			if err != nil {
				return SearchResponse{}, err
			}
			results := make([]MemoryItem, 0, len(points))
			for idx, point := range points {
				item := payloadToMemoryItem(point.ID, point.Payload)
				if idx < len(scores) {
					item.Score = scores[idx]
				}
				results = append(results, item)
			}
			return SearchResponse{Results: results}, nil
		}
		pointsBySource, scoresBySource, err := s.store.SearchBySources(ctx, result.Embedding, req.Limit, filters, req.Sources, vectorName)
		if err != nil {
			return SearchResponse{}, err
		}
		results := fuseByRankFusion(pointsBySource, scoresBySource)
		return SearchResponse{Results: results}, nil
	}

	if embeddingEnabled {
		if s.embedder == nil {
			return SearchResponse{}, fmt.Errorf("embedder not configured")
		}
		vector, err := s.embedder.Embed(ctx, req.Query)
		if err != nil {
			return SearchResponse{}, err
		}
		vectorName := s.vectorNameForText()
		if len(req.Sources) == 0 {
			points, scores, err := s.store.Search(ctx, vector, req.Limit, filters, vectorName)
			if err != nil {
				return SearchResponse{}, err
			}
			results := make([]MemoryItem, 0, len(points))
			for idx, point := range points {
				item := payloadToMemoryItem(point.ID, point.Payload)
				if idx < len(scores) {
					item.Score = scores[idx]
				}
				results = append(results, item)
			}
			return SearchResponse{Results: results}, nil
		}
		pointsBySource, scoresBySource, err := s.store.SearchBySources(ctx, vector, req.Limit, filters, req.Sources, vectorName)
		if err != nil {
			return SearchResponse{}, err
		}
		results := fuseByRankFusion(pointsBySource, scoresBySource)
		return SearchResponse{Results: results}, nil
	}

	if s.bm25 == nil {
		return SearchResponse{}, fmt.Errorf("bm25 indexer not configured")
	}
	lang, err := s.detectLanguage(ctx, req.Query)
	if err != nil {
		return SearchResponse{}, err
	}
	termFreq, _, err := s.bm25.TermFrequencies(lang, req.Query)
	if err != nil {
		return SearchResponse{}, err
	}
	indices, values := s.bm25.BuildQueryVector(lang, termFreq)
	if len(req.Sources) == 0 {
		points, scores, err := s.store.SearchSparse(ctx, indices, values, req.Limit, filters)
		if err != nil {
			return SearchResponse{}, err
		}
		results := make([]MemoryItem, 0, len(points))
		for idx, point := range points {
			item := payloadToMemoryItem(point.ID, point.Payload)
			if idx < len(scores) {
				item.Score = scores[idx]
			}
			results = append(results, item)
		}
		return SearchResponse{Results: results}, nil
	}
	pointsBySource, scoresBySource, err := s.store.SearchSparseBySources(ctx, indices, values, req.Limit, filters, req.Sources)
	if err != nil {
		return SearchResponse{}, err
	}
	results := fuseByRankFusion(pointsBySource, scoresBySource)
	return SearchResponse{Results: results}, nil
}

func (s *Service) EmbedUpsert(ctx context.Context, req EmbedUpsertRequest) (EmbedUpsertResponse, error) {
	if s.resolver == nil {
		return EmbedUpsertResponse{}, fmt.Errorf("embeddings resolver not configured")
	}
	if req.BotID == "" && req.AgentID == "" && req.RunID == "" {
		return EmbedUpsertResponse{}, fmt.Errorf("bot_id, agent_id or run_id is required")
	}
	req.Type = strings.TrimSpace(req.Type)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Model = strings.TrimSpace(req.Model)
	req.Input.Text = strings.TrimSpace(req.Input.Text)
	req.Input.ImageURL = strings.TrimSpace(req.Input.ImageURL)
	req.Input.VideoURL = strings.TrimSpace(req.Input.VideoURL)

	result, err := s.resolver.Embed(ctx, embeddings.Request{
		Type:     req.Type,
		Provider: req.Provider,
		Model:    req.Model,
		Input: embeddings.Input{
			Text:     req.Input.Text,
			ImageURL: req.Input.ImageURL,
			VideoURL: req.Input.VideoURL,
		},
	})
	if err != nil {
		return EmbedUpsertResponse{}, err
	}

	if s.store == nil {
		return EmbedUpsertResponse{}, fmt.Errorf("qdrant store not configured")
	}

	vectorName := ""
	if s.store.usesNamedVectors {
		vectorName = result.Model
	}

	id := uuid.NewString()
	filters := buildEmbedFilters(req)
	payload := buildEmbeddingPayload(req, filters)
	if metadata, ok := payload["metadata"].(map[string]any); ok && result.Model != "" {
		metadata["model_id"] = result.Model
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{{
		ID:         id,
		Vector:     result.Embedding,
		VectorName: vectorName,
		Payload:    payload,
	}}); err != nil {
		return EmbedUpsertResponse{}, err
	}

	item := payloadToMemoryItem(id, payload)
	return EmbedUpsertResponse{
		Item:       item,
		Provider:   result.Provider,
		Model:      result.Model,
		Dimensions: result.Dimensions,
	}, nil
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (MemoryItem, error) {
	if strings.TrimSpace(req.MemoryID) == "" {
		return MemoryItem{}, fmt.Errorf("memory_id is required")
	}
	if strings.TrimSpace(req.Memory) == "" {
		return MemoryItem{}, fmt.Errorf("memory is required")
	}
	if s.store == nil {
		return MemoryItem{}, fmt.Errorf("qdrant store not configured")
	}
	if s.bm25 == nil {
		return MemoryItem{}, fmt.Errorf("bm25 indexer not configured")
	}

	existing, err := s.store.Get(ctx, req.MemoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	if existing == nil {
		return MemoryItem{}, fmt.Errorf("memory not found")
	}

	payload := existing.Payload
	oldText := fmt.Sprint(payload["data"])
	oldLang := fmt.Sprint(payload["lang"])
	if oldLang == "" && strings.TrimSpace(oldText) != "" {
		var detectErr error
		oldLang, detectErr = s.detectLanguage(ctx, oldText)
		if detectErr != nil {
			s.logger.Warn("detect language failed for old text", slog.Any("error", detectErr))
		}
	}
	if strings.TrimSpace(oldText) != "" && strings.TrimSpace(oldLang) != "" {
		oldFreq, oldLen, err := s.bm25.TermFrequencies(oldLang, oldText)
		if err != nil {
			s.logger.Warn("bm25 term frequencies failed", slog.String("lang", oldLang), slog.Any("error", err))
		} else {
			s.bm25.RemoveDocument(oldLang, oldFreq, oldLen)
		}
	}

	newLang, err := s.detectLanguage(ctx, req.Memory)
	if err != nil {
		return MemoryItem{}, err
	}
	newFreq, newLen, err := s.bm25.TermFrequencies(newLang, req.Memory)
	if err != nil {
		return MemoryItem{}, err
	}
	sparseIndices, sparseValues := s.bm25.AddDocument(newLang, newFreq, newLen)

	payload["data"] = req.Memory
	payload["hash"] = hashMemory(req.Memory)
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	payload["lang"] = newLang

	embeddingEnabled := req.EmbeddingEnabled != nil && *req.EmbeddingEnabled
	point := qdrantPoint{
		ID:               req.MemoryID,
		SparseIndices:    sparseIndices,
		SparseValues:     sparseValues,
		SparseVectorName: s.store.sparseVectorName,
		Payload:          payload,
	}
	if embeddingEnabled {
		if s.embedder == nil {
			return MemoryItem{}, fmt.Errorf("embedder not configured")
		}
		vector, err := s.embedder.Embed(ctx, req.Memory)
		if err != nil {
			return MemoryItem{}, err
		}
		point.Vector = vector
		point.VectorName = s.vectorNameForText()
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{point}); err != nil {
		return MemoryItem{}, err
	}
	return payloadToMemoryItem(req.MemoryID, payload), nil
}

func (s *Service) Get(ctx context.Context, memoryID string) (MemoryItem, error) {
	if strings.TrimSpace(memoryID) == "" {
		return MemoryItem{}, fmt.Errorf("memory_id is required")
	}
	point, err := s.store.Get(ctx, memoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	if point == nil {
		return MemoryItem{}, fmt.Errorf("memory not found")
	}
	return payloadToMemoryItem(point.ID, point.Payload), nil
}

func (s *Service) GetAll(ctx context.Context, req GetAllRequest) (SearchResponse, error) {
	filters := map[string]any{}
	for k, v := range req.Filters {
		filters[k] = v
	}
	if req.BotID != "" {
		filters["bot_id"] = req.BotID
	}
	if req.AgentID != "" {
		filters["agent_id"] = req.AgentID
	}
	if req.RunID != "" {
		filters["run_id"] = req.RunID
	}
	if len(filters) == 0 {
		return SearchResponse{}, fmt.Errorf("bot_id, agent_id or run_id is required")
	}

	points, err := s.store.List(ctx, req.Limit, filters)
	if err != nil {
		return SearchResponse{}, err
	}
	results := make([]MemoryItem, 0, len(points))
	for _, point := range points {
		results = append(results, payloadToMemoryItem(point.ID, point.Payload))
	}
	return SearchResponse{Results: results}, nil
}

func (s *Service) Delete(ctx context.Context, memoryID string) (DeleteResponse, error) {
	if strings.TrimSpace(memoryID) == "" {
		return DeleteResponse{}, fmt.Errorf("memory_id is required")
	}
	if err := s.store.Delete(ctx, memoryID); err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Message: "Memory deleted successfully!"}, nil
}

func (s *Service) DeleteBatch(ctx context.Context, memoryIDs []string) (DeleteResponse, error) {
	if len(memoryIDs) == 0 {
		return DeleteResponse{}, fmt.Errorf("memory_ids is required")
	}
	cleaned := make([]string, 0, len(memoryIDs))
	for _, id := range memoryIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			cleaned = append(cleaned, id)
		}
	}
	if len(cleaned) == 0 {
		return DeleteResponse{}, fmt.Errorf("memory_ids is required")
	}
	if err := s.store.DeleteBatch(ctx, cleaned); err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Message: fmt.Sprintf("%d memories deleted successfully!", len(cleaned))}, nil
}

func (s *Service) DeleteAll(ctx context.Context, req DeleteAllRequest) (DeleteResponse, error) {
	filters := map[string]any{}
	for k, v := range req.Filters {
		filters[k] = v
	}
	if req.BotID != "" {
		filters["bot_id"] = req.BotID
	}
	if req.AgentID != "" {
		filters["agent_id"] = req.AgentID
	}
	if req.RunID != "" {
		filters["run_id"] = req.RunID
	}
	if len(filters) == 0 {
		return DeleteResponse{}, fmt.Errorf("bot_id, agent_id or run_id is required")
	}
	if err := s.store.DeleteAll(ctx, filters); err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (s *Service) Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (CompactResult, error) {
	if s.llm == nil {
		return CompactResult{}, fmt.Errorf("llm not configured")
	}
	if s.store == nil {
		return CompactResult{}, fmt.Errorf("qdrant store not configured")
	}
	if ratio <= 0 || ratio > 1 {
		ratio = 0.5
	}

	// Fetch all existing memories.
	points, err := s.store.List(ctx, 0, filters)
	if err != nil {
		return CompactResult{}, err
	}
	beforeCount := len(points)
	if beforeCount <= 1 {
		// Nothing to compact.
		items := make([]MemoryItem, 0, len(points))
		for _, p := range points {
			items = append(items, payloadToMemoryItem(p.ID, p.Payload))
		}
		return CompactResult{
			BeforeCount: beforeCount,
			AfterCount:  beforeCount,
			Ratio:       1.0,
			Results:     items,
		}, nil
	}

	// Build candidate list and compute target.
	candidates := make([]CandidateMemory, 0, beforeCount)
	for _, p := range points {
		candidates = append(candidates, CandidateMemory{
			ID:        p.ID,
			Memory:    fmt.Sprint(p.Payload["data"]),
			CreatedAt: fmt.Sprint(p.Payload["created_at"]),
		})
	}
	targetCount := int(math.Round(float64(beforeCount) * ratio))
	if targetCount < 1 {
		targetCount = 1
	}

	// Ask LLM to consolidate.
	compactResp, err := s.llm.Compact(ctx, CompactRequest{
		Memories:    candidates,
		TargetCount: targetCount,
		DecayDays:   decayDays,
	})
	if err != nil {
		return CompactResult{}, fmt.Errorf("compact llm call failed: %w", err)
	}
	if len(compactResp.Facts) == 0 {
		return CompactResult{}, fmt.Errorf("compact returned no facts")
	}

	// Delete old memories.
	if err := s.store.DeleteAll(ctx, filters); err != nil {
		return CompactResult{}, fmt.Errorf("compact delete old failed: %w", err)
	}

	// Reset BM25 stats for deleted documents.
	if s.bm25 != nil {
		for _, p := range points {
			text := fmt.Sprint(p.Payload["data"])
			lang := fmt.Sprint(p.Payload["lang"])
			if strings.TrimSpace(text) == "" || strings.TrimSpace(lang) == "" {
				continue
			}
			freq, docLen, err := s.bm25.TermFrequencies(lang, text)
			if err != nil {
				continue
			}
			s.bm25.RemoveDocument(lang, freq, docLen)
		}
	}

	// Add compacted facts.
	results := make([]MemoryItem, 0, len(compactResp.Facts))
	for _, fact := range compactResp.Facts {
		if strings.TrimSpace(fact) == "" {
			continue
		}
		item, err := s.applyAdd(ctx, fact, filters, nil, false)
		if err != nil {
			return CompactResult{}, fmt.Errorf("compact add failed: %w", err)
		}
		results = append(results, item)
	}

	afterCount := len(results)
	actualRatio := float64(afterCount) / float64(beforeCount)
	return CompactResult{
		BeforeCount: beforeCount,
		AfterCount:  afterCount,
		Ratio:       math.Round(actualRatio*100) / 100,
		Results:     results,
	}, nil
}

const (
	// Estimated sparse vector overhead per point: ~200 dims * 8 bytes (4 index + 4 value).
	sparseVectorOverheadBytes = 1600
	// Estimated payload metadata overhead per point (hash, dates, filters, lang, metadata JSON).
	payloadMetadataOverheadBytes = 256
)

func (s *Service) Usage(ctx context.Context, filters map[string]any) (UsageResponse, error) {
	if s.store == nil {
		return UsageResponse{}, fmt.Errorf("qdrant store not configured")
	}
	points, err := s.store.List(ctx, 0, filters)
	if err != nil {
		return UsageResponse{}, err
	}
	count := len(points)
	var totalTextBytes int64
	for _, p := range points {
		text := fmt.Sprint(p.Payload["data"])
		totalTextBytes += int64(len(text))
	}
	var avgTextBytes int64
	if count > 0 {
		avgTextBytes = totalTextBytes / int64(count)
	}
	estimatedStorage := totalTextBytes + int64(count)*(sparseVectorOverheadBytes+payloadMetadataOverheadBytes)
	return UsageResponse{
		Count:                 count,
		TotalTextBytes:        totalTextBytes,
		AvgTextBytes:          avgTextBytes,
		EstimatedStorageBytes: estimatedStorage,
	}, nil
}

func (s *Service) WarmupBM25(ctx context.Context, batchSize int) error {
	if s.bm25 == nil || s.store == nil {
		return nil
	}
	var offset *qdrant.PointId
	for {
		points, next, err := s.store.Scroll(ctx, batchSize, nil, offset)
		if err != nil {
			return err
		}
		if len(points) == 0 {
			break
		}
		for _, point := range points {
			text := fmt.Sprint(point.Payload["data"])
			if strings.TrimSpace(text) == "" {
				continue
			}
			lang := fmt.Sprint(point.Payload["lang"])
			if lang == "" {
				lang = fallbackLanguageCode(text)
			}
			termFreq, docLen, err := s.bm25.TermFrequencies(lang, text)
			if err != nil {
				s.logger.Warn("bm25 warmup: term frequencies failed", slog.String("id", point.ID), slog.Any("error", err))
				continue
			}
			s.bm25.AddDocument(lang, termFreq, docLen)
		}
		if next == nil {
			break
		}
		offset = next
	}
	return nil
}

func (s *Service) addRawMessages(ctx context.Context, messages []Message, filters map[string]any, metadata map[string]any, embeddingEnabled bool) (SearchResponse, error) {
	results := make([]MemoryItem, 0, len(messages))
	for _, message := range messages {
		item, err := s.applyAdd(ctx, message.Content, filters, metadata, embeddingEnabled)
		if err != nil {
			return SearchResponse{}, err
		}
		item.Metadata = mergeMetadata(item.Metadata, map[string]any{
			"event": "ADD",
		})
		results = append(results, item)
	}
	return SearchResponse{Results: results}, nil
}

func (s *Service) collectCandidates(ctx context.Context, facts []string, filters map[string]any) ([]CandidateMemory, error) {
	unique := map[string]CandidateMemory{}
	for _, fact := range facts {
		if s.bm25 == nil {
			return nil, fmt.Errorf("bm25 indexer not configured")
		}
		lang, err := s.detectLanguage(ctx, fact)
		if err != nil {
			return nil, err
		}
		termFreq, _, err := s.bm25.TermFrequencies(lang, fact)
		if err != nil {
			return nil, err
		}
		indices, values := s.bm25.BuildQueryVector(lang, termFreq)
		points, _, err := s.store.SearchSparse(ctx, indices, values, 5, filters)
		if err != nil {
			return nil, err
		}
		for _, point := range points {
			item := payloadToMemoryItem(point.ID, point.Payload)
			unique[item.ID] = CandidateMemory{
				ID:       item.ID,
				Memory:   item.Memory,
				Metadata: item.Metadata,
			}
		}
	}

	candidates := make([]CandidateMemory, 0, len(unique))
	for _, candidate := range unique {
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s *Service) applyAdd(ctx context.Context, text string, filters map[string]any, metadata map[string]any, embeddingEnabled bool) (MemoryItem, error) {
	if s.store == nil {
		return MemoryItem{}, fmt.Errorf("qdrant store not configured")
	}
	if s.bm25 == nil {
		return MemoryItem{}, fmt.Errorf("bm25 indexer not configured")
	}
	lang, err := s.detectLanguage(ctx, text)
	if err != nil {
		return MemoryItem{}, err
	}
	termFreq, docLen, err := s.bm25.TermFrequencies(lang, text)
	if err != nil {
		return MemoryItem{}, err
	}
	sparseIndices, sparseValues := s.bm25.AddDocument(lang, termFreq, docLen)
	id := uuid.NewString()
	payload := buildPayload(text, filters, metadata, "")
	payload["lang"] = lang
	point := qdrantPoint{
		ID:               id,
		SparseIndices:    sparseIndices,
		SparseValues:     sparseValues,
		SparseVectorName: s.store.sparseVectorName,
		Payload:          payload,
	}
	if embeddingEnabled {
		if s.embedder == nil {
			return MemoryItem{}, fmt.Errorf("embedder not configured")
		}
		vector, err := s.embedder.Embed(ctx, text)
		if err != nil {
			return MemoryItem{}, err
		}
		point.Vector = vector
		point.VectorName = s.vectorNameForText()
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{point}); err != nil {
		return MemoryItem{}, err
	}
	return payloadToMemoryItem(id, payload), nil
}

// RebuildAdd inserts a memory with a specific ID (from filesystem recovery).
// Like applyAdd but preserves the given ID instead of generating a new UUID.
func (s *Service) RebuildAdd(ctx context.Context, id, text string, filters map[string]any) (MemoryItem, error) {
	if s.store == nil {
		return MemoryItem{}, fmt.Errorf("qdrant store not configured")
	}
	if s.bm25 == nil {
		return MemoryItem{}, fmt.Errorf("bm25 indexer not configured")
	}
	if strings.TrimSpace(id) == "" {
		return MemoryItem{}, fmt.Errorf("id is required for rebuild")
	}
	lang, err := s.detectLanguage(ctx, text)
	if err != nil {
		return MemoryItem{}, err
	}
	termFreq, docLen, err := s.bm25.TermFrequencies(lang, text)
	if err != nil {
		return MemoryItem{}, err
	}
	sparseIndices, sparseValues := s.bm25.AddDocument(lang, termFreq, docLen)
	payload := buildPayload(text, filters, nil, "")
	payload["lang"] = lang
	point := qdrantPoint{
		ID:               id,
		SparseIndices:    sparseIndices,
		SparseValues:     sparseValues,
		SparseVectorName: s.store.sparseVectorName,
		Payload:          payload,
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{point}); err != nil {
		return MemoryItem{}, err
	}
	return payloadToMemoryItem(id, payload), nil
}

func (s *Service) applyUpdate(ctx context.Context, id, text string, filters map[string]any, metadata map[string]any, embeddingEnabled bool) (MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return MemoryItem{}, fmt.Errorf("update action missing id")
	}
	existing, err := s.store.Get(ctx, id)
	if err != nil {
		return MemoryItem{}, err
	}
	if existing == nil {
		return MemoryItem{}, fmt.Errorf("memory not found")
	}

	payload := existing.Payload
	oldText := fmt.Sprint(payload["data"])
	oldLang := fmt.Sprint(payload["lang"])
	if oldLang == "" && strings.TrimSpace(oldText) != "" {
		var detectErr error
		oldLang, detectErr = s.detectLanguage(ctx, oldText)
		if detectErr != nil {
			s.logger.Warn("detect language failed for old text", slog.Any("error", detectErr))
		}
	}
	if strings.TrimSpace(oldText) != "" && strings.TrimSpace(oldLang) != "" {
		oldFreq, oldLen, err := s.bm25.TermFrequencies(oldLang, oldText)
		if err != nil {
			s.logger.Warn("bm25 term frequencies failed", slog.String("lang", oldLang), slog.Any("error", err))
		} else {
			s.bm25.RemoveDocument(oldLang, oldFreq, oldLen)
		}
	}
	newLang, err := s.detectLanguage(ctx, text)
	if err != nil {
		return MemoryItem{}, err
	}
	newFreq, newLen, err := s.bm25.TermFrequencies(newLang, text)
	if err != nil {
		return MemoryItem{}, err
	}
	sparseIndices, sparseValues := s.bm25.AddDocument(newLang, newFreq, newLen)
	payload["data"] = text
	payload["hash"] = hashMemory(text)
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	payload["lang"] = newLang
	if metadata != nil {
		payload["metadata"] = mergeMetadata(payload["metadata"], metadata)
	}
	if filters != nil {
		applyFiltersToPayload(payload, filters)
	}
	point := qdrantPoint{
		ID:               id,
		SparseIndices:    sparseIndices,
		SparseValues:     sparseValues,
		SparseVectorName: s.store.sparseVectorName,
		Payload:          payload,
	}
	if embeddingEnabled {
		if s.embedder == nil {
			return MemoryItem{}, fmt.Errorf("embedder not configured")
		}
		vector, err := s.embedder.Embed(ctx, text)
		if err != nil {
			return MemoryItem{}, err
		}
		point.Vector = vector
		point.VectorName = s.vectorNameForText()
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{point}); err != nil {
		return MemoryItem{}, err
	}
	return payloadToMemoryItem(id, payload), nil
}

func (s *Service) applyDelete(ctx context.Context, id string) (MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return MemoryItem{}, fmt.Errorf("delete action missing id")
	}
	existing, err := s.store.Get(ctx, id)
	if err != nil {
		return MemoryItem{}, err
	}
	if existing == nil {
		return MemoryItem{}, fmt.Errorf("memory not found")
	}
	item := payloadToMemoryItem(id, existing.Payload)
	if s.bm25 != nil {
		oldText := fmt.Sprint(existing.Payload["data"])
		oldLang := fmt.Sprint(existing.Payload["lang"])
		if oldLang == "" && strings.TrimSpace(oldText) != "" {
			var detectErr error
			oldLang, detectErr = s.detectLanguage(ctx, oldText)
			if detectErr != nil {
				s.logger.Warn("detect language failed for old text", slog.Any("error", detectErr))
			}
		}
		if strings.TrimSpace(oldText) != "" && strings.TrimSpace(oldLang) != "" {
			oldFreq, oldLen, err := s.bm25.TermFrequencies(oldLang, oldText)
			if err != nil {
				s.logger.Warn("bm25 term frequencies failed", slog.String("lang", oldLang), slog.Any("error", err))
			} else {
				s.bm25.RemoveDocument(oldLang, oldFreq, oldLen)
			}
		}
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return MemoryItem{}, err
	}
	return item, nil
}

func normalizeMessages(req AddRequest) []Message {
	if len(req.Messages) > 0 {
		return req.Messages
	}
	return []Message{{Role: "user", Content: req.Message}}
}

func (s *Service) detectLanguage(ctx context.Context, text string) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("language detector not configured")
	}
	lang, err := s.llm.DetectLanguage(ctx, text)
	if err == nil && lang != "" {
		return lang, nil
	}
	fallback := fallbackLanguageCode(text)
	if s.logger != nil {
		s.logger.Warn("language detection failed; using fallback", slog.Any("error", err), slog.String("fallback", fallback))
	}
	return fallback, nil
}

func fallbackLanguageCode(text string) string {
	for _, r := range text {
		if isCJKRune(r) {
			return "cjk"
		}
	}
	return "en"
}

func isCJKRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Unified Ideographs Extension A
		return true
	case r >= 0x20000 && r <= 0x2A6DF: // CJK Unified Ideographs Extension B
		return true
	case r >= 0x2A700 && r <= 0x2B73F: // CJK Unified Ideographs Extension C
		return true
	case r >= 0x2B740 && r <= 0x2B81F: // CJK Unified Ideographs Extension D
		return true
	case r >= 0x2B820 && r <= 0x2CEAF: // CJK Unified Ideographs Extension E
		return true
	case r >= 0x2CEB0 && r <= 0x2EBEF: // CJK Unified Ideographs Extension F
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation
		return true
	case r >= 0x3040 && r <= 0x30FF: // Hiragana/Katakana
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
		return true
	}
	return false
}

func buildFilters(req AddRequest) map[string]any {
	filters := map[string]any{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.BotID != "" {
		filters["bot_id"] = req.BotID
	}
	if req.AgentID != "" {
		filters["agent_id"] = req.AgentID
	}
	if req.RunID != "" {
		filters["run_id"] = req.RunID
	}
	return filters
}

func buildSearchFilters(req SearchRequest) map[string]any {
	filters := map[string]any{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.BotID != "" {
		filters["bot_id"] = req.BotID
	}
	if req.AgentID != "" {
		filters["agent_id"] = req.AgentID
	}
	if req.RunID != "" {
		filters["run_id"] = req.RunID
	}
	return filters
}

func buildEmbedFilters(req EmbedUpsertRequest) map[string]any {
	filters := map[string]any{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.BotID != "" {
		filters["bot_id"] = req.BotID
	}
	if req.AgentID != "" {
		filters["agent_id"] = req.AgentID
	}
	if req.RunID != "" {
		filters["run_id"] = req.RunID
	}
	return filters
}

func buildEmbeddingPayload(req EmbedUpsertRequest, filters map[string]any) map[string]any {
	text := req.Input.Text
	payload := buildPayload(text, filters, req.Metadata, "")
	payload["hash"] = hashEmbeddingInput(req.Input.Text, req.Input.ImageURL, req.Input.VideoURL)
	if req.Source != "" {
		payload["source"] = req.Source
	}
	modality := "text"
	if req.Type != "" {
		modality = strings.ToLower(req.Type)
	}
	payload["modality"] = modality

	if payload["metadata"] == nil {
		payload["metadata"] = map[string]any{}
	}
	if metadata, ok := payload["metadata"].(map[string]any); ok {
		if req.Source != "" {
			metadata["source"] = req.Source
		}
		metadata["modality"] = modality
		if req.Input.ImageURL != "" {
			metadata["image_url"] = req.Input.ImageURL
		}
		if req.Input.VideoURL != "" {
			metadata["video_url"] = req.Input.VideoURL
		}
	}
	return payload
}

func (s *Service) vectorNameForText() string {
	if s.store == nil || !s.store.usesNamedVectors {
		return ""
	}
	return strings.TrimSpace(s.defaultTextModelID)
}

func (s *Service) vectorNameForMultimodal() string {
	if s.store == nil || !s.store.usesNamedVectors {
		return ""
	}
	return strings.TrimSpace(s.defaultMultimodalModelID)
}

func buildPayload(text string, filters map[string]any, metadata map[string]any, createdAt string) map[string]any {
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	payload := map[string]any{
		"data":       text,
		"hash":       hashMemory(text),
		"created_at": createdAt,
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}
	applyFiltersToPayload(payload, filters)
	return payload
}

func applyFiltersToPayload(payload map[string]any, filters map[string]any) {
	for key, value := range filters {
		payload[key] = value
	}
}

func payloadToMemoryItem(id string, payload map[string]any) MemoryItem {
	item := MemoryItem{
		ID:     id,
		Memory: fmt.Sprint(payload["data"]),
	}
	if v, ok := payload["hash"].(string); ok {
		item.Hash = v
	}
	if v, ok := payload["created_at"].(string); ok {
		item.CreatedAt = v
	}
	if v, ok := payload["updated_at"].(string); ok {
		item.UpdatedAt = v
	}
	if v, ok := payload["bot_id"].(string); ok {
		item.BotID = v
	}
	if v, ok := payload["agent_id"].(string); ok {
		item.AgentID = v
	}
	if v, ok := payload["run_id"].(string); ok {
		item.RunID = v
	}
	if meta, ok := payload["metadata"].(map[string]any); ok {
		item.Metadata = meta
	} else if payload["metadata"] == nil {
		item.Metadata = map[string]any{}
	}
	if item.Metadata != nil {
		if source, ok := payload["source"].(string); ok && source != "" {
			item.Metadata["source"] = source
		}
		if modality, ok := payload["modality"].(string); ok && modality != "" {
			item.Metadata["modality"] = modality
		}
	}
	return item
}

func hashMemory(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func hashEmbeddingInput(text, imageURL, videoURL string) string {
	combined := strings.Join([]string{
		strings.TrimSpace(text),
		strings.TrimSpace(imageURL),
		strings.TrimSpace(videoURL),
	}, "|")
	sum := md5.Sum([]byte(combined))
	return hex.EncodeToString(sum[:])
}

func mergeMetadata(base any, extra map[string]any) map[string]any {
	merged := map[string]any{}
	if baseMap, ok := base.(map[string]any); ok {
		for k, v := range baseMap {
			merged[k] = v
		}
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

type rerankCandidate struct {
	ID      string
	Payload map[string]any
}

const (
	rrfK = 60.0
)

func fuseByRankFusion(pointsBySource map[string][]qdrantPoint, _ map[string][]float64) []MemoryItem {
	candidates := map[string]*rerankCandidate{}
	rrfScores := map[string]float64{}

	for _, points := range pointsBySource {
		for idx, point := range points {
			if _, ok := candidates[point.ID]; !ok {
				candidates[point.ID] = &rerankCandidate{
					ID:      point.ID,
					Payload: point.Payload,
				}
			}
			rank := float64(idx + 1)
			rrfScores[point.ID] += 1.0 / (rrfK + rank)
		}
	}

	items := make([]MemoryItem, 0, len(candidates))
	for id, candidate := range candidates {
		item := payloadToMemoryItem(candidate.ID, candidate.Payload)
		item.Score = rrfScores[id]
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})
	return items
}
