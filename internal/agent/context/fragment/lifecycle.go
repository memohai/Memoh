package contextfrag

import (
	"strings"
	"sync"
)

const MetadataContextLifecycleKey = "context_lifecycle"

// LifecycleSnapshot is the durable, content-light description of the context
// view used by a run. It intentionally excludes manifest items and payloads.
type LifecycleSnapshot struct {
	Version            int            `json:"version"`
	View               ManifestView   `json:"view,omitempty"`
	Counts             ManifestCounts `json:"counts"`
	AssistantMessageID string         `json:"assistant_message_id,omitempty"`
}

// LifecycleHolder keeps the latest context snapshot shared by the copied
// RunConfig values that participate in one run.
type LifecycleHolder struct {
	mu       sync.RWMutex
	snapshot LifecycleSnapshot
	set      bool
}

func NewLifecycleHolder() *LifecycleHolder {
	return &LifecycleHolder{}
}

func (h *LifecycleHolder) SetManifest(manifest Manifest) {
	if h == nil {
		return
	}
	next := BuildLifecycleSnapshot(manifest)
	h.mu.Lock()
	next.AssistantMessageID = h.snapshot.AssistantMessageID
	h.snapshot = next
	h.set = true
	h.mu.Unlock()
}

func (h *LifecycleHolder) SetAssistantMessageID(messageID string) {
	if h == nil {
		return
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	h.mu.Lock()
	h.snapshot.AssistantMessageID = messageID
	h.mu.Unlock()
}

func (h *LifecycleHolder) Snapshot() (LifecycleSnapshot, bool) {
	if h == nil {
		return LifecycleSnapshot{}, false
	}
	h.mu.RLock()
	snapshot := h.snapshot
	ok := h.set
	h.mu.RUnlock()
	if !ok {
		return LifecycleSnapshot{}, false
	}
	return snapshot, true
}

func BuildLifecycleSnapshot(manifest Manifest) LifecycleSnapshot {
	return LifecycleSnapshot{
		Version: 1,
		View:    manifest.View,
		Counts:  manifest.Counts,
	}
}
