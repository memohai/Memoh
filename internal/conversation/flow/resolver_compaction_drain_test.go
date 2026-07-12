package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/compaction"
)

func TestDrainPreSendCompactionRepeatsWhilePressureFalls(t *testing.T) {
	t.Parallel()

	pressures := []int{75, 60}
	attempts := 0
	rebuilds := 0
	snapshot, pressure, attempted, err := drainPreSendCompaction(
		context.Background(),
		"initial",
		90,
		70,
		func(context.Context, int) (compaction.Result, error) {
			attempts++
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
		func(context.Context) (string, int, error) {
			pressure := pressures[rebuilds]
			rebuilds++
			return "rebuilt", pressure, nil
		},
	)
	if err != nil || snapshot != "rebuilt" || pressure != 60 || !attempted || attempts != 2 || rebuilds != 2 {
		t.Fatalf("drain = snapshot:%q pressure:%d attempted:%t attempts:%d rebuilds:%d err:%v", snapshot, pressure, attempted, attempts, rebuilds, err)
	}
}

func TestDrainPreSendCompactionStopsAfterNoopOrNoProgress(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		result compaction.Result
	}{
		{name: "noop", result: compaction.Result{Status: compaction.StatusNoop}},
		{name: "no progress", result: compaction.Result{Status: compaction.StatusOK}},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			rebuilds := 0
			_, pressure, attempted, err := drainPreSendCompaction(
				context.Background(),
				0,
				90,
				70,
				func(context.Context, int) (compaction.Result, error) {
					attempts++
					return test.result, nil
				},
				func(context.Context) (int, int, error) {
					rebuilds++
					return 1, 90, nil
				},
			)
			if err != nil || pressure != 90 || !attempted || attempts != 1 || rebuilds != 1 {
				t.Fatalf("drain = pressure:%d attempted:%t attempts:%d rebuilds:%d err:%v", pressure, attempted, attempts, rebuilds, err)
			}
		})
	}
}

func TestDrainPreSendCompactionBoundsSourceChangeRetries(t *testing.T) {
	t.Parallel()

	attempts := 0
	rebuilds := 0
	_, pressure, attempted, err := drainPreSendCompaction(
		context.Background(),
		0,
		90,
		70,
		func(context.Context, int) (compaction.Result, error) {
			attempts++
			return compaction.Result{}, compaction.ErrCompactionSourceChanged
		},
		func(context.Context) (int, int, error) {
			rebuilds++
			return rebuilds, 90, nil
		},
	)
	if err != nil || pressure != 90 || !attempted || attempts != maxPreSendCompactionAttempts || rebuilds != maxPreSendCompactionAttempts {
		t.Fatalf("drain = pressure:%d attempted:%t attempts:%d rebuilds:%d err:%v", pressure, attempted, attempts, rebuilds, err)
	}
}

func TestDrainPreSendCompactionReloadsOnceAfterFailure(t *testing.T) {
	t.Parallel()

	for _, compactErr := range []error{
		errors.New("compaction failed"),
		context.Canceled,
		context.DeadlineExceeded,
	} {
		rebuilds := 0
		snapshot, pressure, attempted, err := drainPreSendCompaction(
			context.Background(),
			0,
			90,
			70,
			func(context.Context, int) (compaction.Result, error) {
				return compaction.Result{}, compactErr
			},
			func(context.Context) (int, int, error) {
				rebuilds++
				return 1, 80, nil
			},
		)
		if err != nil || snapshot != 1 || pressure != 80 || !attempted || rebuilds != 1 {
			t.Fatalf("compact error %v: drain = snapshot:%d pressure:%d attempted:%t rebuilds:%d err:%v", compactErr, snapshot, pressure, attempted, rebuilds, err)
		}
	}
}

func TestDrainPreSendCompactionPropagatesRebuildAndContextErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("reload failed")
	_, _, attempted, err := drainPreSendCompaction(
		context.Background(),
		0,
		90,
		70,
		func(context.Context, int) (compaction.Result, error) {
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
		func(context.Context) (int, int, error) { return 0, 0, sentinel },
	)
	if !errors.Is(err, sentinel) || !attempted {
		t.Fatalf("rebuild error = %v, want %v", err, sentinel)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	_, _, attempted, err = drainPreSendCompaction(
		canceled,
		0,
		90,
		70,
		func(context.Context, int) (compaction.Result, error) {
			attempts++
			return compaction.Result{}, nil
		},
		func(context.Context) (int, int, error) { return 0, 0, nil },
	)
	if !errors.Is(err, context.Canceled) || attempted || attempts != 0 {
		t.Fatalf("canceled drain = attempted:%t attempts:%d err:%v", attempted, attempts, err)
	}

	interrupted, interrupt := context.WithCancel(context.Background())
	rebuilds := 0
	_, _, attempted, err = drainPreSendCompaction(
		interrupted,
		0,
		90,
		70,
		func(context.Context, int) (compaction.Result, error) {
			interrupt()
			return compaction.Result{Status: compaction.StatusOK}, nil
		},
		func(context.Context) (int, int, error) {
			rebuilds++
			return 1, 60, nil
		},
	)
	if !errors.Is(err, context.Canceled) || !attempted || rebuilds != 0 {
		t.Fatalf("mid-attempt cancel = attempted:%t rebuilds:%d err:%v", attempted, rebuilds, err)
	}
}
