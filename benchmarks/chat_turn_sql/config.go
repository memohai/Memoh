package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

const benchmarkName = "chat_turn_sql"

type Config struct {
	DB       DBConfig       `toml:"db"`
	Seed     SeedConfig     `toml:"seed"`
	Workload WorkloadConfig `toml:"workload"`
	Output   OutputConfig   `toml:"output"`
}

type DBConfig struct {
	DSN              string `toml:"dsn"`
	MaxOpenConns     int32  `toml:"max_open_conns"`
	StatementTimeout string `toml:"statement_timeout"`
}

type SeedConfig struct {
	Marker               string  `toml:"marker"`
	CleanupBefore        bool    `toml:"cleanup_before"`
	Bots                 int     `toml:"bots"`
	SessionsPerBot       int     `toml:"sessions_per_bot"`
	HotSessionRatio      float64 `toml:"hot_session_ratio"`
	TurnsPerSession      int     `toml:"turns_per_session"`
	BranchFactor         int     `toml:"branch_factor"`
	BranchDepth          int     `toml:"branch_depth"`
	ActiveHeadsPerSess   int     `toml:"active_heads_per_session"`
	MessagesPerTurn      int     `toml:"messages_per_turn"`
	ApprovalEveryNTurns  int     `toml:"approval_every_n_turns"`
	UserInputEveryNTurns int     `toml:"user_input_every_n_turns"`
	AssetEveryNMessages  int     `toml:"asset_every_n_messages"`
	PendingRatio         float64 `toml:"pending_ratio"`
}

type WorkloadConfig struct {
	Runner            string         `toml:"runner"`
	Scenario          string         `toml:"scenario"`
	Duration          string         `toml:"duration"`
	Warmup            string         `toml:"warmup"`
	Concurrency       int            `toml:"concurrency"`
	PageSize          int            `toml:"page_size"`
	RandomSeed        uint64         `toml:"random_seed"`
	FailOnError       bool           `toml:"fail_on_error"`
	SelectedHeadRatio float64        `toml:"selected_head_ratio"`
	HotTrafficRatio   float64        `toml:"hot_traffic_ratio"`
	HTTPFormat        string         `toml:"http_format"`
	HTTPDecodeJSON    bool           `toml:"http_decode_json"`
	QueryWeights      map[string]int `toml:"query_weights"`
}

type OutputConfig struct {
	JSONPath   string `toml:"json_path"`
	CSVPath    string `toml:"csv_path"`
	Explain    bool   `toml:"explain"`
	ExplainDir string `toml:"explain_dir"`
}

type SeedEstimate struct {
	Bots       int64 `json:"bots"`
	Sessions   int64 `json:"sessions"`
	Turns      int64 `json:"turns"`
	Messages   int64 `json:"messages"`
	Heads      int64 `json:"heads"`
	Approvals  int64 `json:"approvals"`
	UserInputs int64 `json:"user_inputs"`
	Assets     int64 `json:"assets"`
}

