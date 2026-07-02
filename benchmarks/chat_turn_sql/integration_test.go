package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrationSeedRunCleanup(t *testing.T) {
	dsn := os.Getenv("MEMOH_BENCH_DSN")
	if dsn == "" {
		t.Skip("set MEMOH_BENCH_DSN to run PostgreSQL integration smoke test")
	}

	cfg := defaultConfig()
	cfg.DB.DSN = dsn
	cfg.DB.MaxOpenConns = 2
	cfg.Seed.Marker = "test-chat-turn-sql"
	cfg.Seed.CleanupBefore = true
	cfg.Seed.Bots = 1
	cfg.Seed.SessionsPerBot = 2
	cfg.Seed.HotSessionRatio = 0.5
	cfg.Seed.TurnsPerSession = 8
	cfg.Seed.BranchFactor = 1
	cfg.Seed.BranchDepth = 2
	cfg.Seed.ActiveHeadsPerSess = 2
	cfg.Seed.MessagesPerTurn = 2
	cfg.Seed.ApprovalEveryNTurns = 1
	cfg.Seed.UserInputEveryNTurns = 1
	cfg.Seed.AssetEveryNMessages = 2
	cfg.Seed.PendingRatio = 1
	cfg.Workload.Duration = "100ms"
	cfg.Workload.Warmup = "0s"
	cfg.Workload.Concurrency = 1
	cfg.Workload.PageSize = 5
	cfg.Output.JSONPath = filepath.Join(t.TempDir(), "results.json")
	cfg.Output.CSVPath = filepath.Join(t.TempDir(), "results.csv")

	ctx := context.Background()
	pool, err := openPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	t.Cleanup(func() {
		_ = cleanupBenchmarkData(context.Background(), pool, cfg.Seed.Marker)
	})

	catalog, err := seedBenchmarkData(ctx, pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Sessions) != cfg.Seed.Bots*cfg.Seed.SessionsPerBot {
		t.Fatalf("sessions = %d", len(catalog.Sessions))
	}
	if catalog.Estimate.Bots == 0 || catalog.Estimate.Turns == 0 || catalog.Estimate.Messages == 0 {
		t.Fatalf("bad actual seed estimate: %#v", catalog.Estimate)
	}
	loadedCatalog, err := loadSeedCatalog(ctx, pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedCatalog.Sessions) != len(catalog.Sessions) {
		t.Fatalf("loaded sessions = %d, want %d", len(loadedCatalog.Sessions), len(catalog.Sessions))
	}
	if loadedCatalog.Estimate != catalog.Estimate {
		t.Fatalf("loaded estimate = %#v, want %#v", loadedCatalog.Estimate, catalog.Estimate)
	}
	var toolCallMessages int64
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM bot_history_messages
WHERE role = 'assistant'
  AND content::text LIKE '%tool-call%'
  AND bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`, cfg.Seed.Marker).Scan(&toolCallMessages); err != nil {
		t.Fatal(err)
	}
	if toolCallMessages == 0 {
		t.Fatal("expected seeded assistant messages with tool-call content")
	}

	queries, err := loadQueries("queries/postgres")
	if err != nil {
		t.Fatal(err)
	}
	for _, runnerName := range []string{runnerSQLC, runnerSQL} {
		cfg.Workload.Runner = runnerName
		for _, scenario := range knownQueries {
			cfg.Workload.Scenario = scenario
			executor, err := newQueryExecutor(cfg, pool, queries)
			if err != nil {
				t.Fatal(err)
			}
			r, err := newRunner(cfg, executor, catalog)
			if err != nil {
				t.Fatal(err)
			}
			result, err := r.run(ctx)
			if err != nil {
				t.Fatalf("%s/%s: %v; result=%#v", runnerName, scenario, err, result)
			}
			if result.Runner != runnerName {
				t.Fatalf("%s/%s: result runner = %q", runnerName, scenario, result.Runner)
			}
			if len(result.Queries) != 1 {
				t.Fatalf("%s/%s: expected one query stat, got %d", runnerName, scenario, len(result.Queries))
			}
			if result.TotalErrors() != 0 {
				t.Fatalf("%s/%s: expected zero errors, got %d", runnerName, scenario, result.TotalErrors())
			}
			if result.Queries[0].TotalCount == 0 {
				t.Fatalf("%s/%s: expected samples", runnerName, scenario)
			}
		}
	}
	cfg.Workload.Runner = runnerHTTP
	for _, scenario := range []string{queryLatestPage, queryBeforePage, queryAfterPage, queryExternalLookup} {
		cfg.Workload.Scenario = scenario
		executor, err := newQueryExecutor(cfg, pool, queries)
		if err != nil {
			t.Fatal(err)
		}
		r, err := newRunner(cfg, executor, catalog)
		if err != nil {
			t.Fatal(err)
		}
		result, err := r.run(ctx)
		if err != nil {
			t.Fatalf("%s/%s: %v; result=%#v", runnerHTTP, scenario, err, result)
		}
		if result.Runner != runnerHTTP {
			t.Fatalf("%s/%s: result runner = %q", runnerHTTP, scenario, result.Runner)
		}
		if result.TotalErrors() != 0 {
			t.Fatalf("%s/%s: expected zero errors, got %d", runnerHTTP, scenario, result.TotalErrors())
		}
		if result.Queries[0].QuerySource != querySourceHTTPHandler {
			t.Fatalf("%s/%s: query source = %q", runnerHTTP, scenario, result.Queries[0].QuerySource)
		}
	}
	cfg.Workload.Scenario = "mixed_saas_read"
	for _, runnerName := range []string{runnerSQLC, runnerSQL} {
		cfg.Workload.Runner = runnerName
		if err := runBenchmark(ctx, cfg, pool, "queries/postgres", catalog); err != nil {
			t.Fatal(err)
		}
	}
	cfg.Workload.Runner = runnerHTTP
	cfg.Workload.QueryWeights = map[string]int{
		queryLatestPage:     4,
		queryBeforePage:     2,
		queryAfterPage:      1,
		queryExternalLookup: 1,
	}
	if err := runBenchmark(ctx, cfg, pool, "queries/postgres", catalog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Output.JSONPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Output.CSVPath); err != nil {
		t.Fatal(err)
	}
	cfg.Output.Explain = true
	cfg.Output.ExplainDir = filepath.Join(t.TempDir(), "explain")
	if err := writeExplainPlans(ctx, pool, cfg, queries, catalog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Output.ExplainDir, queryLatestPage+".json")); err != nil {
		t.Fatal(err)
	}
	if err := cleanupBenchmarkData(ctx, pool, cfg.Seed.Marker); err != nil {
		t.Fatal(err)
	}
	residual, err := benchmarkResidualRows(ctx, pool, cfg.Seed.Marker)
	if err != nil {
		t.Fatal(err)
	}
	if residual.Bots != 0 || residual.Sessions != 0 || residual.Turns != 0 || residual.Messages != 0 || residual.Heads != 0 || residual.Approvals != 0 || residual.UserInputs != 0 || residual.Assets != 0 {
		t.Fatalf("cleanup left residual rows: %#v", residual)
	}
}
