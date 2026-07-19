package flow

import (
	"sync"
	"testing"
	"time"
)

func TestCompactionSchedulerCoalescesWaitingRequestsToLatest(t *testing.T) {
	t.Parallel()

	started := make(chan int, 3)
	release := make(chan struct{}, 3)
	var mu sync.Mutex
	var completed []int
	scheduler := newCompactionScheduler(func(request scheduledCompaction) bool {
		started <- request.inputTokens
		<-release
		mu.Lock()
		completed = append(completed, request.inputTokens)
		mu.Unlock()
		return false
	})

	scheduler.Schedule("session", scheduledCompaction{inputTokens: 100})
	assertScheduledCompaction(t, started, 100)
	scheduler.Schedule("session", scheduledCompaction{inputTokens: 200})
	scheduler.Schedule("session", scheduledCompaction{inputTokens: 300})
	release <- struct{}{}
	assertScheduledCompaction(t, started, 300)
	release <- struct{}{}

	deadline := time.Now().Add(time.Second)
	for scheduler.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if scheduler.Active() != 0 {
		t.Fatal("compaction scheduler did not become idle")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(completed) != 2 || completed[0] != 100 || completed[1] != 300 {
		t.Fatalf("completed compactions = %#v, want [100 300]", completed)
	}
}

func TestCompactionSchedulerDropsStaleWaitingRequestAfterSuccessfulCompaction(t *testing.T) {
	t.Parallel()

	started := make(chan int, 2)
	release := make(chan struct{})
	scheduler := newCompactionScheduler(func(request scheduledCompaction) bool {
		started <- request.inputTokens
		<-release
		return true
	})

	scheduler.Schedule("session", scheduledCompaction{inputTokens: 100})
	assertScheduledCompaction(t, started, 100)
	scheduler.Schedule("session", scheduledCompaction{inputTokens: 100})
	close(release)

	deadline := time.Now().Add(time.Second)
	for scheduler.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	select {
	case got := <-started:
		t.Fatalf("started stale compaction = %d, want no second run", got)
	default:
	}
}

func assertScheduledCompaction(t *testing.T, started <-chan int, want int) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("started compaction = %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for compaction %d", want)
	}
}
