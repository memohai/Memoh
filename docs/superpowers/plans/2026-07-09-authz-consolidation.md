# 鉴权收敛 实施计划（阶段 2）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `team_members.role` 成为权限唯一来源：移除全局 `users.role` 与 `user_role` 枚举，`IsAdmin` 改读请求 scope 的 team 角色，引导流程让首个用户成为 team `owner`。

**Architecture:** 迁移 0106 删 `users.role` + 枚举（down 从 `team_members` 回填）。`accounts.Service` 新增 `IsAdmin(ctx)`（读 `teams.ScopeFromContext(ctx).Role`）和按身份的 `IsTeamAdmin(ctx, userID)`（查 `team_members`）。~18 个 handler 调用点从 `IsAdmin(ctx, channelIdentityID)` 改为 `IsAdmin(ctx)`（`channelIdentityID` 本就是请求调用者 = scope 里的用户）；`builtin.go` 的按身份检查改用 `IsTeamAdmin`。引导流程给首用户授 `owner`。

**Tech Stack:** Go、pgx/v5、sqlc、`internal/accounts`、`internal/teams`、`internal/handlers`、`cmd/agent`。

**依赖：** 阶段 1（`teams.Scope` 已带 `UserID`/`Role`，成员门中间件已注入 scope）。参照 spec §4.2、§5、§10。

## Global Constraints

- 修改 SQL 后运行 `mise run sqlc-generate`；`internal/db/postgres/sqlc/` 禁止手改。
- `0001_init.up.sql` 是完整 canonical schema：删 `users.role`/枚举的同时也要更新它到最终态。
- 增量迁移必须成对 `.up.sql`/`.down.sql`，DDL 用 `IF EXISTS`/`IF NOT EXISTS` 幂等；down 完整逆转 up。
- `IsAdmin(ctx)` 读 scope 角色；仅 `builtin.go` 这类按"特定身份"判定的保留 `IsTeamAdmin(ctx, userID)`。
- 提交信息结尾加：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 每任务后 `go build ./... && go test ./<改动包>/... && golangci-lint run ./<改动包>/...` 通过再提交。

---

### Task 1: 引导流程给首用户授 `owner`（先于删列，避免 EnsureDefault 依赖 users.role）

**Files:**
- Modify: `internal/teams/bootstrap.go:27-37`
- Modify: `cmd/agent/app.go`（`ensureAdminUser`，约 line 1393）
- Test: `internal/teams/bootstrap_test.go`

**Interfaces:**
- Produces: `EnsureDefault` 把所有用户登记为 `member`（不再读 `users.role`）；引导 admin 单独授 `owner`。

- [ ] **Step 1: 改 bootstrap_test.go 断言不再依赖 users.role**

`internal/teams/bootstrap_test.go` 里，把断言"member 回填 SQL 含 `CASE WHEN role = 'admin'`"的用例改为断言回填一律 `member`：

```go
func TestEnsureDefaultEnrollsAllUsersAsMember(t *testing.T) {
	db := &fakeBootstrapDB{}
	if err := EnsureDefault(context.Background(), db); err != nil {
		t.Fatalf("EnsureDefault: %v", err)
	}
	// 第二条 exec 是成员回填；应插入固定 'member'，不再读 users.role。
	if !strings.Contains(db.execs[1].sql, "INSERT INTO team_members") {
		t.Fatal("expected team_members backfill")
	}
	if strings.Contains(db.execs[1].sql, "users.role") || strings.Contains(db.execs[1].sql, "CASE WHEN role") {
		t.Fatal("member backfill must not read users.role")
	}
}
```

> 复用 `bootstrap_test.go` 现有的 `fakeBootstrapDB`/`execs` 结构；若字段名不同按现有为准。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/teams/ -run TestEnsureDefaultEnrollsAllUsersAsMember -count=1`
Expected: FAIL（当前回填含 `CASE WHEN role = 'admin'`）。

- [ ] **Step 3: 改 EnsureDefault 回填为固定 member**

`internal/teams/bootstrap.go`，把成员回填改为：

```go
	_, err := db.Exec(ctx, `
INSERT INTO team_members (team_id, user_id, role)
SELECT $1::uuid, id, 'member'
FROM users
ON CONFLICT (team_id, user_id) DO NOTHING
`, DefaultTeamID)
	return err
