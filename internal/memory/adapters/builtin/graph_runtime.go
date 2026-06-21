//nolint:sloglint // graph runtime ops logs use inline key/value pairs for readability
package builtin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/config"
	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/migrate"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
	"github.com/memohai/memoh/internal/memory/wikistore"
)

// ModeGraph is the graph-first memory mode: PG memory_nodes/memory_edges are
// ModeGraph is the memory mode identifier for the graph runtime. It is the
// only supported mode: PG memory nodes/edges are the source of truth, with
// Markdown as a derived agent-facing view.
const ModeGraph = "graph"

// graphRuntime implements Runtime over the PG/SQLite wiki graph. The wiki
// store (wikistore.Store) is authoritative; the filesystem store (memoryStore)
// graphRuntime implements Runtime over the PG/SQLite wiki graph. The wiki
// store (wikistore.Store) is authoritative; the filesystem store (memoryStore)
// holds the derived Markdown view the agent reads.
type graphRuntime struct {
	store  wikistore.Store
	fs     memoryStore
	cache  *graphCache
	syncer *graphSync
	logger *slog.Logger
}

// NewGraphRuntime constructs a graphRuntime. wikiStore is required; fs is the
// derived-view filesystem store (may be nil in tests, which disables sync).
func NewGraphRuntime(logger *slog.Logger, wikiStore wikistore.Store, fs memoryStore) *graphRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	return &graphRuntime{
		store:  wikiStore,
		fs:     fs,
		cache:  newGraphCache(),
		syncer: newGraphSync(fs, logger),
		logger: logger.With("runtime", "graph"),
	}
}

func (*graphRuntime) Mode() string { return string(ModeGraph) }

// ---- helpers ----

func (r *graphRuntime) syncAndInvalidate(ctx context.Context, botID string) {
	// Regenerate the derived Markdown view from the authoritative PG nodes.
	// Best-effort: PG is the source of truth, so a sync failure is logged, not
	// propagated to the caller.
	if r.syncer == nil || r.fs == nil {
		r.cache.invalidate(botID)
		return
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		r.logger.Warn("graph: list nodes for sync failed", "bot_id", botID, "err", err)
	} else if err := r.syncer.syncMarkdownFromNodes(ctx, botID, nodes); err != nil {
		r.logger.Warn("graph: markdown sync failed", "bot_id", botID, "err", err)
	}
	if _, err := r.store.RebuildImplicitEdges(ctx, botID); err != nil {
		r.logger.Debug("graph: rebuild implicit edges failed", "bot_id", botID, "err", err)
	}
	r.cache.invalidate(botID)
}

// ---- Runtime: CRUD ----

func (r *graphRuntime) Add(ctx context.Context, req adapters.AddRequest) (adapters.SearchResponse, error) {
	if r.store == nil {
		return adapters.SearchResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	text := runtimeText(req.Message, req.Messages)
	if text == "" {
		return adapters.SearchResponse{}, errors.New("graph runtime: message is required")
	}
	now := time.Now().UTC()
	spec := memoryItemToNodeSpec(adapters.MemoryItem{
		ID:        runtimeMemoryID(botID, now),
		Memory:    text,
		Hash:      runtimeHash(text),
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
		Metadata:  req.Metadata,
	}, botID)

	saved, err := r.store.UpsertNode(ctx, spec)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("graph runtime: upsert node: %w", err)
	}
	r.syncAndInvalidate(ctx, botID)
	return adapters.SearchResponse{Results: []adapters.MemoryItem{nodeSpecToMemoryItem(saved)}}, nil
}

