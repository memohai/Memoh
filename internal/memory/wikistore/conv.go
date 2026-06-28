package wikistore

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/memory/migrate"
)

// ---- NodeSpec <-> wire record helpers (shared by both backends) ----

// nodeToRecord flattens a NodeSpec into the internal record shape. Both
// backend implementations call this before building their driver-specific
// sqlc params, so the metadata JSON marshalling stays in one place.
func nodeToRecord(n migrate.NodeSpec) record {
	return record{
		ID:               n.ID,
		BotID:            n.BotID,
		Body:             n.Body,
		Hash:             n.Hash,
		Layer:            string(orDefaultLayer(n.Layer)),
		FactType:         n.FactType,
		Subject:          n.Subject,
		Confidence:       clampConfidence(n.Confidence),
		Metadata:         n.Metadata,
		SourceMessageIDs: n.SourceMessageIDs,
		ProfileRef:       n.ProfileRef,
		Topic:            n.Topic,
		CapturedAt:       n.CapturedAt,
		ExpiresAt:        n.ExpiresAt,
	}
}

func recordToNode(r record) migrate.NodeSpec {
	return migrate.NodeSpec{
		ID:               r.ID,
		BotID:            r.BotID,
		Body:             r.Body,
		Hash:             r.Hash,
		Layer:            migrate.MemoryLayer(r.Layer),
		FactType:         r.FactType,
		Subject:          r.Subject,
		Confidence:       r.Confidence,
		Metadata:         r.Metadata,
		SourceMessageIDs: r.SourceMessageIDs,
		ProfileRef:       r.ProfileRef,
		Topic:            r.Topic,
		CapturedAt:       r.CapturedAt,
		ExpiresAt:        r.ExpiresAt,
	}
}

func orDefaultLayer(layer migrate.MemoryLayer) migrate.MemoryLayer {
	if layer = migrate.MemoryLayer(strings.TrimSpace(string(layer))); layer == "" {
		return migrate.LayerNote
	}
	return layer
}

func clampConfidence(c float32) float32 {
	if c < 0 || c > 1 {
		return 0.5
	}
	return c
}

// marshalJSON serialises a metadata map to a JSON byte slice (empty -> "{}").
func marshalJSON(m map[string]any) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// unmarshalMetadata parses a JSON byte slice into a map[string]any.
func unmarshalMetadata(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// marshalStringList serialises a string slice to a JSON array byte slice.
func marshalStringList(s []string) []byte {
	if len(s) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(s)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func unmarshalStringList(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var s []string
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return s
}

// marshalStringJSON marshals for the SQLite backend, which stores JSONB as TEXT.
func marshalStringJSON(m map[string]any) string { return string(marshalJSON(m)) }

func marshalStringListJSON(s []string) string { return string(marshalStringList(s)) }

// parseTime parses an RFC3339-ish timestamp; returns zero on failure.
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