```

- [ ] **Step 4: 引导 admin 授 owner**

`cmd/agent/app.go` 的 `ensureAdminUser`，在成功创建 admin user 之后（拿到 `user`），追加把该用户登记为默认 team 的 `owner`（用 `accountStore` 的 Pool 或既有 `postgresStore` exec 能力；若 `ensureAdminUser` 无 DB exec 句柄，改为返回创建的 userID，由调用点在 `EnsureDefault` 后执行一条 upsert）：

```go
	// 首个用户即实例引导者：授予默认 team 的 owner 角色。
	if _, err := accountStore.Pool().Exec(ctx, `
INSERT INTO team_members (team_id, user_id, role)
VALUES ($1::uuid, $2::uuid, 'owner')
ON CONFLICT (team_id, user_id) DO UPDATE SET role = 'owner'
`, teams.DefaultTeamID, user.ID); err != nil {
		return fmt.Errorf("grant bootstrap owner: %w", err)
	}
```

> 若 `accountStore` 没有 `Pool()`，用 `cmd/agent/app.go` 里已有的 `postgresStore.Pool()`（`startServer` 已持有）——把这段放到 `startServer` 的 `OnStart` 钩子里 `ensureAdminUser` 之后、第二次 `EnsureDefault` 之后执行。`teams` 已在 app.go import。

- [ ] **Step 5: 跑测试确认通过 + 构建**

Run: `go test ./internal/teams/ -count=1 && go build ./cmd/agent/`
Expected: PASS + 构建通过。

- [ ] **Step 6: 提交**

```bash
git add internal/teams/bootstrap.go internal/teams/bootstrap_test.go cmd/agent/app.go
git commit -m "feat(team): bootstrap grants owner; member backfill drops users.role read

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `accounts.Service` 新增 `IsAdmin(ctx)` + `IsTeamAdmin(ctx, userID)`

**Files:**
- Modify: `internal/accounts/service.go`（`IsAdmin` 改造 + 新增 `IsTeamAdmin`）
- Modify: `internal/db/store/queries.go`（若 `IsTeamAdmin` 复用 Task-阶段1 的 `GetTeamMembership`，无需新查询）
- Test: `internal/accounts/service_test.go`

**Interfaces:**
- Consumes: `teams.ScopeFromContext`；阶段 1 的 `GetTeamMembership`。
- Produces:
  - `func (s *Service) IsAdmin(ctx context.Context) (bool, error)` — 读 scope.Role ∈ {owner, admin}。
  - `func (s *Service) IsTeamAdmin(ctx context.Context, userID string) (bool, error)` — 查该用户在当前 team 的角色 ∈ {owner, admin}。

- [ ] **Step 1: 写失败测试**

`internal/accounts/service_test.go` 追加：

```go
func TestIsAdminReadsScopeRole(t *testing.T) {
	svc := &Service{} // IsAdmin(ctx) 不需要 store
	for role, want := range map[string]bool{"owner": true, "admin": true, "member": false, "": false} {
		ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teams.DefaultTeamID, UserID: "u1", Role: role})
		got, err := svc.IsAdmin(ctx)
		if err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
		if got != want {
			t.Fatalf("role %q: IsAdmin=%v want %v", role, got, want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/accounts/ -run TestIsAdminReadsScopeRole -count=1`
Expected: 编译失败（`IsAdmin` 签名不匹配）。

- [ ] **Step 3: 改造 IsAdmin + 加 IsTeamAdmin**

`internal/accounts/service.go`，把 `IsAdmin(ctx, userID string)` 替换为：

```go
// IsAdmin reports whether the request's acting user is an owner/admin of the
// resolved team (read from the team scope injected by the membership middleware).
func (s *Service) IsAdmin(ctx context.Context) (bool, error) {
	scope, err := teams.ScopeFromContext(ctx)
	if err != nil {
		return false, nil // no team scope ⇒ not an admin (unauthenticated/background)
	}
	return scope.Role == "owner" || scope.Role == "admin", nil
}

// IsTeamAdmin reports whether the given user is owner/admin of the current team.
// Used where a specific identity (not the request caller) is being checked.
func (s *Service) IsTeamAdmin(ctx context.Context, userID string) (bool, error) {
	if s.membership == nil {
		return false, errors.New("membership reader not configured")
	}
	scope := teams.ScopeOrDefault(ctx)
	role, found, err := s.membership.Membership(ctx, scope.TeamID, userID)
	if err != nil || !found {
		return false, err
	}
	return role == "owner" || role == "admin", nil
}
```

在 `Service` 结构体加一个 `membership teams.MembershipReader` 字段（构造函数注入，复用阶段-1/阶段-5 的 `membershipReader` 适配器）。删除 `isAdminRole`（如无其它使用者）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/accounts/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/accounts/service.go internal/accounts/service_test.go
git commit -m "feat(accounts): IsAdmin reads team scope role; add IsTeamAdmin(ctx,user)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 改所有 IsAdmin 调用点