func (r *graphRuntime) Search(ctx context.Context, req adapters.SearchRequest) (adapters.SearchResponse, error) {
	if r.store == nil {
		return adapters.SearchResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// Primary path: graph seed-then-expand over the cached PG graph.
	resp, graphErr := r.searchGraph(ctx, botID, req.Query, limit)
	if graphErr == nil {
		return resp, nil
	}

	// Reliability fallback: degrade to file-lexical over the derived Markdown.
	r.logger.Warn("graph search failed, falling back to file lexical", "bot_id", botID, "err", graphErr)
	return r.searchFileFallback(ctx, botID, req.Query, limit)
}

// searchGraph runs seed-then-expand: lexical-score nodes -> top-K seeds ->
// BFS expand along edges -> merge -> populate Relations.
func (r *graphRuntime) searchGraph(ctx context.Context, botID, query string, limit int) (adapters.SearchResponse, error) {
	graph, err := r.cache.getOrBuild(ctx, botID, r.store)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	nodes := graph.nodeSlice()

	// 1. Seed: lexical-score every node body against the query.
	type seed struct {
		id    string
		score float64
	}
	seeds := make([]seed, 0, len(nodes))
	for _, n := range nodes {
		s := graphLexicalScore(query, n.Body)
		if s <= 0 && strings.TrimSpace(query) != "" {
			continue
		}
		seeds = append(seeds, seed{id: n.ID, score: s})
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].score > seeds[j].score })

	overfetch := limit * 3
	if overfetch < 10 {
		overfetch = 10
	}
	if len(seeds) > overfetch {
		seeds = seeds[:overfetch]
	}

	// 2. Expand: BFS depth 2 from seeds, weighting neighbors.
	const decay = 0.6
	type hit struct {
		score float64
	}
	scores := map[string]hit{}
	hitEdges := map[string]bool{}
	addEdge := func(a, b, rel string) {
		src, dst := a, b
		if dst < src {
			src, dst = dst, src
		}
		hitEdges[src+"\x00"+dst+"\x00"+rel] = true
	}
	for _, s := range seeds {
		scores[s.id] = hit{score: max64(scores[s.id].score, s.score)}
		// depth 1
		for _, nb := range graph.neighbors(s.id) {
			ns := float64(s.score) * float64(nb.weight) * decay
			scores[nb.node.ID] = hit{score: max64(scores[nb.node.ID].score, ns)}
			addEdge(s.id, nb.node.ID, string(nb.rel))
			// depth 2
			for _, nb2 := range graph.neighbors(nb.node.ID) {
				if nb2.node.ID == s.id {
					continue
				}
				ns2 := ns * decay
				scores[nb2.node.ID] = hit{score: max64(scores[nb2.node.ID].score, ns2)}
				addEdge(nb.node.ID, nb2.node.ID, string(nb2.rel))
			}
		}
	}

	// 3. Merge + sort + truncate.
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return scores[ids[i]].score > scores[ids[j]].score })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	results := make([]adapters.MemoryItem, 0, len(ids))
	for _, id := range ids {
		n, ok := graph.nodes[id]
		if !ok {
			continue
		}
		item := nodeSpecToMemoryItem(n)
		item.Score = scores[id].score
		results = append(results, item)
	}

	// 4. Relations (hit edges) for explain.
	relations := make([]any, 0, len(hitEdges))
	for k := range hitEdges {
		parts := strings.SplitN(k, "\x00", 3)
		if len(parts) == 3 {
			relations = append(relations, map[string]any{"from": parts[0], "to": parts[1], "rel": parts[2]})
		}
	}
	return adapters.SearchResponse{Results: results, Relations: relations}, nil
}

// searchFileFallback is the reliability fallback: read the derived Markdown via
// the bridge and score lexically, exactly like fileRuntime. Used when the PG
// graph is unavailable.
func (r *graphRuntime) searchFileFallback(ctx context.Context, botID, query string, limit int) (adapters.SearchResponse, error) {
	if r.fs == nil {
		return adapters.SearchResponse{}, nil
	}
	items, err := r.fs.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return adapters.SearchResponse{}, fmt.Errorf("graph runtime: file fallback read: %w", err)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	results := make([]adapters.MemoryItem, 0, len(items))
	for _, it := range items {
		score := graphLexicalScore(q, it.Memory)
		if q != "" && score <= 0 {
			continue
		}
		m := memoryItemFromStore(it)
		m.BotID = botID
		m.Score = score
		results = append(results, m)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].UpdatedAt > results[j].UpdatedAt
		}
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return adapters.SearchResponse{Results: results}, nil
}

