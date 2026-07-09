# RLS 强制生效 + 部署角色 实施计划（阶段 3）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 RLS 成为真正生效的数据库层兜底：所有租户表 `FORCE ROW LEVEL SECURITY`，运行时用非 owner、非 superuser 的 `memoh_app` 角色连接，每请求把 `app.team_id` 注入到该请求所用连接；跨 team 的启动/reconcile 走 `BYPASSRLS` 维护池。

**Architecture:** 迁移 0107 加 FORCE + 建 `memoh_app` 角色 + 授权。运行时主池用 `memoh_app`；一个请求级中间件从池 pin 一个连接、`set_config('app.team_id', scope.TeamID, false)`、存入 context、请求结束 reset 并释放；DBTX 层若 context 有 pin 连接则用它，否则用池。启动/reconcile 的 4 条 all-team 查询走一个 owner/`BYPASSRLS` 维护池。迁移仍用 owner 角色跑。

**Tech Stack:** Go、pgx/v5 + pgxpool、Echo、PostgreSQL RLS、`internal/db`、`internal/db/postgres/store`、docker-compose。

**依赖：** 阶段 1（scope 已注入，`scope.TeamID` 可用）。参照 spec §4.4、§4.5、§6、§8、§10。**这是四个阶段里部署风险最高的一份，合入前 Task 6 的 RLS 集成测试必须绿。**

## Global Constraints

- 修改 SQL 后 `mise run sqlc-generate`；`internal/db/postgres/sqlc/` 禁止手改。
- 迁移用 owner/DDL 角色跑；运行时应用用 `memoh_app`（非 owner、非 superuser）。
- **FORCE RLS 下受限角色若 `app.team_id` 未设，任何带 team_id 的表查询返回零行**——所以每条走受限池的查询前必须已设 `app.team_id`；跨 team 路径必须走 `BYPASSRLS`/owner 维护池。
- 迁移成对 up/down，DDL 幂等（`IF EXISTS`/`IF NOT EXISTS`/`DO $$ ... $$`）。
- 提交信息结尾加：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 每任务后 `go build ./...` + 相关测试 + lint 通过再提交。

---

### Task 1: 迁移 0107 —— FORCE RLS + `memoh_app` 角色 + 授权

**Files:**
- Create: `db/postgres/migrations/0107_rls_enforcement.up.sql`
- Create: `db/postgres/migrations/0107_rls_enforcement.down.sql`
- Modify: `db/postgres/migrations/0001_init.up.sql`（末尾追加 FORCE + 角色/授权到最终态）
- Test: `internal/db/rls_migration_test.go`

**Interfaces:**
- Produces: 所有租户表 `relforcerowsecurity = true`；角色 `memoh_app` 存在且有 CRUD 授权、无 owner/superuser。

- [ ] **Step 1: 写 up 迁移（FORCE + 角色 + 授权）**

`db/postgres/migrations/0107_rls_enforcement.up.sql`（对照 0105 里 `ENABLE ROW LEVEL SECURITY` 的租户表清单，逐表 FORCE；`memoh_app` 密码用占位、由部署方在建角色后 `ALTER ROLE ... PASSWORD` 或用 env）：

```sql
-- 0107_rls_enforcement
-- Make the team_isolation policy actually enforce: FORCE RLS on every tenant
-- table and connect the runtime as a non-owner, non-superuser role.

DO $$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    -- 与 0105 的 ENABLE 清单一致（team_members, channel_identities, ... , tasks）
    'team_members','channel_identities','user_channel_bindings','providers',
    'search_providers','fetch_providers','models','model_variants','memory_providers',
    'bots','bot_acl_rules','bot_channel_admins','user_channel_identity_bindings',
    'channel_link_codes','bot_plugin_installations','mcp_connections','bot_plugin_resources',
    'mcp_oauth_tokens','bot_channel_configs','channel_identity_bind_codes','bot_channel_routes',
    'bot_sessions','bot_session_events','bot_history_messages','bot_session_discuss_cursors',
    'tool_approval_requests','user_input_requests','containers','bot_workspace_resource_limits',
    'snapshots','container_versions','lifecycle_events','schedule','bot_storage_bindings',
    'media_assets','bot_history_message_assets','bot_heartbeat_logs','bot_history_message_compacts',
    'schedule_logs','email_providers','email_oauth_tokens','bot_email_bindings','email_outbox',
    'provider_oauth_tokens','user_provider_oauth_tokens','bot_user_grants','memory_nodes',
    'memory_edges','browser_contexts','tasks'
  ]
  LOOP
    IF to_regclass('public.'||quote_ident(t)) IS NOT NULL THEN
      EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    END IF;
  END LOOP;
END $$;

-- Runtime application role: NOT superuser, NOT table owner, so FORCE RLS applies.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'memoh_app') THEN
    CREATE ROLE memoh_app LOGIN PASSWORD 'memoh_app';
  END IF;
END $$;

GRANT USAGE ON SCHEMA public TO memoh_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO memoh_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO memoh_app;
-- future tables/sequences created by the owner:
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO memoh_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO memoh_app;
```