**Files:**
- Modify: `internal/handlers/{users,acp_runtime,message,local_channel,handler_helpers,session,session_info}.go`（`.IsAdmin(ctx, channelIdentityID)` → `.IsAdmin(ctx)`）
- Modify: `internal/memory/adapters/builtin/builtin.go:417`（按身份 → `IsTeamAdmin(ctx, channelIdentityID)`）
- Modify: 上述文件对应的 fake/mock（测试里若有 `IsAdmin` 打桩，签名跟着改）
- Test: 各 handler 既有测试

**Interfaces:**
- Consumes: Task 2 的 `IsAdmin(ctx)` / `IsTeamAdmin(ctx, userID)`。

- [ ] **Step 1: 逐处替换 handler 调用点**

对以下每一处，把 `xxx.IsAdmin(<ctx表达式>, channelIdentityID)` 改为 `xxx.IsAdmin(<ctx表达式>)`（**保留原 ctx 表达式**，如 `c.Request().Context()` 或 `ctx`）：

- `internal/handlers/users.go`: 206, 244, 279, 325, 367, 402, 453, 708, 798, 1076, 1480, 1494
- `internal/handlers/acp_runtime.go`: 439
- `internal/handlers/message.go`: 694
- `internal/handlers/local_channel.go`: 1786
- `internal/handlers/handler_helpers.go`: 42（`AuthorizeBotAccessWithPermission` 内——注意它把 `isAdmin` 传给 `botService.AuthorizeAccessWithPermission`，逻辑不变，仅去掉入参）
- `internal/handlers/session.go`: 677
- `internal/handlers/session_info.go`: 187

每处的 `channelIdentityID` 变量若在改动后变为未使用，交给 Step 3 处理（多数仍被后续 `Authorize...`/日志使用）。

- [ ] **Step 2: 改 builtin.go 为按身份变体**

`internal/memory/adapters/builtin/builtin.go:417`，`canAccessChat` 检查的是传入的 `channelIdentityID`（不一定是请求 caller），改为：

```go
		isAdmin, err := p.adminChecker.IsTeamAdmin(ctx, channelIdentityID)
```

并把 `adminChecker` 接口（该包内定义）从 `IsAdmin(ctx, id)` 改为 `IsTeamAdmin(ctx, id)`。

- [ ] **Step 3: 修编译错误（未使用变量 / mock 签名）**

Run: `go build ./... 2>&1 | head -40`
逐条修：①某处 `channelIdentityID` 未使用 → 用 `_ = channelIdentityID` 或删除该行赋值（若确无其它使用）；②测试里实现 `IsAdmin` 的 fake/mock → 签名改为 `IsAdmin(ctx) (bool,error)`，`adminChecker` 的 fake 改 `IsTeamAdmin`。

- [ ] **Step 4: 全量构建 + 相关包测试 + lint**

Run:
```bash
go build ./... && go test ./internal/handlers/... ./internal/accounts/... ./internal/memory/... -count=1 && golangci-lint run ./internal/handlers/... ./internal/memory/adapters/builtin/...
```
Expected: 全 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/handlers/ internal/memory/adapters/builtin/builtin.go
git commit -m "refactor(authz): repoint IsAdmin call sites to team-scope role

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 迁移 0106 —— 删 `users.role` + `user_role` 枚举

**Files:**
- Create: `db/postgres/migrations/0106_drop_users_role.up.sql`
- Create: `db/postgres/migrations/0106_drop_users_role.down.sql`
- Modify: `db/postgres/migrations/0001_init.up.sql`（删 `role user_role` 列 + 枚举定义，更新到最终态）
- Modify（自动生成）: `internal/db/postgres/sqlc/*.go`（若有查询 SELECT users.role，会随之变化）
- Test: `internal/db/team_migration_test.go`

**Interfaces:**
- Produces: `users` 无 `role` 列；`user_role` 枚举移除。

- [ ] **Step 1: 确认没有查询仍 SELECT users.role**

Run: `grep -rn "role" db/postgres/queries/*.sql | grep -iE "users|u\.role" | head`
若有 `SELECT ... role ... FROM users`，先改这些查询去掉 role 列（并入本任务），再 regen。

- [ ] **Step 2: 写 up 迁移**

`db/postgres/migrations/0106_drop_users_role.up.sql`：

```sql
-- 0106_drop_users_role
-- Authority is owned by team_members.role. Remove the global users.role flag
-- and its enum; users stays a global identity table.
ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS role;
DROP TYPE IF EXISTS user_role;
```

