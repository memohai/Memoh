package sessionruntime

import "context"

// runStateBackend owns the persistence invariants shared by every Manager
// operation on an active run. Local and distributed backends deliberately use
// different implementations so MemoryBackend does not emulate leases or
// cross-process ownership.
type runStateBackend interface {
	StartRun(ctx context.Context, key Key, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error)
	UpdateActiveRun(ctx context.Context, handle RunHandle, update ActiveRunUpdate) (Snapshot, bool, error)
	ReleaseRun(ctx context.Context, handle RunHandle, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error)
}

type localRunStateBackend struct {
	backend Backend
}

func (b localRunStateBackend) StartRun(ctx context.Context, key Key, _ StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.backend.Update(ctx, key, func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		now, err := b.backend.Now(ctx)
		if err != nil {
			return snapshot, false, err
		}
		return update(snapshot, now)
	})
}

func (b localRunStateBackend) UpdateActiveRun(ctx context.Context, handle RunHandle, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.backend.Update(ctx, handle.key(), func(snapshot Snapshot, ok bool) (Snapshot, bool, error) {
		if !ok || !runMatchesHandle(snapshot.CurrentRunView, handle) || !isActiveRunStatus(snapshot.CurrentRunView.Status) {
			return snapshot, false, ErrRunOwnershipLost
		}
		now, err := b.backend.Now(ctx)
		if err != nil {
			return snapshot, false, err
		}
		return update(snapshot, now)
	})
}

func (b localRunStateBackend) ReleaseRun(ctx context.Context, handle RunHandle, _ StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.UpdateActiveRun(ctx, handle, update)
}

type distributedRunStateBackend struct {
	backend DistributedBackend
}

func (b distributedRunStateBackend) StartRun(ctx context.Context, key Key, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.backend.StartRun(ctx, key, ref, update)
}

func (b distributedRunStateBackend) UpdateActiveRun(ctx context.Context, handle RunHandle, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.backend.UpdateActiveRun(ctx, handle.key(), handle.StreamID, handle.Generation, update)
}

func (b distributedRunStateBackend) ReleaseRun(ctx context.Context, handle RunHandle, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	return b.backend.ReleaseRun(ctx, handle.key(), ref, update)
}
