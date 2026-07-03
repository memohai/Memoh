package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

type queryMeasurement struct {
	Name     string
	Latency  time.Duration
	Rows     int64
	Err      error
	Warmup   bool
	WorkerID int
}

type QueryStats struct {
	Name        string   `json:"name"`
	QuerySource string   `json:"query_source"`
	ScanMode    string   `json:"scan_mode"`
	TotalCount  int64    `json:"total_count"`
	Count       int64    `json:"count"`
	Errors      int64    `json:"errors"`
	Rows        int64    `json:"rows"`
	P50Millis   float64  `json:"p50_ms"`
	P90Millis   float64  `json:"p90_ms"`
	P95Millis   float64  `json:"p95_ms"`
	P99Millis   float64  `json:"p99_ms"`
	MaxMillis   float64  `json:"max_ms"`
	AvgMillis   float64  `json:"avg_ms"`
	Throughput  float64  `json:"qps"`
	ErrorRate   float64  `json:"error_rate"`
	LastError   string   `json:"last_error,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Args        []string `json:"args,omitempty"`
}

type BenchmarkResult struct {
	Benchmark string       `json:"benchmark"`
	Marker    string       `json:"marker"`
	Runner    string       `json:"runner"`
	Scenario  string       `json:"scenario"`
	StartedAt time.Time    `json:"started_at"`
	Duration  string       `json:"duration"`
	Warmup    string       `json:"warmup"`
	Config    ResultConfig `json:"config"`
	Seed      SeedEstimate `json:"seed"`
	Queries   []QueryStats `json:"queries"`
}

type ResultConfig struct {
	DB       ResultDBConfig `json:"db"`
	Seed     SeedConfig     `json:"seed"`
	Workload WorkloadConfig `json:"workload"`
	Output   OutputConfig   `json:"output"`
}

type ResultDBConfig struct {
	MaxOpenConns     int32  `json:"max_open_conns"`
	StatementTimeout string `json:"statement_timeout"`
}

type statsCollector struct {
	mu      sync.Mutex
	samples map[string]*querySamples
}

type querySamples struct {
	latencies  []time.Duration
	count      int64
	totalCount int64
	errors     int64
	rows       int64
	total      time.Duration
	max        time.Duration
	lastError  string
}

func newStatsCollector() *statsCollector {
	return &statsCollector{samples: make(map[string]*querySamples)}
}

func (c *statsCollector) add(m queryMeasurement) {
	if m.Warmup {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.samples[m.Name]
	if s == nil {
		s = &querySamples{}
		c.samples[m.Name] = s
	}
	s.totalCount++
	if m.Err != nil {
		s.errors++
		s.lastError = m.Err.Error()
		return
	}
	s.count++
	s.rows += m.Rows
	s.latencies = append(s.latencies, m.Latency)
	s.total += m.Latency
	if m.Latency > s.max {
		s.max = m.Latency
	}
}

func (c *statsCollector) result(cfg Config, seed SeedEstimate, startedAt time.Time, measuredDuration time.Duration, meta executorMetadata) BenchmarkResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.samples))
	for name := range c.samples {
		names = append(names, name)
	}
	sort.Strings(names)
	queries := make([]QueryStats, 0, len(names))
	for _, name := range names {
		s := c.samples[name]
		sort.Slice(s.latencies, func(i, j int) bool { return s.latencies[i] < s.latencies[j] })
		avg := 0.0
		if s.count > 0 {
			avg = durationMillis(s.total) / float64(s.count)
		}
		qps := 0.0
		if measuredDuration > 0 {
			qps = float64(s.count) / measuredDuration.Seconds()
		}
		errorRate := 0.0
		if s.totalCount > 0 {
			errorRate = float64(s.errors) / float64(s.totalCount)
		}
		sources, args := resultSourceAndArgs(name, meta)
		queries = append(queries, QueryStats{
			Name:        name,
			QuerySource: meta.QuerySource,
			ScanMode:    meta.ScanMode,
			TotalCount:  s.totalCount,
			Count:       s.count,
			Errors:      s.errors,
			Rows:        s.rows,
			P50Millis:   durationMillis(percentile(s.latencies, 0.50)),
			P90Millis:   durationMillis(percentile(s.latencies, 0.90)),
			P95Millis:   durationMillis(percentile(s.latencies, 0.95)),
			P99Millis:   durationMillis(percentile(s.latencies, 0.99)),
			MaxMillis:   durationMillis(s.max),
			AvgMillis:   avg,
			Throughput:  qps,
			ErrorRate:   errorRate,
			LastError:   s.lastError,
			Sources:     sources,
			Args:        args,
		})
	}
	return BenchmarkResult{
		Benchmark: benchmarkName,
		Marker:    cfg.Seed.Marker,
		Runner:    meta.Runner,
		Scenario:  cfg.Workload.Scenario,
		StartedAt: startedAt,
		Duration:  cfg.Workload.Duration,
		Warmup:    cfg.Workload.Warmup,
		Config:    resultConfig(cfg),
		Seed:      seed,
		Queries:   queries,
	}
}

func resultSourceAndArgs(name string, meta executorMetadata) ([]string, []string) {
	if meta.QuerySource == querySourceHTTPHandler {
		switch name {
		case queryChatPageUI:
			return []string{"internal/handlers/message.go ListMessages"}, []string{"bot_id", "session_id", "limit", "format=ui", "head_turn_id"}
		case queryLatestPage:
			return []string{"internal/handlers/message.go ListMessages"}, []string{"bot_id", "session_id", "limit", "format", "include_graph", "head_turn_id"}
		case queryBeforePage:
			return []string{"internal/handlers/message.go ListMessages"}, []string{"bot_id", "session_id", "limit", "format", "before", "before_id", "head_turn_id"}
		case queryLocateWindow, queryExternalLookup:
			return []string{"internal/handlers/message.go LocateMessage"}, []string{"bot_id", "session_id", "external_message_id", "before", "after", "head_turn_id"}
		default:
			return []string{"internal/handlers/message.go"}, nil
		}
	}
	if sources, args, ok := compositeResultSourceAndArgs(name); ok {
		return sources, args
	}
	def, ok := queryDefinition(name)
	if !ok {
		return nil, nil
	}
	return []string{def.SourceFile + " " + def.SourceName}, def.Args
}

func compositeResultSourceAndArgs(name string) ([]string, []string, bool) {
	switch name {
	case queryChatPageUI:
		return []string{
			"db/postgres/queries/messages.sql ListMessagesLatestBySession",
			"db/postgres/queries/messages.sql ListSessionTurnSiblings",
			"db/postgres/queries/tool_approval.sql ListToolApprovalsBySessionToolCalls",
			"db/postgres/queries/user_input.sql ListUserInputsBySessionToolCalls",
		}, []string{"session_id", "head_turn_id", "max_count", "turn_ids", "tool_call_ids"}, true
	case queryLocateWindow:
		return []string{
			"db/postgres/queries/messages.sql GetMessageByExternalIDBySession",
			"db/postgres/queries/messages.sql ListMessagesBeforeBySession",
			"db/postgres/queries/messages.sql ListMessagesAfterBySession",
		}, []string{"session_id", "head_turn_id", "external_message_id", "cursor", "max_count"}, true
	case queryApprovalResolve:
		return []string{
			"db/postgres/queries/tool_approval.sql GetLatestPendingToolApprovalBySession",
			"db/postgres/queries/tool_approval.sql GetPendingToolApprovalBySessionShortID",
		}, []string{"bot_id", "session_id", "short_id"}, true
	case queryUserInputResolve:
		return []string{
			"db/postgres/queries/user_input.sql GetLatestPendingUserInputBySession",
			"db/postgres/queries/user_input.sql GetPendingUserInputBySessionShortID",
		}, []string{"bot_id", "session_id", "short_id"}, true
	case querySSELiveFilter:
		return []string{"db/postgres/queries/messages.sql GetSessionTurnAncestorMatch"}, []string{"ancestor_turn_id", "turn_id"}, true
	default:
		return nil, nil, false
	}
}

func resultConfig(cfg Config) ResultConfig {
	return ResultConfig{
		DB: ResultDBConfig{
			MaxOpenConns:     cfg.DB.MaxOpenConns,
			StatementTimeout: cfg.DB.StatementTimeout,
		},
		Seed:     cfg.Seed,
		Workload: cfg.Workload,
		Output:   cfg.Output,
	}
}

func (r BenchmarkResult) TotalErrors() int64 {
	var total int64
	for _, q := range r.Queries {
		total += q.Errors
	}
	return total
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	idx := int(math.Ceil(float64(len(values))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func durationMillis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func writeJSON(path string, result BenchmarkResult) error {
	if path == "" {
		return nil
	}
	// #nosec G703 -- benchmark output path is controlled by the local operator.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// #nosec G304,G703 -- benchmark output path is controlled by the local operator.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	return f.Close()
}

func writeCSV(path string, result BenchmarkResult) error {
	if path == "" {
		return nil
	}
	// #nosec G703 -- benchmark output path is controlled by the local operator.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// #nosec G304,G703 -- benchmark output path is controlled by the local operator.
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write([]string{"query", "query_source", "scan_mode", "total_count", "count", "errors", "error_rate", "rows", "p50_ms", "p90_ms", "p95_ms", "p99_ms", "max_ms", "avg_ms", "qps", "last_error"}); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	for _, q := range result.Queries {
		if err := w.Write([]string{
			q.Name,
			q.QuerySource,
			q.ScanMode,
			strconv.FormatInt(q.TotalCount, 10),
			strconv.FormatInt(q.Count, 10),
			strconv.FormatInt(q.Errors, 10),
			fmt.Sprintf("%.6f", q.ErrorRate),
			strconv.FormatInt(q.Rows, 10),
			fmt.Sprintf("%.3f", q.P50Millis),
			fmt.Sprintf("%.3f", q.P90Millis),
			fmt.Sprintf("%.3f", q.P95Millis),
			fmt.Sprintf("%.3f", q.P99Millis),
			fmt.Sprintf("%.3f", q.MaxMillis),
			fmt.Sprintf("%.3f", q.AvgMillis),
			fmt.Sprintf("%.3f", q.Throughput),
			q.LastError,
		}); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return errors.Join(err, closeErr)
			}
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	return f.Close()
}