func (r *graphRuntime) GetAll(ctx context.Context, req adapters.GetAllRequest) (adapters.SearchResponse, error) {
	if r.store == nil {
		return adapters.SearchResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.SearchResponse{}, err
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		// Fallback to derived files if the store is unavailable.
		r.logger.Warn("graph GetAll failed, falling back to files", "bot_id", botID, "err", err)
		return r.searchFileFallback(ctx, botID, "", req.Limit)
	}
	out := make([]adapters.MemoryItem, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeSpecToMemoryItem(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return adapters.SearchResponse{Results: out}, nil
}

func (r *graphRuntime) Update(ctx context.Context, req adapters.UpdateRequest) (adapters.MemoryItem, error) {
	if r.store == nil {
		return adapters.MemoryItem{}, errors.New("graph runtime: wiki store not configured")
	}
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		return adapters.MemoryItem{}, errors.New("graph runtime: memory_id is required")
	}
	text := strings.TrimSpace(req.Memory)
	if text == "" {
		return adapters.MemoryItem{}, errors.New("graph runtime: memory is required")
	}
	botID := runtimeBotIDFromMemoryID(memoryID)
	if botID == "" {
		return adapters.MemoryItem{}, errors.New("graph runtime: invalid memory_id")
	}
	existing, err := r.store.GetNode(ctx, botID, memoryID)
	if err != nil {
		return adapters.MemoryItem{}, fmt.Errorf("graph runtime: get node: %w", err)
	}
	existing.Body = text
	existing.Hash = runtimeHash(text)
	saved, err := r.store.UpsertNode(ctx, existing)
	if err != nil {
		return adapters.MemoryItem{}, fmt.Errorf("graph runtime: update node: %w", err)
	}
	r.syncAndInvalidate(ctx, botID)
	return nodeSpecToMemoryItem(saved), nil
}

func (r *graphRuntime) Delete(ctx context.Context, memoryID string) (adapters.DeleteResponse, error) {
	return r.DeleteBatch(ctx, []string{memoryID})
}

func (r *graphRuntime) DeleteBatch(ctx context.Context, memoryIDs []string) (adapters.DeleteResponse, error) {
	if r.store == nil {
		return adapters.DeleteResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	seen := map[string]bool{}
	for _, rawID := range memoryIDs {
		memoryID := strings.TrimSpace(rawID)
		if memoryID == "" || seen[memoryID] {
			continue
		}
		seen[memoryID] = true
		botID := runtimeBotIDFromMemoryID(memoryID)
		if botID == "" {
			continue
		}
		if err := r.store.DeleteNode(ctx, botID, memoryID); err != nil {
			return adapters.DeleteResponse{}, fmt.Errorf("graph runtime: delete node: %w", err)
		}
		r.syncAndInvalidate(ctx, botID)
	}
	return adapters.DeleteResponse{Message: "Memories deleted successfully!"}, nil
}

func (r *graphRuntime) DeleteAll(ctx context.Context, req adapters.DeleteAllRequest) (adapters.DeleteResponse, error) {
	if r.store == nil {
		return adapters.DeleteResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID(req.BotID, req.Filters)
	if err != nil {
		return adapters.DeleteResponse{}, err
	}
	if err := r.store.DeleteAllNodes(ctx, botID); err != nil {
		return adapters.DeleteResponse{}, fmt.Errorf("graph runtime: delete all nodes: %w", err)
	}
	if r.fs != nil {
		if err := r.fs.RemoveAllMemories(ctx, botID); err != nil {
			r.logger.Warn("graph: remove derived markdown failed", "bot_id", botID, "err", err)
		}
	}
	r.cache.invalidate(botID)
	return adapters.DeleteResponse{Message: "All memories deleted successfully!"}, nil
}

// ---- Runtime: compact ----

func (r *graphRuntime) Compact(ctx context.Context, filters map[string]any, ratio float64, _ int) (adapters.CompactResult, error) {
	if r.store == nil {
		return adapters.CompactResult{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return adapters.CompactResult{}, errors.New("ratio must be in range (0, 1]")
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		return adapters.CompactResult{}, fmt.Errorf("graph runtime: list nodes: %w", err)
	}
	before := len(nodes)
	if before == 0 {
		return adapters.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []adapters.MemoryItem{}}, nil
	}
	// Keep newest target nodes, drop the oldest tail.
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].CapturedAt.After(nodes[j].CapturedAt) })
	target := int(float64(before) * ratio)
	if target < 1 {
		target = 1
	}
	if target > before {
		target = before
	}
	for _, n := range nodes[target:] {
		if err := r.store.DeleteNode(ctx, botID, n.ID); err != nil {
			return adapters.CompactResult{}, fmt.Errorf("graph runtime: compact delete: %w", err)
		}
	}
	kept := nodes[:target]
	r.syncAndInvalidate(ctx, botID)
	items := make([]adapters.MemoryItem, 0, len(kept))
	for _, n := range kept {
		items = append(items, nodeSpecToMemoryItem(n))
	}
	return adapters.CompactResult{BeforeCount: before, AfterCount: len(kept), Ratio: ratio, Results: items}, nil
}

