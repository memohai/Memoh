package main

import (
	"errors"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	values := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
	}
	if got := percentile(values, 0.50); got != 2*time.Millisecond {
		t.Fatalf("p50 = %s", got)
	}
	if got := percentile(values, 0.95); got != 4*time.Millisecond {
		t.Fatalf("p95 = %s", got)
	}
}

func TestStatsCollectorSkipsWarmup(t *testing.T) {
	c := newStatsCollector()
	c.add(queryMeasurement{Name: queryLatestPage, Latency: time.Millisecond, Warmup: true})
	c.add(queryMeasurement{Name: queryLatestPage, Latency: 2 * time.Millisecond, Rows: 10})
	result := c.result(defaultConfig(), SeedEstimate{}, time.Now(), time.Second, executorMetadata{
		Runner:      runnerSQLC,
		QuerySource: querySourceGeneratedSQLC,
		ScanMode:    scanModeSQLCStructScan,
	})
	if len(result.Queries) != 1 {
		t.Fatalf("queries = %d", len(result.Queries))
	}
	if result.Runner != runnerSQLC {
		t.Fatalf("runner = %q", result.Runner)
	}
	q := result.Queries[0]
	if q.QuerySource != querySourceGeneratedSQLC {
		t.Fatalf("query source = %q", q.QuerySource)
	}
	if q.ScanMode != scanModeSQLCStructScan {
		t.Fatalf("scan mode = %q", q.ScanMode)
	}
	if q.Count != 1 {
		t.Fatalf("count = %d", q.Count)
	}
	if q.TotalCount != 1 {
		t.Fatalf("total count = %d", q.TotalCount)
	}
	if q.Rows != 10 {
		t.Fatalf("rows = %d", q.Rows)
	}
	if q.P50Millis != 2 {
		t.Fatalf("p50 = %.3f", q.P50Millis)
	}
}

func TestStatsCollectorSeparatesErrorsFromLatency(t *testing.T) {
	c := newStatsCollector()
	c.add(queryMeasurement{Name: queryLatestPage, Latency: time.Microsecond, Err: errors.New("boom")})
	c.add(queryMeasurement{Name: queryLatestPage, Latency: 2 * time.Millisecond, Rows: 10})
	result := c.result(defaultConfig(), SeedEstimate{}, time.Now(), time.Second, executorMetadata{
		Runner:      runnerSQL,
		QuerySource: querySourceSQLTemplate,
		ScanMode:    scanModeRowDrainOnly,
	})
	q := result.Queries[0]
	if q.TotalCount != 2 {
		t.Fatalf("total count = %d", q.TotalCount)
	}
	if q.Count != 1 {
		t.Fatalf("success count = %d", q.Count)
	}
	if q.Errors != 1 {
		t.Fatalf("errors = %d", q.Errors)
	}
	if q.P50Millis != 2 {
		t.Fatalf("p50 includes failed query latency: %.3f", q.P50Millis)
	}
	if result.TotalErrors() != 1 {
		t.Fatalf("total errors = %d", result.TotalErrors())
	}
}

func TestStatsCollectorUsesHTTPHandlerSource(t *testing.T) {
	c := newStatsCollector()
	c.add(queryMeasurement{Name: queryLatestPage, Latency: time.Millisecond, Rows: 3})
	result := c.result(defaultConfig(), SeedEstimate{}, time.Now(), time.Second, executorMetadata{
		Runner:      runnerHTTP,
		QuerySource: querySourceHTTPHandler,
		ScanMode:    scanModeHTTPJSON,
	})
	q := result.Queries[0]
	if q.QuerySource != querySourceHTTPHandler {
		t.Fatalf("query source = %q", q.QuerySource)
	}
	if q.ScanMode != scanModeHTTPJSON {
		t.Fatalf("scan mode = %q", q.ScanMode)
	}
	if len(q.Sources) != 1 || q.Sources[0] != "internal/handlers/message.go ListMessages" {
		t.Fatalf("sources = %#v", q.Sources)
	}
	if len(q.Args) == 0 || q.Args[0] != "bot_id" {
		t.Fatalf("args = %#v", q.Args)
	}
}
