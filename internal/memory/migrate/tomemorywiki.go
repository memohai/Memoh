// Package migrate contains utilities that convert existing markdown-backed
// memory content into the PostgreSQL/SQLite wiki/graph schema
// (memory_nodes + memory_edges). The conversion logic is backend-agnostic: it
// produces plain NodeSpec/EdgeSpec values so it can be exercised by unit tests
// without a live database, and persisted by any concrete store implementation.
package migrate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

// MemoryLayer is the canonical layer a memory node belongs to. Existing flat
// markdown entries that carry no layer hint are classified as LayerNote.
type MemoryLayer string

const (
	LayerPreference MemoryLayer = "preference"
	LayerIdentity   MemoryLayer = "identity"
	LayerContext    MemoryLayer = "context"
	LayerExperience MemoryLayer = "experience"
	LayerActivity   MemoryLayer = "activity"
	LayerPersona    MemoryLayer = "persona"
	LayerNote       MemoryLayer = "note"
)

// EdgeRel is the canonical relationship type between two memory nodes.
type EdgeRel string

const (
	EdgeSameProfile EdgeRel = "same_profile"
	EdgeSameTopic   EdgeRel = "same_topic"
	EdgeSameDay     EdgeRel = "same_day"
)

// NodeSpec is a backend-agnostic description of a memory_nodes row, produced
// from a storefs.MemoryItem. Concrete stores (PG/SQLite) translate this into
// their sqlc UpsertMemoryNodeParams.
type NodeSpec struct {
	ID               string
	BotID            string
	Body             string
	Hash             string
	Layer            MemoryLayer
	FactType         string
	Subject          string
	Confidence       float32
	Metadata         map[string]any
	SourceMessageIDs []string
	ProfileRef       string
	Topic            string
	CapturedAt       time.Time
	ExpiresAt        time.Time // zero value means "no expiry".
}

// EdgeSpec is a backend-agnostic description of a memory_edges row.
type EdgeSpec struct {
	BotID    string
	SrcNode  string
	DstNode  string
	Rel      EdgeRel
	Weight   float32
	Metadata map[string]any
}

// Result summarises a wiki backfill pass for one bot.
type Result struct {
	BotID      string
	NodeCount  int
	EdgeCount  int
	LayerBreak map[MemoryLayer]int
}

// Plan converts a bot's markdown memory items into wiki node specs plus the
// implicit edges derivable from shared profile_ref / topic / captured day.
// It performs no I/O and is safe to call in dry-run mode.
//
// Layer classification is intentionally conservative: items without any hint
// fall back to LayerNote. This keeps the backfill deterministic and reversible
// until the typed-facts formation (P1) starts emitting explicit layers.
func Plan(botID string, items []storefs.MemoryItem) ([]NodeSpec, []EdgeSpec) {
	nodes := make([]NodeSpec, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, nodeFromItem(botID, item))
	}
	edges := buildImplicitEdges(nodes)
	return nodes, edges
}

// Summarise returns a Result for a planned node/edge set, suitable for CLI
// dry-run reporting.
func Summarise(botID string, nodes []NodeSpec, edges []EdgeSpec) Result {
	r := Result{BotID: botID, NodeCount: len(nodes), EdgeCount: len(edges), LayerBreak: map[MemoryLayer]int{}}
	for _, n := range nodes {
		layer := n.Layer
		if layer == "" {
			layer = LayerNote
		}
		r.LayerBreak[layer]++
	}
	return r
}

func nodeFromItem(botID string, item storefs.MemoryItem) NodeSpec {
	body := strings.TrimSpace(item.Memory)
	layer := classifyLayer(item)
	topic := metadataString(item.Metadata, "topic")
	profileRef := metadataString(item.Metadata, "profile_ref")
	if profileRef == "" {
		profileRef = metadataString(item.Metadata, "profile_user_id")
	}
	captured := parseTime(item.CreatedAt)
	if captured.IsZero() {
		captured = parseTime(item.UpdatedAt)
	}
	if captured.IsZero() {
		captured = time.Now().UTC()
	}
	return NodeSpec{
		ID:         strings.TrimSpace(item.ID),
		BotID:      botID,
		Body:       body,
		Hash:       strings.TrimSpace(item.Hash),
		Layer:      layer,
		Subject:    metadataString(item.Metadata, "subject"),
		Confidence: metadataFloat(item.Metadata, "confidence", 0.5),
		Metadata:   cloneMetadata(item.Metadata),
		ProfileRef: profileRef,
		Topic:      topic,
		CapturedAt: captured,
	}
}

