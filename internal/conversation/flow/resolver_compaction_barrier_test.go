package flow

import (
	"testing"
	"time"
)

func TestSessionCompactionWaitsForEveryOutboundAssetLinker(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	releaseFirst := resolver.DeferSessionCompaction("bot-1", "session-1")
	releaseSecond := resolver.DeferSessionCompaction("bot-1", "session-1")
	compactionEntered := make(chan struct{})
	releaseCompaction := make(chan struct{})
	go func() {
		done := resolver.enterSessionCompaction("bot-1", "session-1")
		close(compactionEntered)
		<-releaseCompaction
		done()
	}()

	releaseFirst()
	assertChannelBlocked(t, compactionEntered, "compaction started while one asset linker remained")
	releaseSecond()
	assertChannelReady(t, compactionEntered, "compaction did not start after every asset linker finished")
	close(releaseCompaction)
}

func TestWaitingSessionCompactionBlocksLaterAssetLinkers(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	releaseFirst := resolver.DeferSessionCompaction("bot-1", "session-1")
	compactionEntered := make(chan struct{})
	releaseCompaction := make(chan struct{})
	go func() {
		done := resolver.enterSessionCompaction("bot-1", "session-1")
		close(compactionEntered)
		<-releaseCompaction
		done()
	}()
	assertChannelBlocked(t, compactionEntered, "compaction ignored an active asset linker")

	secondEntered := make(chan func(), 1)
	go func() {
		secondEntered <- resolver.DeferSessionCompaction("bot-1", "session-1")
	}()
	releaseFirst()
	assertChannelReady(t, compactionEntered, "waiting compaction did not acquire the session gate")
	assertChannelBlocked(t, secondEntered, "later asset linker bypassed waiting compaction")
	close(releaseCompaction)

	var releaseSecond func()
	select {
	case releaseSecond = <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("later asset linker did not acquire the released session gate")
	}
	releaseSecond()
}

func assertChannelBlocked[T any](t *testing.T, ch <-chan T, message string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertChannelReady[T any](t *testing.T, ch <-chan T, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}