// CompactWithLLM lets the builtin provider advertise semantic compact
// capability for the graph runtime. It summarizes the node set via the LLM,
// upserts the resulting facts as fresh nodes, and deletes the originals.
func (r *graphRuntime) CompactWithLLM(ctx context.Context, filters map[string]any, ratio float64, decayDays int, llm adapters.LLM) (adapters.CompactResult, error) {
	if r.store == nil {
		return adapters.CompactResult{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return adapters.CompactResult{}, errors.New("ratio must be in range (0, 1]")
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		return adapters.CompactResult{}, fmt.Errorf("graph runtime: list nodes: %w", err)
	}
	before := len(nodes)
	if before == 0 {
		return adapters.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []adapters.MemoryItem{}}, nil
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].CapturedAt.After(nodes[j].CapturedAt) })

	// Build compact candidates as store items for the shared compact helper.
	storeItems := make([]storefs.MemoryItem, 0, before)
	for _, n := range nodes {
		storeItems = append(storeItems, storefs.MemoryItem{
			ID:        n.ID,
			Memory:    n.Body,
			Hash:      n.Hash,
			CreatedAt: formatNodeTime(n.CapturedAt),
			UpdatedAt: formatNodeTime(n.CapturedAt),
			Metadata:  buildNodeMetadata(n),
		})
	}
	compacted, _, err := compactStoreItemsWithLLM(ctx, botID, storeItems, ratio, decayDays, llm)
	if err != nil {
		return adapters.CompactResult{}, fmt.Errorf("graph runtime: llm compact: %w", err)
	}

	// Reconcile PG: delete all originals, upsert the compacted facts as new nodes.
	if err := r.store.DeleteAllNodes(ctx, botID); err != nil {
		return adapters.CompactResult{}, fmt.Errorf("graph runtime: compact clear: %w", err)
	}
	now := time.Now().UTC()
	for _, fact := range compacted {
		spec := memoryItemToNodeSpec(adapters.MemoryItem{
			ID:        runtimeMemoryID(botID, now),
			Memory:    fact.Memory,
			Hash:      runtimeHash(fact.Memory),
			CreatedAt: now.Format(time.RFC3339),
			UpdatedAt: now.Format(time.RFC3339),
			Metadata:  fact.Metadata,
		}, botID)
		if _, err := r.store.UpsertNode(ctx, spec); err != nil {
			r.logger.Warn("graph: compact upsert failed", "bot_id", botID, "err", err)
		}
	}
	keptNodes, _ := r.store.ListNodes(ctx, botID)
	r.syncAndInvalidate(ctx, botID)
	items := make([]adapters.MemoryItem, 0, len(keptNodes))
	for _, n := range keptNodes {
		items = append(items, nodeSpecToMemoryItem(n))
	}
	return adapters.CompactResult{BeforeCount: before, AfterCount: len(keptNodes), Ratio: ratio, Results: items}, nil
}

// ---- Runtime: usage / status / rebuild ----