- [ ] **Step 2: 写 down 迁移**

`db/postgres/migrations/0107_rls_enforcement.down.sql`：

```sql
-- 0107_rls_enforcement (down)
DO $$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'team_members','channel_identities', -- ...（同 up 的清单）...
    'tasks'
  ]
  LOOP
    IF to_regclass('public.'||quote_ident(t)) IS NOT NULL THEN
      EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    END IF;
  END LOOP;
END $$;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM memoh_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM memoh_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM memoh_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE USAGE, SELECT ON SEQUENCES FROM memoh_app;
REVOKE USAGE ON SCHEMA public FROM memoh_app;
DROP ROLE IF EXISTS memoh_app;
```

- [ ] **Step 3: 更新 0001 canonical schema**

在 `0001_init.up.sql` 末尾（`team_isolation` 策略创建之后）追加与 up 相同的 FORCE 循环 + 角色 + 授权块。

- [ ] **Step 4: 写迁移契约/状态测试**

`internal/db/rls_migration_test.go`：断言 0107 与 0001 都含 `FORCE ROW LEVEL SECURITY`、`CREATE ROLE memoh_app`：

```go
func TestRLSMigrationForcesAndCreatesRole(t *testing.T) {
	for _, path := range []string{"postgres/migrations/0107_rls_enforcement.up.sql", "postgres/migrations/0001_init.up.sql"} {
		data, err := embeddeddb.MigrationsFS.ReadFile(path)
		if err != nil { t.Fatalf("read %s: %v", path, err) }
		s := string(data)
		if !strings.Contains(s, "FORCE ROW LEVEL SECURITY") { t.Errorf("%s missing FORCE RLS", path) }
		if !strings.Contains(s, "memoh_app") { t.Errorf("%s missing memoh_app role", path) }
	}
}
```

- [ ] **Step 5: 迁移 up/down/up 循环 + FORCE 状态验证**

Run（脚本同阶段 2 Task 4 Step 6，库名 `memoh_p3`），末尾加：
```bash
docker exec memoh-dev-postgres psql -U memoh -d memoh_p3 -tc "SELECT count(*) FROM pg_class WHERE relforcerowsecurity AND relnamespace='public'::regnamespace;"  # >0
docker exec memoh-dev-postgres psql -U memoh -d memoh_p3 -tc "SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname='memoh_app';"  # f | f
```
Expected: 三段循环无报错；FORCE 表数 >0；`memoh_app` 非 superuser、非 bypassrls。

- [ ] **Step 6: 提交**

