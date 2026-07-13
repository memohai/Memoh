package flow

import (
	"testing"
	"time"
)

func TestCompactionBarrierWaitsForOutboundAssetLinking(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	release := resolver.DeferStreamCompaction("stream-1")
	unblocked := make(chan struct{})
	go func() {
		resolver.waitForStreamCompaction("stream-1")
		close(unblocked)
	}()

	select {
	case <-unblocked:
		t.Fatal("compaction barrier opened before outbound assets were linked")
	case <-time.After(20 * time.Millisecond):
	}

	release()
	select {
	case <-unblocked:
	case <-time.After(time.Second):
		t.Fatal("compaction barrier stayed closed after outbound assets were linked")
	}
}

func TestCompactionBarrierAllowsLateWaiterAfterRelease(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	release := resolver.DeferStreamCompaction("stream-1")
	release()

	done := make(chan struct{})
	go func() {
		resolver.waitForStreamCompaction("stream-1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("released compaction barrier blocked a late waiter")
	}
}
