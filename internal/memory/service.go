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

	"github.com/memohai/memoh/internal/embeddings"
)

type Service struct {
	llm                      LLM
	embedder                 embeddings.Embedder
	store                    *QdrantStore
	resolver                 *embeddings.Resolver
	logger                   *slog.Logger
	defaultTextModelID       string
	defaultMultimodalModelID string
}

func NewService(log *slog.Logger, llm LLM, embedder embeddings.Embedder, store *QdrantStore, resolver *embeddings.Resolver, defaultTextModelID, defaultMultimodalModelID string) *Service {
	return &Service{
		llm:                      llm,
		embedder:                 embedder,
		store:                    store,
		resolver:                 resolver,
		logger:                   log.With(slog.String("service", "memory")),
		defaultTextModelID:       defaultTextModelID,
		defaultMultimodalModelID: defaultMultimodalModelID,
	}
}

func (s *Service) Add(ctx context.Context, req AddRequest) (SearchResponse, error) {
	if req.Message == "" && len(req.Messages) == 0 {
		return SearchResponse{}, fmt.Errorf("message or messages is required")
	}
	if req.UserID == "" && req.AgentID == "" && req.RunID == "" {
		return SearchResponse{}, fmt.Errorf("user_id, agent_id or run_id is required")
	}

	messages := normalizeMessages(req)
	filters := buildFilters(req)

	if req.Infer != nil && !*req.Infer {
		return s.addRawMessages(ctx, messages, filters, req.Metadata)
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
			item, err := s.applyAdd(ctx, action.Text, filters, req.Metadata)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]interface{}{
				"event": "ADD",
			})
			results = append(results, item)
		case "UPDATE":
			item, err := s.applyUpdate(ctx, action.ID, action.Text, filters, req.Metadata)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]interface{}{
				"event":           "UPDATE",
				"previous_memory": action.OldMemory,
			})
			results = append(results, item)
		case "DELETE":
			item, err := s.applyDelete(ctx, action.ID)
			if err != nil {
				return SearchResponse{}, err
			}
			item.Metadata = mergeMetadata(item.Metadata, map[string]interface{}{
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
	filters := buildSearchFilters(req)
	modality := ""
	if raw, ok := filters["modality"].(string); ok {
		modality = strings.ToLower(strings.TrimSpace(raw))
	}

	var (
		vector     []float32
		store      *QdrantStore
		vectorName string
		err        error
	)
	if modality == embeddings.TypeMultimodal {
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
		vector = result.Embedding
		store = s.store
		vectorName = s.vectorNameForMultimodal()
	} else {
		vector, err = s.embedder.Embed(ctx, req.Query)
		if err != nil {
			return SearchResponse{}, err
		}
		store = s.store
		vectorName = s.vectorNameForText()
	}

	if len(req.Sources) == 0 {
		points, scores, err := store.Search(ctx, vector, req.Limit, filters, vectorName)
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

	pointsBySource, scoresBySource, err := store.SearchBySources(ctx, vector, req.Limit, filters, req.Sources, vectorName)
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
	if req.UserID == "" && req.AgentID == "" && req.RunID == "" {
		return EmbedUpsertResponse{}, fmt.Errorf("user_id, agent_id or run_id is required")
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
	if s.store != nil && s.store.usesNamedVectors {
		vectorName = result.Model
	}

	id := uuid.NewString()
	filters := buildEmbedFilters(req)
	payload := buildEmbeddingPayload(req, filters)
	if metadata, ok := payload["metadata"].(map[string]interface{}); ok && result.Model != "" {
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

	existing, err := s.store.Get(ctx, req.MemoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	if existing == nil {
		return MemoryItem{}, fmt.Errorf("memory not found")
	}

	payload := existing.Payload
	payload["data"] = req.Memory
	payload["hash"] = hashMemory(req.Memory)
	payload["updatedAt"] = time.Now().UTC().Format(time.RFC3339)

	vector, err := s.embedder.Embed(ctx, req.Memory)
	if err != nil {
		return MemoryItem{}, err
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{{
		ID:         req.MemoryID,
		Vector:     vector,
		VectorName: s.vectorNameForText(),
		Payload:    payload,
	}}); err != nil {
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
	filters := map[string]interface{}{}
	if req.UserID != "" {
		filters["userId"] = req.UserID
	}
	if req.AgentID != "" {
		filters["agentId"] = req.AgentID
	}
	if req.RunID != "" {
		filters["runId"] = req.RunID
	}
	if len(filters) == 0 {
		return SearchResponse{}, fmt.Errorf("user_id, agent_id or run_id is required")
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

func (s *Service) DeleteAll(ctx context.Context, req DeleteAllRequest) (DeleteResponse, error) {
	filters := map[string]interface{}{}
	if req.UserID != "" {
		filters["userId"] = req.UserID
	}
	if req.AgentID != "" {
		filters["agentId"] = req.AgentID
	}
	if req.RunID != "" {
		filters["runId"] = req.RunID
	}
	if len(filters) == 0 {
		return DeleteResponse{}, fmt.Errorf("user_id, agent_id or run_id is required")
	}
	if err := s.store.DeleteAll(ctx, filters); err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (s *Service) addRawMessages(ctx context.Context, messages []Message, filters map[string]interface{}, metadata map[string]interface{}) (SearchResponse, error) {
	results := make([]MemoryItem, 0, len(messages))
	for _, message := range messages {
		item, err := s.applyAdd(ctx, message.Content, filters, metadata)
		if err != nil {
			return SearchResponse{}, err
		}
		item.Metadata = mergeMetadata(item.Metadata, map[string]interface{}{
			"event": "ADD",
		})
		results = append(results, item)
	}
	return SearchResponse{Results: results}, nil
}

func (s *Service) collectCandidates(ctx context.Context, facts []string, filters map[string]interface{}) ([]CandidateMemory, error) {
	unique := map[string]CandidateMemory{}
	for _, fact := range facts {
		vector, err := s.embedder.Embed(ctx, fact)
		if err != nil {
			return nil, err
		}
		points, _, err := s.store.Search(ctx, vector, 5, filters, s.vectorNameForText())
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

func (s *Service) applyAdd(ctx context.Context, text string, filters map[string]interface{}, metadata map[string]interface{}) (MemoryItem, error) {
	vector, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return MemoryItem{}, err
	}
	id := uuid.NewString()
	payload := buildPayload(text, filters, metadata, "")
	if err := s.store.Upsert(ctx, []qdrantPoint{{
		ID:         id,
		Vector:     vector,
		VectorName: s.vectorNameForText(),
		Payload:    payload,
	}}); err != nil {
		return MemoryItem{}, err
	}
	return payloadToMemoryItem(id, payload), nil
}

func (s *Service) applyUpdate(ctx context.Context, id, text string, filters map[string]interface{}, metadata map[string]interface{}) (MemoryItem, error) {
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
	payload["data"] = text
	payload["hash"] = hashMemory(text)
	payload["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	if metadata != nil {
		payload["metadata"] = mergeMetadata(payload["metadata"], metadata)
	}
	if filters != nil {
		applyFiltersToPayload(payload, filters)
	}
	vector, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return MemoryItem{}, err
	}
	if err := s.store.Upsert(ctx, []qdrantPoint{{
		ID:         id,
		Vector:     vector,
		VectorName: s.vectorNameForText(),
		Payload:    payload,
	}}); err != nil {
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

func buildFilters(req AddRequest) map[string]interface{} {
	filters := map[string]interface{}{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.UserID != "" {
		filters["userId"] = req.UserID
	}
	if req.AgentID != "" {
		filters["agentId"] = req.AgentID
	}
	if req.RunID != "" {
		filters["runId"] = req.RunID
	}
	return filters
}

func buildSearchFilters(req SearchRequest) map[string]interface{} {
	filters := map[string]interface{}{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.UserID != "" {
		filters["userId"] = req.UserID
	}
	if req.AgentID != "" {
		filters["agentId"] = req.AgentID
	}
	if req.RunID != "" {
		filters["runId"] = req.RunID
	}
	return filters
}

func buildEmbedFilters(req EmbedUpsertRequest) map[string]interface{} {
	filters := map[string]interface{}{}
	for key, value := range req.Filters {
		filters[key] = value
	}
	if req.UserID != "" {
		filters["userId"] = req.UserID
	}
	if req.AgentID != "" {
		filters["agentId"] = req.AgentID
	}
	if req.RunID != "" {
		filters["runId"] = req.RunID
	}
	return filters
}

func buildEmbeddingPayload(req EmbedUpsertRequest, filters map[string]interface{}) map[string]interface{} {
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
		payload["metadata"] = map[string]interface{}{}
	}
	if metadata, ok := payload["metadata"].(map[string]interface{}); ok {
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

func buildPayload(text string, filters map[string]interface{}, metadata map[string]interface{}, createdAt string) map[string]interface{} {
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	payload := map[string]interface{}{
		"data":      text,
		"hash":      hashMemory(text),
		"createdAt": createdAt,
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}
	applyFiltersToPayload(payload, filters)
	return payload
}

func applyFiltersToPayload(payload map[string]interface{}, filters map[string]interface{}) {
	for key, value := range filters {
		payload[key] = value
	}
}

func payloadToMemoryItem(id string, payload map[string]interface{}) MemoryItem {
	item := MemoryItem{
		ID:     id,
		Memory: fmt.Sprint(payload["data"]),
	}
	if v, ok := payload["hash"].(string); ok {
		item.Hash = v
	}
	if v, ok := payload["createdAt"].(string); ok {
		item.CreatedAt = v
	}
	if v, ok := payload["updatedAt"].(string); ok {
		item.UpdatedAt = v
	}
	if v, ok := payload["userId"].(string); ok {
		item.UserID = v
	}
	if v, ok := payload["agentId"].(string); ok {
		item.AgentID = v
	}
	if v, ok := payload["runId"].(string); ok {
		item.RunID = v
	}
	if meta, ok := payload["metadata"].(map[string]interface{}); ok {
		item.Metadata = meta
	} else if payload["metadata"] == nil {
		item.Metadata = map[string]interface{}{}
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

func mergeMetadata(base interface{}, extra map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	if baseMap, ok := base.(map[string]interface{}); ok {
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
	Payload map[string]interface{}
	Score   float64
	Source  string
	Rank    int
}

const (
	fusionModeRRF     = "rrf"
	fusionModeCombMNZ = "combmnz"
	fusionMode        = fusionModeRRF
	rrfK              = 60.0
)

func fuseByRankFusion(pointsBySource map[string][]qdrantPoint, scoresBySource map[string][]float64) []MemoryItem {
	candidates := map[string]*rerankCandidate{}
	rrfScores := map[string]float64{}
	combScores := map[string]float64{}
	combCounts := map[string]int{}

	for source, points := range pointsBySource {
		scores := scoresBySource[source]
		minScore := math.MaxFloat64
		maxScore := -math.MaxFloat64
		for idx, point := range points {
			if idx >= len(scores) {
				continue
			}
			score := scores[idx]
			if score < minScore {
				minScore = score
			}
			if score > maxScore {
				maxScore = score
			}
			if _, ok := candidates[point.ID]; !ok {
				candidates[point.ID] = &rerankCandidate{
					ID:      point.ID,
					Payload: point.Payload,
				}
			}
		}
		if minScore == math.MaxFloat64 {
			minScore = 0
		}
		if maxScore == -math.MaxFloat64 {
			maxScore = minScore
		}

		for idx, point := range points {
			if idx >= len(scores) {
				continue
			}
			score := scores[idx]
			rank := float64(idx + 1)
			rrfScores[point.ID] += 1.0 / (rrfK + rank)

			scoreNorm := normalizeScore(score, minScore, maxScore)
			combScores[point.ID] += scoreNorm
			combCounts[point.ID]++
		}
	}

	items := make([]MemoryItem, 0, len(candidates))
	for id, candidate := range candidates {
		item := payloadToMemoryItem(candidate.ID, candidate.Payload)
		switch fusionMode {
		case fusionModeCombMNZ:
			item.Score = combScores[id] * float64(combCounts[id])
		default:
			item.Score = rrfScores[id]
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})
	return items
}

func normalizeScore(score, minScore, maxScore float64) float64 {
	if maxScore <= minScore {
		return 1
	}
	return (score - minScore) / (maxScore - minScore)
}