```bash
git add db/postgres/migrations/0107_rls_enforcement.up.sql db/postgres/migrations/0107_rls_enforcement.down.sql db/postgres/migrations/0001_init.up.sql internal/db/rls_migration_test.go
git commit -m "feat(migration): FORCE RLS + memoh_app runtime role (0107)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 配置 —— 运行时受限凭据 + 维护凭据

**Files:**
- Modify: `internal/config/config.go`（`PostgresConfig` 增 `AppUser`/`AppPassword`，或新增 `RuntimeUser` 语义）
- Modify: `internal/db/db.go`（新增 `AppDSN(cfg)` 返回受限角色 DSN；`DSN(cfg)` 保持 owner 用于迁移）
- Modify: `conf/app.example.toml`、`conf/app.docker.toml`、`devenv/app.dev.toml`
- Test: `internal/db/dsn_test.go`

**Interfaces:**
- Produces: `db.AppDSN(cfg config.PostgresConfig) string`（受限角色）与既有 `db.DSN`（owner/迁移）。

- [ ] **Step 1: 写失败测试**

`internal/db/dsn_test.go`：

```go
func TestAppDSNUsesRuntimeRole(t *testing.T) {
	cfg := config.PostgresConfig{Host: "h", Port: 5432, User: "memoh", Password: "owner", AppUser: "memoh_app", AppPassword: "apppw", Database: "memoh", SSLMode: "disable"}
	if got := AppDSN(cfg); !strings.Contains(got, "memoh_app") || strings.Contains(got, "user=memoh ") {
		t.Fatalf("AppDSN = %q, want runtime role", got)
	}
	if got := DSN(cfg); !strings.Contains(got, "memoh") { // owner unchanged
		t.Fatalf("DSN = %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败** → `go test ./internal/db/ -run TestAppDSNUsesRuntimeRole -count=1`（编译失败：无 `AppUser`/`AppDSN`）。

- [ ] **Step 3: 加字段与 AppDSN**

`config.go` 的 `PostgresConfig` 增：

```go
	AppUser     string `toml:"app_user"`
	AppPassword string `toml:"app_password" json:"-"`
```

`db.go` 加 `AppDSN`（复制 `DSN` 逻辑，把 user/password 换成 `AppUser`/`AppPassword`；二者为空时回退到 owner，便于 OSS 首启在角色建好前不崩——但记录 warn）。

- [ ] **Step 4: 更新示例配置** —— 在各 toml 的 `[postgres]` 加 `app_user = "memoh_app"` / `app_password = "..."`。

- [ ] **Step 5: 测试 + 提交**

Run: `go test ./internal/db/ ./internal/config/ -count=1`
```bash
git add internal/config/config.go internal/db/db.go internal/db/dsn_test.go conf/ devenv/app.dev.toml
git commit -m "feat(config): runtime restricted DB credential (memoh_app) + AppDSN

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 请求级连接 pin + `app.team_id` 注入中间件

**Files:**
- Create: `internal/db/postgres/store/conn_pin.go`（context 里存/取 pin 的 `*pgxpool.Conn`）
- Create: `internal/server/team_conn_middleware.go`
- Modify: `internal/db/postgres/store/team_dbtx.go`（DBTX 的 Query/Exec/QueryRow：context 有 pin 连接则用它）
- Modify: `internal/server/server.go`（在成员门中间件之后挂）
- Test: `internal/db/postgres/store/conn_pin_test.go`

**Interfaces:**
- Produces:
  - `func WithPinnedConn(ctx, *pgxpool.Conn) context.Context` / `func PinnedConn(ctx) (*pgxpool.Conn, bool)`
  - `func TeamConnMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc`

- [ ] **Step 1: 写失败测试（DBTX 优先用 pin 连接）**

`internal/db/postgres/store/conn_pin_test.go`：

```go
func TestPinnedConnRoundTrip(t *testing.T) {
	ctx := WithPinnedConn(context.Background(), nil) // 仅验证存取；nil 连接下 PinnedConn 返回 ok=true
	if _, ok := PinnedConn(ctx); !ok {
		t.Fatal("expected pinned conn present")
	}
	if _, ok := PinnedConn(context.Background()); ok {
		t.Fatal("expected no pinned conn on bare context")
	}
}
```

- [ ] **Step 2: 实现 conn_pin.go**

```go
package postgresstore

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pinnedConnKey struct{}

func WithPinnedConn(ctx context.Context, c *pgxpool.Conn) context.Context {
	return context.WithValue(ctx, pinnedConnKey{}, c)
}

func PinnedConn(ctx context.Context) (*pgxpool.Conn, bool) {
	v := ctx.Value(pinnedConnKey{})
	if v == nil { return nil, false }
	c, _ := v.(*pgxpool.Conn)
	return c, true
}
```

- [ ] **Step 3: DBTX 用 pin 连接**

`internal/db/postgres/store/team_dbtx.go` 的单条查询路径（`Query`/`Exec`/`QueryRow`）开头：若 `c, ok := PinnedConn(ctx); ok && c != nil` 则用 `c.Query/Exec/QueryRow`（连接上已 `set_config` 过 `app.team_id`），否则走原池逻辑。事务路径（`SET LOCAL`）保持不变。

- [ ] **Step 4: 实现中间件**

`internal/server/team_conn_middleware.go`——从池 acquire、set_config、存 context、结束 reset+release：

```go
package server

import (
	"github.com/labstack/echo/v4"
	"github.com/jackc/pgx/v5/pgxpool"

	pgstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/teams"
)

// TeamConnMiddleware pins a connection with app.team_id set to the resolved
// team, so every query in the request runs under enforced RLS. Must run AFTER
// the membership gate (scope must already be in context).
func TeamConnMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scope, err := teams.ScopeFromContext(c.Request().Context())
			if err != nil {
				return next(c) // unauthenticated/skipped routes: no pin
			}
			conn, err := pool.Acquire(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(500, "db acquire failed")
			}
			defer func() {
				_, _ = conn.Exec(c.Request().Context(), "SELECT set_config('app.team_id', '', false)")
				conn.Release()
			}()
			if _, err := conn.Exec(c.Request().Context(), "SELECT set_config('app.team_id', $1, false)", scope.TeamID); err != nil {
				return echo.NewHTTPError(500, "set team scope failed")
			}
			req := c.Request()
			c.SetRequest(req.WithContext(pgstore.WithPinnedConn(req.Context(), conn)))
			return next(c)
		}
	}
}
```

- [ ] **Step 5: server.go 挂载**（成员门中间件**之后**）：

```go
	e.Use(TeamConnMiddleware(pool)) // pool 为运行时受限 *pgxpool.Pool
