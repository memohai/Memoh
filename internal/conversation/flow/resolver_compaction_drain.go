package flow

import (
	"context"
	"errors"

	"github.com/memohai/memoh/internal/compaction"
)

const maxPreSendCompactionAttempts = 3

func drainPreSendCompaction[T any](
	ctx context.Context,
	initial T,
	initialPressure int,
	threshold int,
	compact func(context.Context, int) (compaction.Result, error),
	rebuild func(context.Context) (T, int, error),
) (T, int, bool, error) {
	snapshot := initial
	pressure := max(initialPressure, 0)
	attempted := false
	for attempt := 0; attempt < maxPreSendCompactionAttempts && compaction.ShouldCompact(pressure, threshold); attempt++ {
		if err := ctx.Err(); err != nil {
			return snapshot, pressure, attempted, err
		}
		attempted = true
		result, compactErr := compact(ctx, pressure)
		if err := ctx.Err(); err != nil {
			return snapshot, pressure, attempted, err
		}
		next, nextPressure, err := rebuild(ctx)
		if err != nil {
			return snapshot, pressure, attempted, err
		}
		nextPressure = max(nextPressure, 0)
		previousPressure := pressure
		snapshot, pressure = next, nextPressure
		if compactErr != nil {
			if errors.Is(compactErr, compaction.ErrCompactionSourceChanged) {
				continue
			}
			return snapshot, pressure, attempted, nil
		}
		if result.Status != compaction.StatusOK || pressure >= previousPressure {
			return snapshot, pressure, attempted, nil
		}
	}
	return snapshot, pressure, attempted, nil
}
