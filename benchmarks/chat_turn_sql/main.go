package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runMain() error {
	var (
		configPath string
		mode       string
		dsn        string
		scenario   string
		runnerName string
		queriesDir string
	)
	flag.StringVar(&configPath, "config", "benchmarks/chat_turn_sql/config.example.toml", "Path to benchmark TOML config")
	flag.StringVar(&mode, "mode", "seed-run", "Mode: estimate, seed, run, seed-run, cleanup, explain")
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN override")
	flag.StringVar(&scenario, "scenario", "", "Scenario override")
	flag.StringVar(&runnerName, "runner", "", `Runner override: "sqlc" for generated production path, "sql" for SQL templates, "http" for Echo handler path`)
	flag.StringVar(&queriesDir, "queries-dir", "benchmarks/chat_turn_sql/queries/postgres", "Directory containing runnable SQL templates")
	flag.Parse()

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if dsn != "" {
		cfg = applyRuntimeOverrides(cfg, dsn, scenario, runnerName, mode)
	} else {
		cfg = applyRuntimeOverrides(cfg, os.Getenv("MEMOH_BENCH_DSN"), scenario, runnerName, mode)
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	switch mode {
	case "estimate":
		return printJSON(estimateSeed(cfg))
	case "seed", "run", "seed-run", "cleanup", "explain":
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}

	ctx := context.Background()
	pool, err := openPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch mode {
	case "cleanup":
		return cleanupBenchmarkData(ctx, pool, cfg.Seed.Marker)
	case "seed":
		catalog, err := seedBenchmarkData(ctx, pool, cfg)
		if err != nil {
			return err
		}
		return printJSON(catalog.Estimate)
	case "run":
		catalog, err := loadSeedCatalog(ctx, pool, cfg)
		if err != nil {
			return err
		}
		return runBenchmark(ctx, cfg, pool, queriesDir, catalog)
	case "seed-run":
		catalog, err := seedBenchmarkData(ctx, pool, cfg)
		if err != nil {
			return err
		}
		return runBenchmark(ctx, cfg, pool, queriesDir, catalog)
	case "explain":
		catalog, err := loadSeedCatalog(ctx, pool, cfg)
		if err != nil {
			return err
		}
		queries, err := loadQueries(queriesDir)
		if err != nil {
			return err
		}
		cfg.Output.Explain = true
		return writeExplainPlans(ctx, pool, cfg, queries, catalog)
	default:
		panic("unreachable")
	}
}

func runBenchmark(ctx context.Context, cfg Config, pool *pgxpool.Pool, queriesDir string, catalog SeedCatalog) error {
	var queries QuerySet
	if cfg.Workload.Runner == runnerSQL || cfg.Output.Explain {
		var err error
		queries, err = loadQueries(queriesDir)
		if err != nil {
			return err
		}
	}
	executor, err := newQueryExecutor(cfg, pool, queries)
	if err != nil {
		return err
	}
	r, err := newRunner(cfg, executor, catalog)
	if err != nil {
		return err
	}
	start := time.Now()
	result, err := r.run(ctx)
	if err := writeJSON(cfg.Output.JSONPath, result); err != nil {
		return err
	}
	if err := writeCSV(cfg.Output.CSVPath, result); err != nil {
		return err
	}
	if err := writeExplainPlans(ctx, pool, cfg, queries, catalog); err != nil {
		return err
	}
	fmt.Printf("completed %s runner=%s in %s\n", cfg.Workload.Scenario, cfg.Workload.Runner, time.Since(start).Round(time.Millisecond))
	for _, q := range result.Queries {
		fmt.Printf("%-32s total=%d ok=%d errors=%d p50=%.3fms p95=%.3fms p99=%.3fms qps=%.2f\n", q.Name, q.TotalCount, q.Count, q.Errors, q.P50Millis, q.P95Millis, q.P99Millis, q.Throughput)
	}
	fmt.Printf("json=%s csv=%s\n", cfg.Output.JSONPath, cfg.Output.CSVPath)
	return err
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func applyRuntimeOverrides(cfg Config, dsn, scenario, runnerName, mode string) Config {
	if dsn != "" {
		cfg.DB.DSN = dsn
	}
	if scenario != "" {
		cfg.Workload.Scenario = scenario
	}
	if runnerName != "" {
		cfg.Workload.Runner = runnerName
	}
	if mode == "explain" {
		cfg.Workload.Runner = runnerSQL
	}
	return cfg
}