func defaultConfig() Config {
	return Config{
		DB: DBConfig{
			MaxOpenConns:     16,
			StatementTimeout: "30s",
		},
		Seed: SeedConfig{
			Marker:               "local",
			CleanupBefore:        true,
			Bots:                 4,
			SessionsPerBot:       25,
			HotSessionRatio:      0.1,
			TurnsPerSession:      1000,
			BranchFactor:         2,
			BranchDepth:          64,
			ActiveHeadsPerSess:   2,
			MessagesPerTurn:      2,
			ApprovalEveryNTurns:  50,
			UserInputEveryNTurns: 80,
			AssetEveryNMessages:  0,
			PendingRatio:         0.5,
		},
		Workload: WorkloadConfig{
			Runner:            runnerSQLC,
			Scenario:          "mixed_saas_read",
			Duration:          "30s",
			Warmup:            "5s",
			Concurrency:       8,
			PageSize:          50,
			RandomSeed:        1,
			FailOnError:       true,
			SelectedHeadRatio: 0.25,
			HotTrafficRatio:   0.9,
			HTTPFormat:        "ui",
			HTTPDecodeJSON:    true,
			QueryWeights: map[string]int{
				queryLatestPage:               40,
				queryBeforePage:               18,
				queryAfterPage:                4,
				queryExternalLookup:           3,
				queryTurnGraph:                0,
				queryHeadResolve:              4,
				queryTurnSiblings:             8,
				queryTurnPath:                 4,
				queryApprovalPendingList:      4,
				queryApprovalGraphList:        2,
				queryApprovalLatest:           4,
				queryApprovalShortID:          3,
				queryApprovalVisibleRequest:   1,
				queryApprovalBaseHeadRequest:  1,
				queryApprovalReplyMessage:     1,
				queryUserInputPendingList:     3,
				queryUserInputGraphList:       2,
				queryUserInputLatest:          3,
				queryUserInputShortID:         2,
				queryUserInputVisibleRequest:  1,
				queryUserInputBaseHeadRequest: 1,
				queryUserInputReplyMessage:    1,
			},
		},
		Output: OutputConfig{
			JSONPath:   "benchmarks/chat_turn_sql/out/results.json",
			CSVPath:    "benchmarks/chat_turn_sql/out/results.csv",
			Explain:    false,
			ExplainDir: "benchmarks/chat_turn_sql/out/explain",
		},
	}
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	var meta *toml.MetaData
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return Config{}, err
		}
		decoded, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return Config{}, err
		}
		if undecoded := decoded.Undecoded(); len(undecoded) > 0 {
			return Config{}, fmt.Errorf("unknown config keys: %v", undecoded)
		}
		meta = &decoded
	} else {
		cfg = defaultConfig()
	}
	cfg.applyDefaults(meta)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults(meta *toml.MetaData) {
	d := defaultConfig()
	if !isDefined(meta, "db", "max_open_conns") {
		c.DB.MaxOpenConns = d.DB.MaxOpenConns
	}
	if !isDefined(meta, "db", "statement_timeout") {
		c.DB.StatementTimeout = d.DB.StatementTimeout
	}
	if !isDefined(meta, "seed", "marker") {
		c.Seed.Marker = d.Seed.Marker
	}
	if !isDefined(meta, "seed", "cleanup_before") {
		c.Seed.CleanupBefore = d.Seed.CleanupBefore
	}
	if !isDefined(meta, "seed", "bots") {
		c.Seed.Bots = d.Seed.Bots
	}
	if !isDefined(meta, "seed", "sessions_per_bot") {
		c.Seed.SessionsPerBot = d.Seed.SessionsPerBot
	}
	if !isDefined(meta, "seed", "hot_session_ratio") {
		c.Seed.HotSessionRatio = d.Seed.HotSessionRatio
	}
	if !isDefined(meta, "seed", "turns_per_session") {
		c.Seed.TurnsPerSession = d.Seed.TurnsPerSession
	}
	if !isDefined(meta, "seed", "branch_factor") {
		c.Seed.BranchFactor = d.Seed.BranchFactor
	}
	if !isDefined(meta, "seed", "branch_depth") {
		c.Seed.BranchDepth = d.Seed.BranchDepth
	}
	if !isDefined(meta, "seed", "messages_per_turn") {
		c.Seed.MessagesPerTurn = d.Seed.MessagesPerTurn
	}
	if !isDefined(meta, "seed", "active_heads_per_session") {
		c.Seed.ActiveHeadsPerSess = d.Seed.ActiveHeadsPerSess
	}
	if !isDefined(meta, "seed", "approval_every_n_turns") {
		c.Seed.ApprovalEveryNTurns = d.Seed.ApprovalEveryNTurns
	}
	if !isDefined(meta, "seed", "user_input_every_n_turns") {
		c.Seed.UserInputEveryNTurns = d.Seed.UserInputEveryNTurns
	}
	if !isDefined(meta, "seed", "asset_every_n_messages") {
		c.Seed.AssetEveryNMessages = d.Seed.AssetEveryNMessages
	}
	if !isDefined(meta, "seed", "pending_ratio") {
		c.Seed.PendingRatio = d.Seed.PendingRatio
	}
	if !isDefined(meta, "workload", "scenario") {
		c.Workload.Scenario = d.Workload.Scenario
	}
	if !isDefined(meta, "workload", "runner") {
		c.Workload.Runner = d.Workload.Runner
	}
	if !isDefined(meta, "workload", "duration") {
		c.Workload.Duration = d.Workload.Duration
	}
	if !isDefined(meta, "workload", "warmup") {
		c.Workload.Warmup = d.Workload.Warmup
	}
	if !isDefined(meta, "workload", "concurrency") {
		c.Workload.Concurrency = d.Workload.Concurrency
	}
	if !isDefined(meta, "workload", "page_size") {
		c.Workload.PageSize = d.Workload.PageSize
	}
	if !isDefined(meta, "workload", "random_seed") {
		c.Workload.RandomSeed = d.Workload.RandomSeed
	}
	if !isDefined(meta, "workload", "fail_on_error") {
		c.Workload.FailOnError = d.Workload.FailOnError
	}
	if !isDefined(meta, "workload", "selected_head_ratio") {
		c.Workload.SelectedHeadRatio = d.Workload.SelectedHeadRatio
	}
	if !isDefined(meta, "workload", "hot_traffic_ratio") {
		c.Workload.HotTrafficRatio = d.Workload.HotTrafficRatio
	}
	if !isDefined(meta, "workload", "http_format") {
		c.Workload.HTTPFormat = d.Workload.HTTPFormat
	}
	if !isDefined(meta, "workload", "http_decode_json") {
		c.Workload.HTTPDecodeJSON = d.Workload.HTTPDecodeJSON
	}
	if !isDefined(meta, "workload", "query_weights") {
		c.Workload.QueryWeights = d.Workload.QueryWeights
	}
	if !isDefined(meta, "output", "json_path") {
		c.Output.JSONPath = d.Output.JSONPath
	}
	if !isDefined(meta, "output", "csv_path") {
		c.Output.CSVPath = d.Output.CSVPath
	}
	if !isDefined(meta, "output", "explain") {
		c.Output.Explain = d.Output.Explain
	}
	if !isDefined(meta, "output", "explain_dir") {
		c.Output.ExplainDir = d.Output.ExplainDir
	}
}