- [ ] **Step 3: 写 down 迁移（回填 from team_members）**

`db/postgres/migrations/0106_drop_users_role.down.sql`：

```sql
-- 0106_drop_users_role (down)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
    CREATE TYPE user_role AS ENUM ('member', 'admin');
  END IF;
END $$;

ALTER TABLE IF EXISTS users ADD COLUMN IF NOT EXISTS role user_role NOT NULL DEFAULT 'member';

UPDATE users u
SET role = 'admin'
WHERE EXISTS (
  SELECT 1 FROM team_members m
  WHERE m.user_id = u.id AND m.role IN ('owner', 'admin')
);
```

- [ ] **Step 4: 更新 0001 canonical schema**

`db/postgres/migrations/0001_init.up.sql`：删除 `user_role` 枚举的创建块（line 5-6 附近的 `CREATE TYPE user_role ...` DO 块）与 `users` 表里的 `role user_role NOT NULL DEFAULT 'member',` 那一行。

- [ ] **Step 5: 重生成 + 更新迁移契约测试**

Run: `mise run sqlc-generate`
`internal/db/team_migration_test.go`：`0001` 断言集合里**移除**任何对 `user_role`/`users.role` 的期望（若有）；可加一条反向断言：

```go
	if strings.Contains(baseline, "role user_role") {
		t.Fatal("0001 must not define users.role after 0106")
	}
```

- [ ] **Step 6: 迁移 up/down/up 循环验证**

Run（脚本化，同 CI；每文件 BEGIN/COMMIT 包裹）：
```bash
docker exec memoh-dev-postgres psql -U memoh -d postgres -q -c "DROP DATABASE IF EXISTS memoh_p2; CREATE DATABASE memoh_p2;"
cd db/postgres/migrations
for f in $(ls *.up.sql|sort); do (echo BEGIN\;; cat $f; echo COMMIT\;) | docker exec -i memoh-dev-postgres psql -U memoh -d memoh_p2 -q -v ON_ERROR_STOP=1 >/dev/null; done
for f in $(ls *.down.sql|sort -r); do (echo BEGIN\;; cat $f; echo COMMIT\;) | docker exec -i memoh-dev-postgres psql -U memoh -d memoh_p2 -q -v ON_ERROR_STOP=1 >/dev/null; done
for f in $(ls *.up.sql|sort); do (echo BEGIN\;; cat $f; echo COMMIT\;) | docker exec -i memoh-dev-postgres psql -U memoh -d memoh_p2 -q -v ON_ERROR_STOP=1 >/dev/null; done
docker exec memoh-dev-postgres psql -U memoh -d memoh_p2 -tc "SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name='role';"  # expect 0
docker exec memoh-dev-postgres psql -U memoh -d postgres -q -c "DROP DATABASE memoh_p2;"
```
Expected: 三段循环无报错；`users.role` 列数为 0。

- [ ] **Step 7: 全量测试 + 提交**

Run: `go build ./... && go test ./internal/db/... -count=1`
```bash
git add db/postgres/migrations/0106_drop_users_role.up.sql db/postgres/migrations/0106_drop_users_role.down.sql db/postgres/migrations/0001_init.up.sql internal/db/postgres/sqlc/ internal/db/team_migration_test.go db/postgres/queries/
git commit -m "feat(migration): drop users.role and user_role enum (0106)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec 覆盖（§4.2 / §5 / §10）：** IsAdmin → team 角色（Task 2/3）✓；删 users.role + 枚举、down 回填（Task 4）✓；引导授 owner（Task 1）✓；People 页语义靠 team_members（现有 handler 走 IsAdmin 门 + 阶段-1 成员门，本阶段不新增 UI）✓；builtin.go 按身份变体（Task 3）覆盖 §10 "有几处拿 channelIdentityID" 的风险 ✓。

**占位符扫描：** 无 TBD。Task 1 Step 4 给了两种落点（`accountStore.Pool()` 或 `postgresStore.Pool()` in OnStart）取决于现有句柄——是明确的二选一，非占位。Task 3 逐处列了行号 + 精确变换。

**类型一致性：** `IsAdmin(ctx) (bool,error)`、`IsTeamAdmin(ctx, userID) (bool,error)`、`MembershipReader.Membership(ctx,teamID,userID)(string,bool,error)`（复用阶段 1）——跨任务一致。

**顺序：** Task 1（引导授 owner，先于删列，让 EnsureDefault 不再依赖 users.role）→ Task 2/3（代码改读 scope）→ Task 4（删列）。若先删列，EnsureDefault 旧 SQL 会报错，故 Task 1 必须在 Task 4 之前。
