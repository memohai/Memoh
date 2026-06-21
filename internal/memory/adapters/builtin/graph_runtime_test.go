package builtin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/migrate"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

var errForced = errors.New("forced store error for test")

// fakeWikiStore is an in-memory wikistore.Store for graphRuntime tests.
type fakeWikiStore struct {
	nodes map[string]migrate.NodeSpec // keyed by node ID
	edges map[string]migrate.EdgeSpec // keyed by src\0dst\0rel
}

func newFakeWikiStore() *fakeWikiStore {
	return &fakeWikiStore{nodes: map[string]migrate.NodeSpec{}, edges: map[string]migrate.EdgeSpec{}}
}

func (s *fakeWikiStore) UpsertNode(_ context.Context, node migrate.NodeSpec) (migrate.NodeSpec, error) {
	s.nodes[node.ID] = node
	return node, nil
}

func (s *fakeWikiStore) GetNode(_ context.Context, _, nodeID string) (migrate.NodeSpec, error) {
	if n, ok := s.nodes[nodeID]; ok {
		return n, nil
	}
	return migrate.NodeSpec{}, errForced
}

func (s *fakeWikiStore) ListNodes(_ context.Context, _ string) ([]migrate.NodeSpec, error) {
	out := make([]migrate.NodeSpec, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (s *fakeWikiStore) ListNodesByLayer(_ context.Context, _ string, layer migrate.MemoryLayer) ([]migrate.NodeSpec, error) {
	out := []migrate.NodeSpec{}
	for _, n := range s.nodes {
		if n.Layer == layer {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *fakeWikiStore) DeleteNode(_ context.Context, _, nodeID string) error {
	delete(s.nodes, nodeID)
	return nil
}

func (s *fakeWikiStore) DeleteAllNodes(_ context.Context, _ string) error {
	s.nodes = map[string]migrate.NodeSpec{}
	s.edges = map[string]migrate.EdgeSpec{}
	return nil
}

func (s *fakeWikiStore) CountNodes(_ context.Context, _ string) (int, error) {
	return len(s.nodes), nil
}

func (s *fakeWikiStore) UpsertEdges(_ context.Context, edges []migrate.EdgeSpec) error {
	for _, e := range edges {
		s.edges[e.SrcNode+"\x00"+e.DstNode+"\x00"+string(e.Rel)] = e
	}
	return nil
}

func (s *fakeWikiStore) ListEdges(_ context.Context, _ string) ([]migrate.EdgeSpec, error) {
	out := make([]migrate.EdgeSpec, 0, len(s.edges))
	for _, e := range s.edges {
		out = append(out, e)
	}
	return out, nil
}
func (*fakeWikiStore) DeleteEdgesForNode(_ context.Context, _, _ string) error { return nil }
func (s *fakeWikiStore) DeleteAllEdges(_ context.Context, _ string) error {
	s.edges = map[string]migrate.EdgeSpec{}
	return nil
}

func (s *fakeWikiStore) CountEdges(_ context.Context, _ string) (int, error) {
	return len(s.edges), nil
}

func (s *fakeWikiStore) RebuildImplicitEdges(_ context.Context, _ string) (int, error) {
	nodes := []migrate.NodeSpec{}
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	implicit := migrate.PlanFromNodes(nodes)
	for _, e := range implicit {
		s.edges[e.SrcNode+"\x00"+e.DstNode+"\x00"+string(e.Rel)] = e
	}
	return len(implicit), nil
}

func TestGraphRuntimeAddSearchDelete(t *testing.T) {
	t.Parallel()
	store := newFakeWikiStore()
	rt := newGraphRuntime(nil, store, newFakeStore())

	botID := "graph-bot-1"
	ctx := context.Background()

	// Add two memories sharing a profile_ref -> implicit same_profile edge.
	if _, err := rt.Add(ctx, adapters.AddRequest{
		BotID:    botID,
		Message:  "I prefer oolong tea",
		Metadata: map[string]any{"profile_ref": "user:1", "topic": "drinks"},
	}); err != nil {
		t.Fatalf("Add 1: %v", err)
	}
	if _, err := rt.Add(ctx, adapters.AddRequest{
		BotID:    botID,
		Message:  "I live in Berlin",
		Metadata: map[string]any{"profile_ref": "user:1", "topic": "location"},
	}); err != nil {
		t.Fatalf("Add 2: %v", err)
	}

	// GetAll reflects 2 nodes.
	all, err := rt.GetAll(ctx, adapters.GetAllRequest{BotID: botID})
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all.Results) != 2 {
		t.Fatalf("GetAll = %d, want 2", len(all.Results))
	}

	// Search "tea" seeds the oolong node; expansion reaches the Berlin node via
	// the shared-profile edge, so both surface and Relations is non-empty.
	resp, err := rt.Search(ctx, adapters.SearchRequest{BotID: botID, Query: "tea", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("Search returned no results")
	}
	if len(resp.Relations) == 0 {
		t.Fatal("Search Relations empty; expected the same_profile edge")
	}
	foundOolong := false
	for _, r := range resp.Results {
		if strings.Contains(r.Memory, "oolong") {
			foundOolong = true
		}
	}
	if !foundOolong {
		t.Fatal("Search did not surface the oolong memory")
	}

	// Status reports mode graph + node count.
	st, err := rt.Status(ctx, botID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.MemoryMode != "graph" {
		t.Fatalf("Status mode = %q, want graph", st.MemoryMode)
	}
	if st.SourceCount != 2 {
		t.Fatalf("Status node count = %d, want 2", st.SourceCount)
	}

	// Delete one node; count drops to 1.
	firstID := all.Results[0].ID
	if _, err := rt.Delete(ctx, firstID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	remaining, _ := rt.GetAll(ctx, adapters.GetAllRequest{BotID: botID})
	if len(remaining.Results) != 1 {
		t.Fatalf("after Delete, GetAll = %d, want 1", len(remaining.Results))
	}

	// DeleteAll clears everything.
	if _, err := rt.DeleteAll(ctx, adapters.DeleteAllRequest{BotID: botID}); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if n, _ := store.CountNodes(ctx, botID); n != 0 {
		t.Fatalf("after DeleteAll, node count = %d, want 0", n)
	}
}

func TestGraphRuntimeFileFallback(t *testing.T) {
	t.Parallel()
	// A wiki store that returns an error on ListNodes forces the file fallback.
	store := &errWikiStore{}
	fs := newFakeStore(
		storefs.MemoryItem{ID: "bot-x:m1", Memory: "fallback oolong memory", CreatedAt: time.Now().Format(time.RFC3339)},
	)
	rt := newGraphRuntime(nil, store, fs)

	resp, err := rt.Search(context.Background(), adapters.SearchRequest{
		BotID: "bot-x", Query: "oolong", Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search (fallback) should not error: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("file fallback returned no results")
	}
	if !strings.Contains(resp.Results[0].Memory, "oolong") {
		t.Fatalf("file fallback result = %q, want oolong", resp.Results[0].Memory)
	}
}

// errWikiStore returns an error on every read so the graph runtime degrades to
// the file-lexical fallback path.
type errWikiStore struct{}

func (errWikiStore) UpsertNode(context.Context, migrate.NodeSpec) (migrate.NodeSpec, error) {
	return migrate.NodeSpec{}, errForced
}

func (errWikiStore) GetNode(context.Context, string, string) (migrate.NodeSpec, error) {
	return migrate.NodeSpec{}, errForced
}

func (errWikiStore) ListNodes(context.Context, string) ([]migrate.NodeSpec, error) {
	return nil, errForced
}

func (errWikiStore) ListNodesByLayer(context.Context, string, migrate.MemoryLayer) ([]migrate.NodeSpec, error) {
	return nil, errForced
}
func (errWikiStore) DeleteNode(context.Context, string, string) error      { return nil }
func (errWikiStore) DeleteAllNodes(context.Context, string) error          { return nil }
func (errWikiStore) CountNodes(context.Context, string) (int, error)       { return 0, errForced }
func (errWikiStore) UpsertEdges(context.Context, []migrate.EdgeSpec) error { return nil }
func (errWikiStore) ListEdges(context.Context, string) ([]migrate.EdgeSpec, error) {
	return nil, errForced
}
func (errWikiStore) DeleteEdgesForNode(context.Context, string, string) error { return nil }
func (errWikiStore) DeleteAllEdges(context.Context, string) error             { return nil }
func (errWikiStore) CountEdges(context.Context, string) (int, error)          { return 0, errForced }
func (errWikiStore) RebuildImplicitEdges(context.Context, string) (int, error) {
	return 0, errForced
}