```

- [ ] **Step 6: 构建 + 单测 + lint**

Run: `go build ./... && go test ./internal/db/postgres/store/... ./internal/server/... -count=1 && golangci-lint run ./internal/db/postgres/store/... ./internal/server/...`
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/db/postgres/store/conn_pin.go internal/db/postgres/store/conn_pin_test.go internal/db/postgres/store/team_dbtx.go internal/server/team_conn_middleware.go internal/server/server.go
git commit -m "feat(rls): pin per-request conn with app.team_id for enforced RLS

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 维护池 —— 跨 team 启动查询走 BYPASSRLS/owner

**Files:**
- Modify: `cmd/agent/app.go`（构造两个池：主受限池 `AppDSN` + 维护池 `DSN`/owner）
- Modify: 4 条 all-team 查询的调用点（reconcile/bootstrap）改用维护池的 queries
- Test: `internal/db/postgres/store/maintenance_pool_test.go`（可选：断言维护 queries 用 owner DSN 构造）

**Interfaces:**
- Consumes: `db.DSN`（owner，绕过 RLS）。
- Produces: 一个独立的 `dbstore.Queries`（维护），仅供 `ReconcileContainers` / schedule.Bootstrap / heartbeat.Bootstrap / channel 刷新使用。

- [ ] **Step 1: app.go 构造维护池**

在 `app.go` 的 DB 构造处：主池用 `db.AppDSN`（受限），另建维护池 `pgxpool.NewWithConfig(ctx, ParseConfig(db.DSN(cfg.Postgres)))`（owner，天然 BYPASSRLS）。分别包成两个 `dbstore.Queries`：`queries`（受限，请求路径）与 `maintenanceQueries`（owner，启动路径）。

- [ ] **Step 2: 注入维护 queries 到 4 个启动路径**

- `internal/workspace`（`ReconcileContainers` 用的 `m.queries`）
- `internal/schedule`（`Bootstrap` 的 `s.queries`）
- `internal/heartbeat`（`Bootstrap` 的 `s.queries`）
- `internal/channel`（`Manager` 刷新用的 store）

给这些服务的构造函数增加一个"维护 queries"入参（或让 FX provider 传维护实例）。**注意**：这些路径**只**跑那 4 条 all-team 查询 + 下游按每行 team_id 的写入——下游写入若走受限池会被 RLS 拦（app.team_id 未设），所以下游按行处理时需用 `teams.WithScope(ctx, Scope{TeamID: row.TeamID})` + 走受限池的 pin，或统一在维护池上执行。**决策：** 启动路径整体用维护池（owner）执行读+写，简单且正确；待正式流量再回到受限池。

- [ ] **Step 3: 构建 + 测试**

Run: `go build ./... && go test ./internal/workspace/... ./internal/schedule/... ./internal/heartbeat/... ./internal/channel/... -count=1`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add cmd/agent/app.go internal/workspace/ internal/schedule/ internal/heartbeat/ internal/channel/
git commit -m "feat(rls): run cross-team startup queries on a BYPASSRLS maintenance pool

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: docker-compose / 部署角色 provision

**Files:**
- Modify: `devenv/docker-compose.yml`、`docker-compose.yml`（`server` 服务传 `app_user`/`app_password` env；`migrate` 保持 owner）
- Modify: `docker/` entrypoints / `conf/app.docker.toml`
- Modify: `README*.md`（部署说明：迁移用 owner、运行时用 memoh_app）

- [ ] **Step 1: compose 环境变量**

给 `server` 服务加 `MEMOH_POSTGRES_APP_USER=memoh_app` / `MEMOH_POSTGRES_APP_PASSWORD=...`（与 config env 映射一致；env 命名对齐现有 `internal/config` 的 env 前缀）。`memoh_app` 角色由迁移（0107）创建，密码用 `ALTER ROLE memoh_app PASSWORD :'pw'` 或迁移里的占位再由 entrypoint `psql -c "ALTER ROLE ..."` 设置成 env 值。

- [ ] **Step 2: 起 dev 环境验证**

Run: `mise run dev`（或重建 server 容器），确认 server 以 `memoh_app` 连接、正常启动、正常读写（走 pin 中间件）。
Expected: 无 RLS 零行报错；`mise run dev:logs -- server` 无 `app.team_id` 相关错误。

- [ ] **Step 3: 提交**

```bash
git add devenv/docker-compose.yml docker-compose.yml docker/ conf/app.docker.toml README.md README_CN.md README_JA.md
git commit -m "chore(deploy): provision memoh_app runtime role in compose

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: RLS 集成测试（硬性验收闸——证明"形同虚设"被修好）