func (c Config) validate() error {
	if c.DB.MaxOpenConns <= 0 {
		return errors.New("db.max_open_conns must be > 0")
	}
	if _, err := time.ParseDuration(c.DB.StatementTimeout); err != nil {
		return fmt.Errorf("db.statement_timeout: %w", err)
	}
	if _, err := c.workloadDuration(); err != nil {
		return err
	}
	if _, err := c.warmupDuration(); err != nil {
		return err
	}
	if c.Seed.BranchDepth < 0 {
		return errors.New("seed.branch_depth must be >= 0")
	}
	if c.Seed.BranchFactor < 0 {
		return errors.New("seed.branch_factor must be >= 0")
	}
	if c.Seed.Marker == "" {
		return errors.New("seed.marker must not be empty")
	}
	if c.Seed.Bots <= 0 {
		return errors.New("seed.bots must be > 0")
	}
	if c.Seed.SessionsPerBot <= 0 {
		return errors.New("seed.sessions_per_bot must be > 0")
	}
	if c.Seed.TurnsPerSession <= 0 {
		return errors.New("seed.turns_per_session must be > 0")
	}
	if c.Seed.MessagesPerTurn < 2 {
		return errors.New("seed.messages_per_turn must be >= 2")
	}
	if c.Seed.ActiveHeadsPerSess <= 0 {
		return errors.New("seed.active_heads_per_session must be > 0")
	}
	if c.Seed.ApprovalEveryNTurns < 0 {
		return errors.New("seed.approval_every_n_turns must be >= 0")
	}
	if c.Seed.UserInputEveryNTurns < 0 {
		return errors.New("seed.user_input_every_n_turns must be >= 0")
	}
	if c.Seed.AssetEveryNMessages < 0 {
		return errors.New("seed.asset_every_n_messages must be >= 0")
	}
	if c.Seed.HotSessionRatio < 0 || c.Seed.HotSessionRatio > 1 {
		return errors.New("seed.hot_session_ratio must be between 0 and 1")
	}
	if c.Seed.PendingRatio < 0 || c.Seed.PendingRatio > 1 {
		return errors.New("seed.pending_ratio must be between 0 and 1")
	}
	if c.Workload.Concurrency <= 0 {
		return errors.New("workload.concurrency must be > 0")
	}
	switch c.Workload.Runner {
	case runnerSQLC, runnerSQL, runnerHTTP:
	default:
		return fmt.Errorf("workload.runner must be one of %q, %q, or %q", runnerSQLC, runnerSQL, runnerHTTP)
	}
	if c.Workload.PageSize <= 0 {
		return errors.New("workload.page_size must be > 0")
	}
	if c.Workload.PageSize > math.MaxInt32 {
		return fmt.Errorf("workload.page_size must be <= %d", math.MaxInt32)
	}
	if c.Workload.SelectedHeadRatio < 0 || c.Workload.SelectedHeadRatio > 1 {
		return errors.New("workload.selected_head_ratio must be between 0 and 1")
	}
	if c.Workload.HotTrafficRatio < 0 || c.Workload.HotTrafficRatio > 1 {
		return errors.New("workload.hot_traffic_ratio must be between 0 and 1")
	}
	if c.Workload.Scenario != "mixed_saas_read" && !isKnownQuery(c.Workload.Scenario) {
		return fmt.Errorf("unknown workload.scenario %q", c.Workload.Scenario)
	}
	if c.Workload.Runner == runnerHTTP {
		if err := validateHTTPRunnerScenario(c.Workload); err != nil {
			return err
		}
	}
	if c.Workload.Scenario == "mixed_saas_read" {
		if _, err := normalizeWeights(c.Workload.QueryWeights); err != nil {
			return err
		}
	}
	return nil
}