// classifyLayer maps a flat memory item to a canonical layer using light
// metadata heuristics. Items that already declare a `layer` metadata key are
// honoured (validated against the known set); otherwise the item defaults to
// LayerNote. This is deliberately non-magical: real classification happens
// later in the typed-facts formation (P1).
func classifyLayer(item storefs.MemoryItem) MemoryLayer {
	if raw := metadataString(item.Metadata, "layer"); raw != "" {
		switch MemoryLayer(strings.ToLower(strings.TrimSpace(raw))) {
		case LayerPreference, LayerIdentity, LayerContext, LayerExperience, LayerActivity, LayerPersona, LayerNote:
			return MemoryLayer(raw)
		}
	}
	return LayerNote
}

// buildImplicitEdges derives same_profile / same_topic / same_day edges between
// nodes. Edges are undirected in intent but stored as directed src->dst pairs
// where src < dst (lexicographically by node ID) to avoid duplicates. A node
// never edges to itself.
func buildImplicitEdges(nodes []NodeSpec) []EdgeSpec {
	if len(nodes) < 2 {
		return nil
	}
	byProfile := indexBy(nodes, func(n NodeSpec) string { return n.ProfileRef })
	byTopic := indexBy(nodes, func(n NodeSpec) string { return n.Topic })
	byDay := indexBy(nodes, func(n NodeSpec) string { return n.CapturedAt.UTC().Format("2006-01-02") })

	seen := map[string]struct{}{}
	edges := make([]EdgeSpec, 0)
	add := func(a, b NodeSpec, rel EdgeRel, weight float32) {
		if a.ID == b.ID {
			return
		}
		src, dst := a.ID, b.ID
		if dst < src {
			src, dst = dst, src
		}
		key := src + "\x00" + dst + "\x00" + string(rel)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		edges = append(edges, EdgeSpec{BotID: a.BotID, SrcNode: src, DstNode: dst, Rel: rel, Weight: weight})
	}

	emit := func(groups [][]NodeSpec, rel EdgeRel, weight float32) {
		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			for i := 0; i < len(group); i++ {
				for j := i + 1; j < len(group); j++ {
					add(group[i], group[j], rel, weight)
				}
			}
		}
	}
	emit(byProfile, EdgeSameProfile, 1.0)
	emit(byTopic, EdgeSameTopic, 0.8)
	emit(byDay, EdgeSameDay, 0.5)

	// Deterministic ordering keeps dry-run output stable.
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Rel != edges[j].Rel {
			return edges[i].Rel < edges[j].Rel
		}
		if edges[i].SrcNode != edges[j].SrcNode {
			return edges[i].SrcNode < edges[j].SrcNode
		}
		return edges[i].DstNode < edges[j].DstNode
	})
	return edges
}

// indexBy groups nodes sharing the same non-empty key.
func indexBy(nodes []NodeSpec, key func(NodeSpec) string) [][]NodeSpec {
	buckets := map[string][]NodeSpec{}
	order := []string{}
	for _, n := range nodes {
		k := strings.TrimSpace(key(n))
		if k == "" {
			continue
		}
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], n)
	}
	out := make([][]NodeSpec, 0, len(buckets))
	for _, k := range order {
		out = append(out, buckets[k])
	}
	return out
}

func metadataString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	default:
		return strings.TrimSpace(toString(v))
	}
}

func metadataFloat(m map[string]any, key string, def float32) float32 {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return clamp32(float32(n), def)
	case float32:
		return clamp32(n, def)
	case int:
		return clamp32(float32(n), def)
	case int64:
		return clamp32(float32(n), def)
	case string:
		return def
	default:
		return def
	}
}

func clamp32(v, def float32) float32 {
	if v < 0 || v > 1 {
		return def
	}
	return v
}

func cloneMetadata(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
