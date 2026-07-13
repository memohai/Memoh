package flow

import (
	"strings"
	"sync"
)

func (r *Resolver) DeferStreamCompaction(streamID string) func() {
	streamID = strings.TrimSpace(streamID)
	if r == nil || streamID == "" {
		return func() {}
	}

	ready := make(chan struct{})
	r.streamCompactionMu.Lock()
	if r.streamCompactions == nil {
		r.streamCompactions = make(map[string]chan struct{})
	}
	r.streamCompactions[streamID] = ready
	r.streamCompactionMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.streamCompactionMu.Lock()
			if r.streamCompactions[streamID] == ready {
				delete(r.streamCompactions, streamID)
			}
			r.streamCompactionMu.Unlock()
			close(ready)
		})
	}
}

func (r *Resolver) waitForStreamCompaction(streamID string) {
	streamID = strings.TrimSpace(streamID)
	if r == nil || streamID == "" {
		return
	}
	r.streamCompactionMu.Lock()
	ready := r.streamCompactions[streamID]
	r.streamCompactionMu.Unlock()
	if ready != nil {
		<-ready
	}
}
