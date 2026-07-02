package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if cfg.DB.DSN == "" {
		return nil, errors.New("db.dsn is required; pass -dsn or set [db].dsn in config")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DB.DSN)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConns = cfg.DB.MaxOpenConns
	statementTimeout := cfg.DB.StatementTimeout
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "memoh-chat-turn-sql-bench"
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SELECT set_config('statement_timeout', $1, false)", statementTimeout)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func cleanupBenchmarkData(ctx context.Context, pool *pgxpool.Pool, marker string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	statements := []string{
		`UPDATE bot_sessions
		 SET default_head_turn_id = NULL
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`UPDATE bot_channel_routes
		 SET active_session_id = NULL
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`UPDATE bot_history_turns
		 SET request_message_id = NULL,
		     final_assistant_message_id = NULL
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_history_message_assets
		 WHERE message_id IN (
		   SELECT id FROM bot_history_messages
		   WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)
		 )`,
		`DELETE FROM tool_approval_requests
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM user_input_requests
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_session_turn_heads
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_history_messages
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_history_turns
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_sessions
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bot_channel_routes
		 WHERE bot_id IN (SELECT id FROM bots WHERE metadata->>'benchmark_marker' = $1)`,
		`DELETE FROM bots WHERE metadata->>'benchmark_marker' = $1`,
		`DELETE FROM channel_identities WHERE metadata->>'benchmark_marker' = $1`,
		`DELETE FROM users WHERE metadata->>'benchmark_marker' = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, marker); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func analyzeBenchmarkTables(ctx context.Context, pool *pgxpool.Pool) error {
	tables := []string{
		"users",
		"channel_identities",
		"bots",
		"bot_channel_routes",
		"bot_sessions",
		"bot_history_turns",
		"bot_session_turn_heads",
		"bot_history_messages",
		"bot_history_message_assets",
		"tool_approval_requests",
		"user_input_requests",
	}
	for _, table := range tables {
		if _, err := pool.Exec(ctx, "ANALYZE "+table); err != nil {
			return err
		}
	}
	return nil
}

type copyBatcher struct {
	ctx       context.Context
	tx        pgx.Tx
	table     string
	columns   []string
	batchSize int
	rows      [][]any
	total     int64
}

func newCopyBatcher(ctx context.Context, tx pgx.Tx, table string, columns []string, batchSize int) *copyBatcher {
	if batchSize <= 0 {
		batchSize = 5000
	}
	return &copyBatcher{
		ctx:       ctx,
		tx:        tx,
		table:     table,
		columns:   columns,
		batchSize: batchSize,
		rows:      make([][]any, 0, batchSize),
	}
}

func (b *copyBatcher) add(values ...any) error {
	b.rows = append(b.rows, values)
	if len(b.rows) >= b.batchSize {
		return b.flush()
	}
	return nil
}

func (b *copyBatcher) flush() error {
	if len(b.rows) == 0 {
		return nil
	}
	count, err := b.tx.CopyFrom(
		b.ctx,
		pgx.Identifier{b.table},
		b.columns,
		pgx.CopyFromRows(b.rows),
	)
	if err != nil {
		return fmt.Errorf("copy %s: %w", b.table, err)
	}
	b.total += count
	b.rows = b.rows[:0]
	return nil
}

func timestampOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
