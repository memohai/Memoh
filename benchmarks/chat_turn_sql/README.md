# Chat Turn SQL Benchmark

This benchmark measures PostgreSQL hot paths for the chat-turn variant model. It does not add migrations or change the app schema. Seeded rows are written to normal app tables and tagged with a benchmark marker in `metadata`.

The runner is a closed-loop benchmark: each worker waits for one request/query to finish before sending the next. The default runner is `sqlc`, which calls generated Postgres sqlc methods and includes pgx scan/decode plus Go allocation cost. The optional `sql` runner is DB-only and row-drain-only: it drains rows from runnable SQL templates and is mainly for SQL microbenchmarks, `EXPLAIN`, and candidate SQL comparison, not Go scan or handler cost. The optional `http` runner uses Echo `httptest` to call the real `MessageHandler` path without TCP network noise.

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
- `workload.runner = "sql"` or `-runner sql`: SQL template runner, drains rows only and supports `EXPLAIN`; use it as a DB-only microbench.
- `workload.runner = "http"` or `-runner http`: handler-level benchmark for `ListMessages` / `LocateMessage`, covering auth context parsing, bot/session authorization, selected-head validation, message service calls, UI conversion, decorators, JSON encoding, and optional JSON response decode.

HTTP runner scope notes:

- The handler is invoked directly through Echo `httptest` contexts, so Echo route matching, the middleware chain, and JWT signature verification are not measured; only claim extraction and everything below it are.
- The media service is not wired, so `fillAssetMimeFromStorage` no-ops and per-asset media mime resolution cost is not measured, even though the seed creates assets.
- HTTP `locate_window` calls `LocateMessage` with a symmetric window. It is intentionally not named `after_page`; low-level SQL `after_page` remains a standalone pagination component and should not be compared horizontally with HTTP locate results.

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
- `workload.query_weights`: scenario mix for `mixed_saas_read`. The default mix uses semantic production-ish paths (`chat_page_ui`, `locate_window`, `approval_resolve`, `user_input_resolve`, `sse_live_filter`) instead of randomly interleaving every component query. Keep low-level scenarios such as `latest_page`, `before_page`, `turn_siblings`, `turn_graph`, and `turn_path` for SQL component microbenchmarks.
- `workload.http_format`: HTTP runner query `format`. Use `"ui"` for chat UI shape or `""` for raw message REST shape. UI pages of chat sessions carry per-turn variant metadata automatically.
- `workload.http_decode_json`: HTTP runner decodes the JSON response and counts `items` when true; disabling it counts response bytes only.
- `output.explain`: writes `EXPLAIN (ANALYZE, BUFFERS, WAL, FORMAT JSON)` per hot query.

Config parsing is strict. Unknown keys and invalid explicit values fail early instead of being silently clamped.

## Scenarios

- `chat_page_ui`: semantic chat UI page path. `runner=http` calls `ListMessages(format=ui)` and covers handler UI conversion/decorators. `runner=sqlc` / `runner=sql` run the SQL component bundle: latest page, UI turn metadata, and tool-call decoration lookups when the seed has matching tool calls.
- `locate_window`: semantic locate path. `runner=http` calls `/messages/locate` with a before/after window. `runner=sqlc` / `runner=sql` run external lookup plus before/after window components. Do not compare this with the low-level `after_page` component scenario.
- `approval_resolve`: representative approval resolution path for non-page UI operations.
- `user_input_resolve`: representative user-input resolution path for non-page UI operations.
- `sse_live_filter`: SSE live-message branch filter. It maps to the `turn_ancestor` existence check and treats no match as a valid false result, not a benchmark error.
- `latest_page`: low-level latest visible message page component.
- `before_page`: low-level cursor pagination toward older messages. Cursors are sampled from the selected head path, including old/mid/recent positions, so branch sessions do not accidentally use sibling-branch cursors.
- `after_page`: low-level cursor pagination toward newer messages. This is a SQL component scenario only; the HTTP runner uses `locate_window` for the handler locate path.
- `external_lookup`: low-level visible-message lookup by external message id. Seeds sample recent/mid/old external ids instead of always using the first message.
- `turn_graph`: full visible turn graph from active heads. This is no longer part of the default mixed production-path workload.
- `head_resolve`: resolve a non-head turn id to the newest active head containing it (variant switching with a pinned mid-path turn). Uses the seeded `mid_path_turn_id`; reseed or reload the catalog after upgrading.
- `turn_siblings`: per-page sibling variant metadata aggregation — the query every UI transcript page of a chat session now runs. Uses the seeded `page_turn_ids`, about 25 turns for the default 50-message / 2-message-per-turn UI page, with branch fork points placed inside that page window.
- `turn_path`: single-head ancestor path ids. This is kept as an old-path comparison scenario; it is no longer part of the default production mix.
- `turn_ancestor`: low-level SSE live-filter ancestor existence check. Samples mostly the common direct append case, plus deep ancestor and cross-branch negative checks, without returning the whole path to Go.
- `approval_tool_calls`: direct UI decoration query for `ListToolApprovalsBySessionToolCalls`.
- `user_input_tool_calls`: direct UI decoration query for `ListUserInputsBySessionToolCalls`.
- `approval_pending_list`, `approval_graph_list`, `approval_latest`, `approval_short_id`, `approval_visible_request`, `approval_base_head_request`, `approval_reply_message`: production tool approval read paths kept as component microbenchmarks.
- `user_input_pending_list`, `user_input_graph_list`, `user_input_latest`, `user_input_short_id`, `user_input_visible_request`, `user_input_base_head_request`, `user_input_reply_message`: production user input read paths kept as component microbenchmarks.
- `runner=http` supports `chat_page_ui`, `latest_page`, `before_page`, `locate_window`, and `external_lookup`.
- `mixed_saas_read`: weighted mix of the configured scenarios.

## Output

The runner prints a compact table and writes JSON/CSV files with:

- `runner`, `query_source`, and `scan_mode`
- count and errors
- rows returned
- p50, p90, p95, p99, max, average latency
- per-query QPS
- production source query name and argument shape

`count`, latency, and QPS include successful queries only. `total_count`, `errors`, `error_rate`, and `last_error` are reported separately. With the default `fail_on_error = true`, a run with measured errors still writes JSON/CSV, then exits with an error. For `sqlc`, `scan_mode = sqlc_struct_scan`; for `sql`, `scan_mode = row_drain_only`; for `http`, `scan_mode = http_json_decode`. For `http`, `rows` means decoded UI/API items (or response bytes when JSON decode is disabled), not SQL rows, so do not compare HTTP row counts with `sqlc`/`sql` row counts.

Benchmark-owned rows can be removed with:

```bash
go run ./benchmarks/chat_turn_sql -mode cleanup
```
