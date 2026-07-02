# Chat Turn SQL Benchmark

This benchmark measures PostgreSQL hot paths for the chat-turn variant model. It does not add migrations or change the app schema. Seeded rows are written to normal app tables and tagged with a benchmark marker in `metadata`.

The runner is a closed-loop benchmark: each worker waits for one request/query to finish before sending the next. The default runner is `sqlc`, which calls generated Postgres sqlc methods and includes pgx scan/decode plus Go allocation cost. The optional `sql` runner drains rows from runnable SQL templates and is mainly for SQL microbenchmarks, `EXPLAIN`, and candidate SQL comparison. The optional `http` runner uses Echo `httptest` to call the real `MessageHandler` path without TCP network noise.

## Run

```bash
go run ./benchmarks/chat_turn_sql \
  -config benchmarks/chat_turn_sql/config.example.toml \
  -mode seed-run
```

Useful modes:

```bash
go run ./benchmarks/chat_turn_sql -mode estimate
go run ./benchmarks/chat_turn_sql -mode seed
go run ./benchmarks/chat_turn_sql -mode run
go run ./benchmarks/chat_turn_sql -mode explain
go run ./benchmarks/chat_turn_sql -mode cleanup
```

You can override the DSN and scenario without editing SQL:

```bash
go run ./benchmarks/chat_turn_sql \
  -dsn "$MEMOH_BENCH_DSN" \
  -scenario latest_page \
  -runner sqlc \
  -mode run
```

Runner modes:

- `workload.runner = "sqlc"` or `-runner sqlc`: generated Postgres sqlc methods, closest to the SaaS production Go hot path.
- `workload.runner = "sql"` or `-runner sql`: SQL template runner, drains rows only and supports `EXPLAIN`.
- `workload.runner = "http"` or `-runner http`: handler-level benchmark for `ListMessages` / `LocateMessage`, covering auth context parsing, bot/session authorization, selected-head validation, message service calls, UI conversion, decorators, JSON encoding, and optional JSON response decode.

HTTP runner scope notes:

- The handler is invoked directly through Echo `httptest` contexts, so Echo route matching, the middleware chain, and JWT signature verification are not measured; only claim extraction and everything below it are.
- The media service is not wired, so `fillAssetMimeFromStorage` no-ops and per-asset media mime resolution cost is not measured, even though the seed creates assets.
- `after_page` has no direct `ListMessages` equivalent and is mapped to `LocateMessage` with a symmetric window, so `after_page` numbers are not comparable across `http` and `sqlc`/`sql` runners.

`-mode explain` always uses SQL templates, even if the config says `runner = "sqlc"`, because generated sqlc methods do not expose a query string that can be prefixed with `EXPLAIN`.

## Configuration

Edit `config.example.toml` or pass another TOML file with `-config`.

Important knobs:

- `seed.bots`, `seed.sessions_per_bot`, `seed.turns_per_session`: total tenant/session/history scale.
- `seed.hot_session_ratio` and `workload.hot_traffic_ratio`: SaaS skew where a small set of sessions receives most traffic.
- `seed.branch_factor`, `seed.branch_depth`, `seed.active_heads_per_session`: variant graph shape.
- `seed.approval_every_n_turns`, `seed.user_input_every_n_turns`, `seed.asset_every_n_messages`: decoration/query side-table density.
- `workload.runner`: `sqlc` for production-path benchmark, `sql` for SQL templates.
- `workload.random_seed`: reproducible sampling.
- `workload.fail_on_error`: returns non-zero if measured query errors occur.
- `workload.query_weights`: query mix for `mixed_saas_read`. The default mix sets `turn_graph = 0` because production no longer loads a full graph; per-page variant metadata reads (`head_resolve`, `turn_siblings`, `turn_path`) replaced it. Keep `turn_graph` as a standalone scenario for old-path or candidate-SQL microbenchmarks.
- `workload.http_format`: HTTP runner query `format`. Use `"ui"` for chat UI shape or `""` for raw message REST shape. UI pages of chat sessions carry per-turn variant metadata automatically.
- `workload.http_decode_json`: HTTP runner decodes the JSON response and counts `items` when true; disabling it counts response bytes only.
- `output.explain`: writes `EXPLAIN (ANALYZE, BUFFERS, WAL, FORMAT JSON)` per hot query.

Config parsing is strict. Unknown keys and invalid explicit values fail early instead of being silently clamped.

## Scenarios

- `latest_page`: latest visible message page.
- `before_page`: cursor pagination toward older messages.
- `after_page`: cursor pagination toward newer messages.
- `external_lookup`: locate a visible message by external message id.
- `runner=http` supports `latest_page`, `before_page`, `after_page`, and `external_lookup`. `latest_page` and `before_page` call `ListMessages`; `after_page` and `external_lookup` call `LocateMessage`.
- `turn_graph`: full visible turn graph from active heads. This is no longer part of the default mixed production-path workload; production replaced graph loading with the per-page variant metadata scenarios below.
- `head_resolve`: resolve a non-head turn id to the newest active head containing it (variant switching with a pinned mid-path turn). Uses the seeded `mid_path_turn_id`; reseed or reload the catalog after upgrading.
- `turn_siblings`: per-page sibling variant metadata aggregation — the query every UI transcript page of a chat session now runs. Uses the seeded `page_turn_ids` (falls back to the default head for old catalogs).
- `turn_path`: single-head ancestor path ids, the SSE live-filter read.
- `approval_pending_list`, `approval_graph_list`, `approval_latest`, `approval_short_id`, `approval_visible_request`, `approval_base_head_request`, `approval_reply_message`: production tool approval read paths.
- `user_input_pending_list`, `user_input_graph_list`, `user_input_latest`, `user_input_short_id`, `user_input_visible_request`, `user_input_base_head_request`, `user_input_reply_message`: production user input read paths.
- `mixed_saas_read`: weighted mix of the configured scenarios.

## Output

The runner prints a compact table and writes JSON/CSV files with:

- `runner`, `query_source`, and `scan_mode`
- count and errors
- rows returned
- p50, p90, p95, p99, max, average latency
- per-query QPS
- production source query name and argument shape

`count`, latency, and QPS include successful queries only. `total_count`, `errors`, `error_rate`, and `last_error` are reported separately. With the default `fail_on_error = true`, a run with measured errors still writes JSON/CSV, then exits with an error. For `sqlc`, `scan_mode = sqlc_struct_scan`; for `sql`, `scan_mode = row_drain_only`; for `http`, `scan_mode = http_json_decode`.

Benchmark-owned rows can be removed with:

```bash
go run ./benchmarks/chat_turn_sql -mode cleanup
```