func (r *graphRuntime) Usage(ctx context.Context, filters map[string]any) (adapters.UsageResponse, error) {
	if r.store == nil {
		return adapters.UsageResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return adapters.UsageResponse{}, err
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		return adapters.UsageResponse{}, fmt.Errorf("graph runtime: usage list: %w", err)
	}
	var usage adapters.UsageResponse
	usage.Count = len(nodes)
	for _, n := range nodes {
		usage.TotalTextBytes += int64(len(n.Body))
	}
	if usage.Count > 0 {
		usage.AvgTextBytes = usage.TotalTextBytes / int64(usage.Count)
	}
	usage.EstimatedStorageBytes = usage.TotalTextBytes
	return usage, nil
}

func (r *graphRuntime) Status(ctx context.Context, botID string) (adapters.MemoryStatusResponse, error) {
	if r.store == nil {
		return adapters.MemoryStatusResponse{}, errors.New("graph runtime: wiki store not configured")
	}
	nodeCount, _ := r.store.CountNodes(ctx, botID)
	edgeCount, _ := r.store.CountEdges(ctx, botID)
	resp := adapters.MemoryStatusResponse{
		ProviderType:  BuiltinType,
		MemoryMode:    string(ModeGraph),
		CanManualSync: true,
		SourceDir:     path.Join(config.DefaultDataMount, "memory"),
		OverviewPath:  path.Join(config.DefaultDataMount, "MEMORY.md"),
		SourceCount:   nodeCount,
		IndexedCount:  edgeCount,
	}
	if r.fs != nil {
		if fc, err := r.fs.CountMemoryFiles(ctx, botID); err == nil {
			resp.MarkdownFileCount = fc
		}
	}
	return resp, nil
}

func (r *graphRuntime) Rebuild(ctx context.Context, botID string) (adapters.RebuildResult, error) {
	if r.store == nil {
		return adapters.RebuildResult{}, errors.New("graph runtime: wiki store not configured")
	}
	nodes, err := r.store.ListNodes(ctx, botID)
	if err != nil {
		return adapters.RebuildResult{}, fmt.Errorf("graph runtime: rebuild list: %w", err)
	}
	r.syncAndInvalidate(ctx, botID)
	count, _ := r.store.CountNodes(ctx, botID)
	return adapters.RebuildResult{FsCount: len(nodes), StorageCount: count}, nil
}

// ---- node/memory item conversion ----

// memoryItemToNodeSpec derives a NodeSpec from a MemoryItem, classifying the
// layer from metadata (defaulting to note) and pulling profile_ref/topic out.
func memoryItemToNodeSpec(item adapters.MemoryItem, botID string) migrate.NodeSpec {
	body := strings.TrimSpace(item.Memory)
	layer := migrate.LayerNote
	if raw, ok := item.Metadata["layer"].(string); ok && raw != "" {
		switch migrate.MemoryLayer(strings.ToLower(strings.TrimSpace(raw))) {
		case migrate.LayerPreference, migrate.LayerIdentity, migrate.LayerContext,
			migrate.LayerExperience, migrate.LayerActivity, migrate.LayerPersona, migrate.LayerNote:
			layer = migrate.MemoryLayer(raw)
		}
	}
	profileRef := metadataStringVal(item.Metadata, "profile_ref")
	if profileRef == "" {
		profileRef = metadataStringVal(item.Metadata, "profile_user_id")
	}
	return migrate.NodeSpec{
		ID:         strings.TrimSpace(item.ID),
		BotID:      botID,
		Body:       body,
		Hash:       strings.TrimSpace(item.Hash),
		Layer:      layer,
		Subject:    metadataStringVal(item.Metadata, "subject"),
		Confidence: metadataFloatVal(item.Metadata, "confidence", 0.5),
		Metadata:   item.Metadata,
		ProfileRef: profileRef,
		Topic:      metadataStringVal(item.Metadata, "topic"),
		CapturedAt: parseGraphTime(item.CreatedAt),
	}
}

func metadataStringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func metadataFloatVal(m map[string]any, key string, def float32) float32 {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		f := float32(n)
		if f < 0 || f > 1 {
			return def
		}
		return f
	case float32:
		if n < 0 || n > 1 {
			return def
		}
		return n
	}
	return def
}

func parseGraphTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC()
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
