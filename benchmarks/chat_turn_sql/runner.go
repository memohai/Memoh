package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type runner struct {
	cfg      Config
	executor queryExecutor
	meta     executorMetadata
	catalog  SeedCatalog
	weighted []WeightedQuery
	stats    *statsCollector
	counter  atomic.Int64
	errors   atomic.Int64
}

func newRunner(cfg Config, executor queryExecutor, catalog SeedCatalog) (*runner, error) {
	if executor == nil {
		return nil, errors.New("query executor must not be nil")
	}
	var weighted []WeightedQuery
	var err error
	if cfg.Workload.Scenario == "mixed_saas_read" {
		weighted, err = normalizeWeights(cfg.Workload.QueryWeights)
		if err != nil {
			return nil, err
		}
	}
	return &runner{
		cfg:      cfg,
		executor: executor,
		meta: executorMetadata{
			Runner:      cfg.Workload.Runner,
			QuerySource: executor.querySource(),
			ScanMode:    executor.scanMode(),
		},
		catalog:  catalog,
		weighted: weighted,
		stats:    newStatsCollector(),
	}, nil
}

func (r *runner) run(ctx context.Context) (BenchmarkResult, error) {
	warmup, err := r.cfg.warmupDuration()
	if err != nil {
		return BenchmarkResult{}, err
	}
	duration, err := r.cfg.workloadDuration()
	if err != nil {
		return BenchmarkResult{}, err
	}
	if warmup > 0 {
		if err := r.runPhase(ctx, warmup, true); err != nil {
			return BenchmarkResult{}, err
		}
	}
	r.counter.Store(0)
	startedAt := time.Now().UTC()
	if err := r.runPhase(ctx, duration, false); err != nil {
		return BenchmarkResult{}, err
	}
	result := r.stats.result(r.cfg, r.catalog.Estimate, startedAt, duration, r.meta)
	if r.cfg.Workload.FailOnError && result.TotalErrors() > 0 {
		return result, fmt.Errorf("benchmark completed with %d query errors", result.TotalErrors())
	}
	return result, nil
}

func (r *runner) runPhase(ctx context.Context, duration time.Duration, warmup bool) error {
	phaseCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, r.cfg.Workload.Concurrency)
	for workerID := 0; workerID < r.cfg.Workload.Concurrency; workerID++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			if err := r.worker(phaseCtx, workerID, warmup); err != nil {
				errCh <- err
			}
		}(workerID)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		return err
	}
	return nil
}

func (r *runner) worker(ctx context.Context, workerID int, warmup bool) error {
	if workerID < 0 {
		return fmt.Errorf("worker id must be non-negative: %d", workerID)
	}
	workerSeed := uint64(workerID) + 1
	// #nosec G404 -- deterministic pseudo-random sampling is required for repeatable benchmarks.
	rng := rand.New(rand.NewPCG(r.cfg.Workload.RandomSeed, workerSeed))
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil
			}
			return ctx.Err()
		default:
		}
		n := int(r.counter.Add(1))
		queryName := r.nextQuery(n)
		sample := r.nextSample(rng)
		start := time.Now()
		rows, err := r.executor.execQuery(ctx, queryName, sample, rng)
		if errors.Is(err, context.DeadlineExceeded) {
			if phaseDone(ctx) {
				return nil
			}
		}
		if errors.Is(err, context.Canceled) {
			if phaseDone(ctx) {
				return nil
			}
			return err
		}
		if err != nil && !warmup {
			r.errors.Add(1)
		}
		r.stats.add(queryMeasurement{
			Name:     queryName,
			Latency:  time.Since(start),
			Rows:     rows,
			Err:      err,
			Warmup:   warmup,
			WorkerID: workerID,
		})
	}
}

func (r *runner) nextQuery(n int) string {
	if r.cfg.Workload.Scenario != "mixed_saas_read" {
		return r.cfg.Workload.Scenario
	}
	return pickWeightedQuery(r.weighted, n)
}

