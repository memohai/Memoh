package agent

import (
	"fmt"
	"sync"
	"testing"
)

func TestToolAbortRegistryConcurrentAccess(t *testing.T) {
	registry := newToolAbortRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("call-%d", i)
			registry.Add(id)
			if !registry.Any() {
				t.Error("expected registry to report pending aborts")
			}
			if !registry.Take(id) {
				t.Errorf("expected to take %s", id)
			}
		}(i)
	}
	wg.Wait()

	if registry.Any() {
		t.Fatal("expected registry to be empty")
	}
}