func validateHTTPRunnerScenario(workload WorkloadConfig) error {
	if workload.Scenario != "mixed_saas_read" {
		if !isHTTPRunnerQuery(workload.Scenario) {
			return fmt.Errorf("http runner supports only %s, %s, %s, and %s scenarios", queryLatestPage, queryBeforePage, queryAfterPage, queryExternalLookup)
		}
		return nil
	}
	for name, weight := range workload.QueryWeights {
		if weight > 0 && !isHTTPRunnerQuery(name) {
			return fmt.Errorf("http runner mixed workload does not support weighted query %q", name)
		}
	}
	return nil
}

func isHTTPRunnerQuery(name string) bool {
	switch name {
	case queryLatestPage, queryBeforePage, queryAfterPage, queryExternalLookup:
		return true
	default:
		return false
	}
}

func isDefined(meta *toml.MetaData, keys ...string) bool {
	return meta != nil && meta.IsDefined(keys...)
}

func (c Config) workloadDuration() (time.Duration, error) {
	d, err := time.ParseDuration(c.Workload.Duration)
	if err != nil {
		return 0, fmt.Errorf("workload.duration: %w", err)
	}
	if d <= 0 {
		return 0, errors.New("workload.duration must be > 0")
	}
	return d, nil
}

func (c Config) warmupDuration() (time.Duration, error) {
	d, err := time.ParseDuration(c.Workload.Warmup)
	if err != nil {
		return 0, fmt.Errorf("workload.warmup: %w", err)
	}
	if d < 0 {
		return 0, errors.New("workload.warmup must be >= 0")
	}
	return d, nil
}

func estimateSeed(cfg Config) SeedEstimate {
	sessions := int64(cfg.Seed.Bots * cfg.Seed.SessionsPerBot)
	branchHeads := branchHeadCount(cfg.Seed)
	baseTurns := sessions * int64(cfg.Seed.TurnsPerSession)
	branchTurns := sessions * int64(branchHeads*cfg.Seed.BranchDepth)
	turns := baseTurns + branchTurns
	messages := turns * int64(cfg.Seed.MessagesPerTurn)
	estimateEvery := func(total int64, every int) int64 {
		if every <= 0 {
			return 0
		}
		return int64(math.Floor(float64(total) / float64(every)))
	}
	return SeedEstimate{
		Bots:       int64(cfg.Seed.Bots),
		Sessions:   sessions,
		Turns:      turns,
		Messages:   messages,
		Heads:      sessions * int64(1+branchHeads),
		Approvals:  estimateEvery(turns, cfg.Seed.ApprovalEveryNTurns),
		UserInputs: estimateEvery(turns, cfg.Seed.UserInputEveryNTurns),
		Assets:     estimateEvery(messages, cfg.Seed.AssetEveryNMessages),
	}
}

func branchHeadCount(seed SeedConfig) int {
	if seed.BranchDepth <= 0 || seed.BranchFactor <= 0 || seed.ActiveHeadsPerSess <= 1 {
		return 0
	}
	return min(seed.BranchFactor, seed.ActiveHeadsPerSess-1)
}