**Files:**
- Create: `internal/db/rls_enforcement_integration_test.go`

**Interfaces:**
- Consumes: 真 Postgres（dev 库或 CI 库）、`memoh_app` 角色。

- [ ] **Step 1: 写集成测试**

以 `memoh_app` 连接、set `app.team_id` = team A，断言看不到/改不到 team B 的行：

```go
//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRLSBlocksCrossTeam(t *testing.T) {
	ctx := context.Background()
	// 用 memoh_app 凭据连接（从测试配置取）
	conn, err := pgx.Connect(ctx, appDSNForTest(t))
	if err != nil { t.Fatalf("connect memoh_app: %v", err) }
	defer conn.Close(ctx)

	teamA := seedTeam(t, ownerConn(t), "aaaa...") // 用 owner 连接播种两个 team + 各一 bot
	teamB := seedTeam(t, ownerConn(t), "bbbb...")
	_ = teamB

	if _, err := conn.Exec(ctx, "SELECT set_config('app.team_id', $1, false)", teamA); err != nil {
		t.Fatalf("set scope: %v", err)
	}
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM bots").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	// 只应看到 team A 的 bot，看不到 team B 的
	if n != countBotsInTeam(t, ownerConn(t), teamA) {
		t.Fatalf("memoh_app saw %d bots, expected only team A's", n)
	}
	// 尝试改 team B 的行应影响 0 行
	tag, err := conn.Exec(ctx, "UPDATE bots SET name = name WHERE team_id = $1", teamB)
	if err != nil { t.Fatalf("update: %v", err) }
	if tag.RowsAffected() != 0 {
		t.Fatalf("memoh_app updated %d team B rows, want 0", tag.RowsAffected())
	}
}
```

> `appDSNForTest`/`ownerConn`/`seedTeam`/`countBotsInTeam` 按 `internal/db` 现有集成测试样式实现最小 helper。测试用 `//go:build integration` 标签，CI 的迁移/集成 job 里跑。

- [ ] **Step 2: 跑集成测试**

Run: `go test -tags integration ./internal/db/ -run TestRLSBlocksCrossTeam -count=1`
Expected: PASS —— `memoh_app` 只见 team A、改 team B 命中 0 行。**这条绿了，才算 RLS 真正生效。**

- [ ] **Step 3: 提交**

```bash
git add internal/db/rls_enforcement_integration_test.go
git commit -m "test(rls): integration proof memoh_app cannot cross team boundary

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec 覆盖（§4.4 / §4.5 / §6 / §8）：** FORCE + memoh_app 角色 + 授权（Task 1）✓；每连接 app.team_id 注入（Task 3）✓；维护池处理跨 team 启动查询、避开 FORCE 零行陷阱（Task 4）✓；配置双凭据（Task 2）✓；compose provision（Task 5）✓；RLS 集成测试作为验收闸（Task 6）✓；webhook 从资源行取 team（§4.5——现有 webhook 路径已走 all-team 查询解析资源，其后写入走维护池或按 row.team_id 的 scope，Task 4 决策覆盖）。

**占位符扫描：** 无 TBD。角色密码用占位 + 部署方 `ALTER ROLE`/env 设置（明确路径）。Task 4 对"启动路径整体用维护池"给了明确决策。集成测试 helper 明确要求按现有样式补最小实现。

**类型一致性：** `WithPinnedConn`/`PinnedConn`、`AppDSN`/`DSN`、`TeamConnMiddleware(pool)`、`memoh_app`——跨任务一致。0107 与 0001 的 FORCE 表清单必须与 0105 的 ENABLE 清单逐字一致（Task 1 强调）。

**最高风险：** Task 3（连接 pin + DBTX 改造）与 Task 4（维护池边界）。合入前置条件：Task 6 集成测试绿 + `mise run dev` 全链路无 RLS 零行报错。若 Task 3 的 pin 方案在高并发下暴露连接占用问题，回退方案是"写路径用事务 + SET LOCAL、读路径显式 team_id 谓词 + FORCE 仅作兜底"，需另行评估。
