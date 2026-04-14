package agent

import "sync"

type toolAbortRegistry struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func newToolAbortRegistry() *toolAbortRegistry {
	return &toolAbortRegistry{
		ids: make(map[string]struct{}),
	}
}

func (r *toolAbortRegistry) Add(toolCallID string) {
	if r == nil || toolCallID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids[toolCallID] = struct{}{}
}

func (r *toolAbortRegistry) Take(toolCallID string) bool {
	if r == nil || toolCallID == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.ids[toolCallID]; !ok {
		return false
	}
	delete(r.ids, toolCallID)
	return true
}

func (r *toolAbortRegistry) Any() bool {
	if r == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ids) > 0
}
