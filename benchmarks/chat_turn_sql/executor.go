package main

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	runnerSQLC = "sqlc"
	runnerSQL  = "sql"
	runnerHTTP = "http"

	querySourceGeneratedSQLC = "generated_sqlc"
	querySourceSQLTemplate   = "sql_template"
	querySourceHTTPHandler   = "http_handler"

	scanModeSQLCStructScan = "sqlc_struct_scan"
	scanModeRowDrainOnly   = "row_drain_only"
	scanModeHTTPJSON       = "http_json_decode"
)

type queryExecutor interface {
	execQuery(ctx context.Context, queryName string, s SessionSeed, rng *rand.Rand) (int64, error)
	querySource() string
	scanMode() string
}

type executorMetadata struct {
	Runner      string
	QuerySource string
	ScanMode    string
}

func newQueryExecutor(cfg Config, pool *pgxpool.Pool, queries QuerySet) (queryExecutor, error) {
	switch cfg.Workload.Runner {
	case runnerSQLC:
		return newSQLCExecutor(cfg, pool), nil
	case runnerSQL:
		return newSQLTemplateExecutor(cfg, pool, queries)
	case runnerHTTP:
		return newHTTPExecutor(cfg, pool), nil
	default:
		return nil, fmt.Errorf("unknown workload.runner %q", cfg.Workload.Runner)
	}
}