func (r *runner) nextSample(rng *rand.Rand) SessionSeed {
	useHot := rng.Float64() < r.cfg.Workload.HotTrafficRatio && len(r.catalog.HotSessions) > 0
	var idx int
	switch {
	case useHot:
		idx = r.catalog.HotSessions[rng.IntN(len(r.catalog.HotSessions))]
	case len(r.catalog.ColdSessions) > 0:
		idx = r.catalog.ColdSessions[rng.IntN(len(r.catalog.ColdSessions))]
	default:
		idx = rng.IntN(len(r.catalog.Sessions))
	}
	return r.catalog.Sessions[idx]
}

func selectedHead(cfg Config, s SessionSeed, rng *rand.Rand) uuid.UUID {
	if len(s.HeadTurnIDs) == 0 || rng.Float64() >= cfg.Workload.SelectedHeadRatio {
		return uuid.Nil
	}
	return s.HeadTurnIDs[rng.IntN(len(s.HeadTurnIDs))]
}

func selectedHeadForBase(s SessionSeed) uuid.UUID {
	if len(s.HeadTurnIDs) > 0 {
		return s.HeadTurnIDs[0]
	}
	return s.DefaultHeadTurnID
}

// The head_resolve scenario needs a non-head turn: production resolves via
// the cheap heads-table lookup first and only recurses for non-heads, so
// benchmarking with a head id would measure the wrong shape.
func variantResolveTarget(queryName string, s SessionSeed) any {
	if s.MidPathTurnID == uuid.Nil {
		return queryArgError(fmt.Sprintf("%s requires mid_path_turn_id in seed catalog; reseed or reload catalog", queryName))
	}
	return s.MidPathTurnID
}

// Turn ids standing in for one latest transcript page. Catalogs written
// before page_turn_ids existed fall back to the default head alone — a
// valid, single-turn page.
func variantPageTurnIDs(s SessionSeed) []uuid.UUID {
	if len(s.PageTurnIDs) > 0 {
		return s.PageTurnIDs
	}
	if s.DefaultHeadTurnID != uuid.Nil {
		return []uuid.UUID{s.DefaultHeadTurnID}
	}
	return nil
}

func variantPathHead(cfg Config, s SessionSeed, rng *rand.Rand) uuid.UUID {
	if headID := selectedHead(cfg, s, rng); headID != uuid.Nil {
		return headID
	}
	return s.DefaultHeadTurnID
}

func selectedCursor(s SessionSeed, rng *rand.Rand) (uuid.UUID, time.Time) {
	if len(s.CursorMessageIDs) == 0 {
		return s.LatestMessageID, time.Now().UTC()
	}
	idx := rng.IntN(len(s.CursorMessageIDs))
	cursorID := s.CursorMessageIDs[idx]
	var cursorTime time.Time
	if idx < len(s.CursorCreatedAts) {
		cursorTime = s.CursorCreatedAts[idx]
	}
	if cursorTime.IsZero() {
		cursorTime = time.Now().UTC()
	}
	return cursorID, cursorTime
}

type queryArgError string

func (e queryArgError) Error() string {
	return string(e)
}

func requireShortID(queryName string, v int32) any {
	if v <= 0 {
		return queryArgError(fmt.Sprintf("%s requires a pending short_id; increase pending_ratio or request density", queryName))
	}
	return v
}

func requireUUID(queryName string, id uuid.UUID) any {
	if id == uuid.Nil {
		return queryArgError(fmt.Sprintf("%s requires a pending request id; increase pending_ratio or request density", queryName))
	}
	return id
}

func requireText(queryName, value string) any {
	if value == "" {
		return queryArgError(fmt.Sprintf("%s requires a prompt external id; increase pending_ratio or request density", queryName))
	}
	return value
}

