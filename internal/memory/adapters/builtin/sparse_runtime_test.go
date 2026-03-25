package builtin

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"testing"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	qdrantclient "github.com/memohai/memoh/internal/memory/qdrant"
	"github.com/memohai/memoh/internal/memory/sparse"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

type fakeSparseStore struct {
	items map[string]storefs.MemoryItem
}

func newFakeSparseStore(items ...storefs.MemoryItem) *fakeSparseStore {
	store := &fakeSparseStore{items: map[string]storefs.MemoryItem{}}
	for _, item := range items {
		store.items[item.ID] = item
	}
	return store
}

func (s *fakeSparseStore) PersistMemories(_ context.Context, _ string, items []storefs.MemoryItem, _ map[string]any) error {
	for _, item := range items {
		s.items[item.ID] = item
	}
	return nil
}

func (s *fakeSparseStore) ReadAllMemoryFiles(_ context.Context, _ string) ([]storefs.MemoryItem, error) {
	out := make([]storefs.MemoryItem, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *fakeSparseStore) RemoveMemories(_ context.Context, _ string, ids []string) error {
	for _, id := range ids {
		delete(s.items, strings.TrimSpace(id))
	}
	return nil
}

func (s *fakeSparseStore) RemoveAllMemories(_ context.Context, _ string) error {
	s.items = map[string]storefs.MemoryItem{}
	return nil
}

func (s *fakeSparseStore) RebuildFiles(_ context.Context, _ string, items []storefs.MemoryItem, _ map[string]any) error {
	s.items = map[string]storefs.MemoryItem{}
	for _, item := range items {
		s.items[item.ID] = item
	}
	return nil
}

func (*fakeSparseStore) SyncOverview(context.Context, string) error { return nil }

func (s *fakeSparseStore) CountMemoryFiles(_ context.Context, _ string) (int, error) {
	if len(s.items) == 0 {
		return 0, nil
	}
	return 1, nil
}

type fakeSparseEncoder struct {
	lastQuery string
}

func (*fakeSparseEncoder) EncodeDocument(_ context.Context, _ string) (*sparse.SparseVector, error) {
	return &sparse.SparseVector{Indices: []uint32{1, 2, 3}, Values: []float32{1, 3, 2}}, nil
}

func (*fakeSparseEncoder) EncodeDocuments(_ context.Context, texts []string) ([]sparse.SparseVector, error) {
	out := make([]sparse.SparseVector, 0, len(texts))
	for _, text := range texts {
		_ = text
		out = append(out, sparse.SparseVector{Indices: []uint32{1, 2, 3}, Values: []float32{1, 3, 2}})
	}
	return out, nil
}

func (e *fakeSparseEncoder) EncodeQuery(_ context.Context, text string) (*sparse.SparseVector, error) {
	e.lastQuery = text
	return &sparse.SparseVector{Indices: []uint32{9}, Values: []float32{1}}, nil
}

func (*fakeSparseEncoder) Health(context.Context) error { return nil }

type fakeSparseIndex struct {
	encoder    *fakeSparseEncoder
	collection string
	exists     bool
	points     map[string]qdrantclient.SearchResult
}

func newFakeSparseIndex(encoder *fakeSparseEncoder) *fakeSparseIndex {
	return &fakeSparseIndex{
		encoder:    encoder,
		collection: "memory_sparse_test",
		points:     map[string]qdrantclient.SearchResult{},
	}
}

func (i *fakeSparseIndex) CollectionName() string { return i.collection }

func (i *fakeSparseIndex) CollectionExists(context.Context) (bool, error) { return i.exists, nil }

func (i *fakeSparseIndex) EnsureCollection(context.Context) error {
	i.exists = true
	return nil
}

func (i *fakeSparseIndex) Upsert(_ context.Context, id string, _ qdrantclient.SparseVector, payload map[string]string) error {
	i.exists = true
	i.points[id] = qdrantclient.SearchResult{
		ID:      id,
		Score:   1,
		Payload: payload,
	}
	return nil
}

func (i *fakeSparseIndex) Search(_ context.Context, _ qdrantclient.SparseVector, botID string, limit int) ([]qdrantclient.SearchResult, error) {
	query := strings.ToLower(strings.TrimSpace(i.encoder.lastQuery))
	results := make([]qdrantclient.SearchResult, 0, len(i.points))
	for _, point := range i.points {
		if strings.TrimSpace(point.Payload["bot_id"]) != strings.TrimSpace(botID) {
			continue
		}
		text := strings.ToLower(point.Payload["memory"])
		if query != "" && !strings.Contains(text, query) {
			continue
		}
		point.Score = 1
		results = append(results, point)
	}
	sort.Slice(results, func(a, b int) bool { return results[a].ID < results[b].ID })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (i *fakeSparseIndex) Scroll(_ context.Context, botID string, limit int) ([]qdrantclient.SearchResult, error) {
	results := make([]qdrantclient.SearchResult, 0, len(i.points))
	for _, point := range i.points {
		if strings.TrimSpace(point.Payload["bot_id"]) != strings.TrimSpace(botID) {
			continue
		}
		results = append(results, point)
	}
	sort.Slice(results, func(a, b int) bool { return results[a].ID < results[b].ID })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (i *fakeSparseIndex) Count(_ context.Context, botID string) (int, error) {
	count := 0
	for _, point := range i.points {
		if strings.TrimSpace(point.Payload["bot_id"]) == strings.TrimSpace(botID) {
			count++
		}
	}
	return count, nil
}

func (i *fakeSparseIndex) DeleteByIDs(_ context.Context, ids []string) error {
	for _, id := range ids {
		delete(i.points, strings.TrimSpace(id))
	}
	return nil
}

func (i *fakeSparseIndex) DeleteByBotID(_ context.Context, botID string) error {
	for id, point := range i.points {
		if strings.TrimSpace(point.Payload["bot_id"]) == strings.TrimSpace(botID) {
			delete(i.points, id)
		}
	}
	return nil
}

func TestSparseRuntimeAddWritesSourceAndSupportsRecall(t *testing.T) {
	t.Parallel()

	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore()
	runtime := &sparseRuntime{
		qdrant:  index,
		encoder: encoder,
		store:   store,
	}

	resp, err := runtime.Add(context.Background(), adapters.AddRequest{
		BotID:   "bot-1",
		Message: "Ran likes oolong tea",
		Filters: map[string]any{"scopeId": "bot-1"},
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 add result, got %d", len(resp.Results))
	}
	item := resp.Results[0]
	if item.ID == "" {
		t.Fatal("expected source memory id to be populated")
	}
	if _, ok := store.items[item.ID]; !ok {
		t.Fatalf("expected memory %q to be written to markdown source", item.ID)
	}
	point, ok := index.points[runtimePointID("bot-1", item.ID)]
	if !ok {
		t.Fatalf("expected qdrant point for source memory %q", item.ID)
	}
	if point.Payload["source_entry_id"] != item.ID {
		t.Fatalf("expected source_entry_id payload %q, got %q", item.ID, point.Payload["source_entry_id"])
	}
	if len(item.TopKBuckets) != 0 || len(item.CDFCurve) != 0 {
		t.Fatalf("expected add response to skip explain stats, got %#v", item)
	}

	searchResp, err := runtime.Search(context.Background(), adapters.SearchRequest{
		BotID: "bot-1",
		Query: "oolong tea",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(searchResp.Results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(searchResp.Results))
	}
	if searchResp.Results[0].ID != item.ID {
		t.Fatalf("expected search result id %q, got %q", item.ID, searchResp.Results[0].ID)
	}
	if len(searchResp.Results[0].TopKBuckets) != 0 || len(searchResp.Results[0].CDFCurve) != 0 {
		t.Fatalf("expected search result to skip explain stats, got %#v", searchResp.Results[0])
	}
}

func TestSparseRuntimeRebuildSyncsSourceAndRemovesStalePoints(t *testing.T) {
	t.Parallel()

	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore(
		storefs.MemoryItem{
			ID:        "bot-1:mem_1",
			Memory:    "Ran likes tea",
			Hash:      runtimeHash("Ran likes tea"),
			CreatedAt: "2026-03-13T09:00:00Z",
			UpdatedAt: "2026-03-13T09:00:00Z",
		},
		storefs.MemoryItem{
			ID:        "bot-1:mem_2",
			Memory:    "Ran works in Berlin",
			Hash:      runtimeHash("Ran works in Berlin"),
			CreatedAt: "2026-03-13T10:00:00Z",
			UpdatedAt: "2026-03-13T10:00:00Z",
		},
	)
	runtime := &sparseRuntime{
		qdrant:  index,
		encoder: encoder,
		store:   store,
	}

	index.points[runtimePointID("bot-1", "bot-1:mem_1")] = qdrantclient.SearchResult{
		ID: runtimePointID("bot-1", "bot-1:mem_1"),
		Payload: map[string]string{
			"bot_id":          "bot-1",
			"memory":          "Ran likes tea",
			"source_entry_id": "bot-1:mem_1",
			"hash":            "outdated",
			"created_at":      "2026-03-13T09:00:00Z",
			"updated_at":      "2026-03-13T09:00:00Z",
		},
	}
	index.points[runtimePointID("bot-1", "bot-1:stale")] = qdrantclient.SearchResult{
		ID: runtimePointID("bot-1", "bot-1:stale"),
		Payload: map[string]string{
			"bot_id":          "bot-1",
			"memory":          "stale memory",
			"source_entry_id": "bot-1:stale",
		},
	}

	result, err := runtime.Rebuild(context.Background(), "bot-1")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if result.FsCount != 2 {
		t.Fatalf("expected fs_count=2, got %d", result.FsCount)
	}
	if result.StorageCount != 2 {
		t.Fatalf("expected storage_count=2, got %d", result.StorageCount)
	}
	if result.MissingCount != 1 {
		t.Fatalf("expected missing_count=1, got %d", result.MissingCount)
	}
	if result.RestoredCount != 2 {
		t.Fatalf("expected restored_count=2, got %d", result.RestoredCount)
	}
	if _, ok := index.points[runtimePointID("bot-1", "bot-1:stale")]; ok {
		t.Fatal("expected stale qdrant point to be removed")
	}
}

func TestSparseRuntimeGetAllIncludesExplainStats(t *testing.T) {
	t.Parallel()

	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore(
		storefs.MemoryItem{
			ID:        "bot-1:mem_1",
			Memory:    "Ran likes tea",
			Hash:      runtimeHash("Ran likes tea"),
			CreatedAt: "2026-03-13T09:00:00Z",
			UpdatedAt: "2026-03-13T09:00:00Z",
		},
	)
	runtime := &sparseRuntime{
		qdrant:  index,
		encoder: encoder,
		store:   store,
	}

	resp, err := runtime.GetAll(context.Background(), adapters.GetAllRequest{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if len(resp.Results[0].TopKBuckets) == 0 || len(resp.Results[0].CDFCurve) == 0 {
		t.Fatalf("expected get all result to include explain stats, got %#v", resp.Results[0])
	}
	if resp.Results[0].TopKBuckets[0].Index != 2 {
		t.Fatalf("expected top bucket index 2, got %d", resp.Results[0].TopKBuckets[0].Index)
	}
	if got := resp.Results[0].CDFCurve[len(resp.Results[0].CDFCurve)-1].Cumulative; got != 1 {
		t.Fatalf("expected final CDF cumulative to be 1, got %v", got)
	}
}

func TestBuiltinProviderMultiTurnRecallUsesSparseSourceRuntime(t *testing.T) {
	t.Parallel()

	encoder := &fakeSparseEncoder{}
	index := newFakeSparseIndex(encoder)
	store := newFakeSparseStore()
	runtime := &sparseRuntime{
		qdrant:  index,
		encoder: encoder,
		store:   store,
	}
	provider := NewBuiltinProvider(slog.Default(), runtime, nil, nil)

	err := provider.OnAfterChat(context.Background(), adapters.AfterChatRequest{
		BotID: "bot-1",
		Messages: []adapters.Message{
			{Role: "user", Content: "I like oolong tea."},
			{Role: "assistant", Content: "Noted, you like oolong tea."},
		},
	})
	if err != nil {
		t.Fatalf("OnAfterChat round 1 error = %v", err)
	}
	err = provider.OnAfterChat(context.Background(), adapters.AfterChatRequest{
		BotID: "bot-1",
		Messages: []adapters.Message{
			{Role: "user", Content: "I am based in Berlin."},
			{Role: "assistant", Content: "Understood, you are based in Berlin."},
		},
	})
	if err != nil {
		t.Fatalf("OnAfterChat round 2 error = %v", err)
	}

	before, err := provider.OnBeforeChat(context.Background(), adapters.BeforeChatRequest{
		BotID: "bot-1",
		Query: "berlin",
	})
	if err != nil {
		t.Fatalf("OnBeforeChat() error = %v", err)
	}
	if before == nil || !strings.Contains(strings.ToLower(before.ContextText), "berlin") {
		t.Fatalf("expected recalled context to mention berlin, got %#v", before)
	}

	before, err = provider.OnBeforeChat(context.Background(), adapters.BeforeChatRequest{
		BotID: "bot-1",
		Query: "oolong tea",
	})
	if err != nil {
		t.Fatalf("OnBeforeChat() tea error = %v", err)
	}
	if before == nil || !strings.Contains(strings.ToLower(before.ContextText), "oolong tea") {
		t.Fatalf("expected recalled context to mention oolong tea, got %#v", before)
	}
}