func phaseDone(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func writeExplainPlans(ctx context.Context, pool *pgxpool.Pool, cfg Config, queries QuerySet, catalog SeedCatalog) error {
	if !cfg.Output.Explain {
		return nil
	}
	if len(catalog.Sessions) == 0 {
		return errors.New("cannot explain without seed catalog")
	}
	// #nosec G703 -- benchmark explain output directory is controlled by the local operator.
	if err := os.MkdirAll(cfg.Output.ExplainDir, 0o750); err != nil {
		return err
	}
	s := catalog.Sessions[0]
	headID := uuid.Nil
	if len(s.HeadTurnIDs) > 1 {
		headID = s.HeadTurnIDs[1]
	}
	// #nosec G404 -- deterministic pseudo-random sampling is required for repeatable explain plans.
	explainRNG := rand.New(rand.NewPCG(cfg.Workload.RandomSeed, 1))
	cursorID, cursorTime := selectedCursor(s, explainRNG)
	argsByQuery := map[string][]any{
		queryLatestPage:               {s.SessionID, nilUUID(headID), cfg.Workload.PageSize},
		queryBeforePage:               {s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, cfg.Workload.PageSize},
		queryAfterPage:                {s.SessionID, nilUUID(headID), nilUUID(cursorID), cursorTime, cfg.Workload.PageSize},
		queryExternalLookup:           {s.SessionID, nilUUID(headID), s.ExternalMessageID},
		queryTurnGraph:                {s.SessionID},
		queryHeadResolve:              {s.SessionID, variantResolveTarget(queryHeadResolve, s)},
		queryTurnSiblings:             {s.SessionID, variantPageTurnIDs(s)},
		queryTurnPath:                 {variantPathHead(cfg, s, explainRNG)},
		queryApprovalPendingList:      {s.BotID, s.SessionID},
		queryApprovalGraphList:        {s.BotID, s.SessionID},
		queryApprovalLatest:           {s.BotID, s.SessionID},
		queryApprovalShortID:          {s.BotID, s.SessionID, requireShortID(queryApprovalShortID, s.ApprovalShortID)},
		queryApprovalVisibleRequest:   {requireUUID(queryApprovalVisibleRequest, s.ApprovalRequestID), s.BotID, s.SessionID},
		queryApprovalBaseHeadRequest:  {requireUUID(queryApprovalBaseHeadRequest, s.ApprovalBaseReqID), s.BotID, s.SessionID, requireUUID(queryApprovalBaseHeadRequest, selectedHeadForBase(s))},
		queryApprovalReplyMessage:     {s.BotID, s.SessionID, requireText(queryApprovalReplyMessage, s.ApprovalPromptID)},
		queryUserInputPendingList:     {s.BotID, s.SessionID},
		queryUserInputGraphList:       {s.BotID, s.SessionID},
		queryUserInputLatest:          {s.BotID, s.SessionID},
		queryUserInputShortID:         {s.BotID, s.SessionID, requireShortID(queryUserInputShortID, s.UserInputShortID)},
		queryUserInputVisibleRequest:  {requireUUID(queryUserInputVisibleRequest, s.UserInputRequestID), s.BotID, s.SessionID},
		queryUserInputBaseHeadRequest: {requireUUID(queryUserInputBaseHeadRequest, s.UserInputBaseReqID), s.BotID, s.SessionID, requireUUID(queryUserInputBaseHeadRequest, selectedHeadForBase(s))},
		queryUserInputReplyMessage:    {s.BotID, s.SessionID, requireText(queryUserInputReplyMessage, s.UserInputPromptID)},
	}
	for _, name := range knownQueries {
		for _, arg := range argsByQuery[name] {
			if argErr, ok := arg.(queryArgError); ok {
				return argErr
			}
		}
		sql := "EXPLAIN (ANALYZE, BUFFERS, WAL, FORMAT JSON) " + queries[name]
		var raw string
		if err := pool.QueryRow(ctx, sql, argsByQuery[name]...).Scan(&raw); err != nil {
			return fmt.Errorf("explain %s: %w", name, err)
		}
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return fmt.Errorf("decode explain %s: %w", name, err)
		}
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err != nil {
			return err
		}
		// #nosec G703 -- benchmark explain output directory is controlled by the local operator.
		if err := os.WriteFile(filepath.Join(cfg.Output.ExplainDir, name+".json"), append(pretty, '\n'), 0o600); err != nil {
			return err
		}
	}
	return nil
}
