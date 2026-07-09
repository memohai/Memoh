# 团队解析 + 成员门 实施计划（阶段 1）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 每个已认证请求解析出"当前 team"并校验调用者是该 team 的成员，非成员返回 403；`teams.Scope` 带上操作用户与其 team 角色。

**Architecture:** 新增一个可插拔的 `TeamResolver` 接口 + 开源版 `SingleTeamResolver`（解析到唯一默认 team，用一条新的 `team_members` 查询校验成员并载入角色）。一个新中间件排在 `auth.JWTMiddleware` 之后，读 `user_id`、调 resolver、把带 UserID/Role 的 scope 注入请求 context，非成员 403。`teams.Scope` 向后兼容地扩展两个字段。

**Tech Stack:** Go、Echo v4、pgx/v5、sqlc、`internal/teams`、`internal/auth`、`internal/db/store`。

**参照 spec：** `docs/superpowers/specs/2026-07-09-team-tenancy-authz-rls-design.md`（§4.1、§4.5、§7）。

## Global Constraints

- 修改任何 SQL（migrations 或 queries）后运行 `mise run sqlc-generate` 重生成 Go 代码；不得手改 `internal/db/postgres/sqlc/` 下文件。
- `internal/db/postgres/sqlc/` 由 sqlc 自动生成，禁止手改。
- 本阶段**不动** `users.role`、不动 RLS、不动数据库角色（那是阶段 2/3）。
- `teams.Scope` 的扩展必须**向后兼容**：现有只用 `TeamID` 的代码不受影响。
- 未认证路由（登录、健康检查、inbound webhook）**不**经过成员门——与 `auth.JWTMiddleware` 共用同一个 skipper。
- 提交信息结尾加：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 每个任务结束跑 `go build ./... && go test ./<改动包>/... && golangci-lint run ./<改动包>/...` 均通过后再提交。

---

### Task 1: 新增 team 成员查询 `GetTeamMembership`

**Files:**
- Modify: `db/postgres/queries/*.sql`（新建 `db/postgres/queries/team_members.sql`）
- Modify（自动生成）: `internal/db/postgres/sqlc/team_members.sql.go`
- Modify: `internal/db/store/queries.go`（把新方法加入 `Queries` 接口）
- Test: `internal/db/team_membership_query_test.go`

**Interfaces:**
- Produces: 生成方法 `GetTeamMembership(ctx, GetTeamMembershipParams{TeamID, UserID pgtype.UUID}) (GetTeamMembershipRow, error)`，`GetTeamMembershipRow` 含 `Role string`。接口方法 `GetTeamMembership(ctx context.Context, arg dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error)`。

- [ ] **Step 1: 写查询文件**

创建 `db/postgres/queries/team_members.sql`：

```sql
-- name: GetTeamMembership :one
-- Returns the caller's role in a team. Both args are explicit (team + user);
-- this is the membership gate lookup, not an auto-team-scoped query.
SELECT team_id, user_id, role
FROM team_members
WHERE team_id = sqlc.arg(team_id)::uuid
  AND user_id = sqlc.arg(user_id)::uuid;
```

- [ ] **Step 2: 重生成 sqlc**

Run: `mise run sqlc-generate`
Expected: 无报错；`internal/db/postgres/sqlc/team_members.sql.go` 出现 `GetTeamMembership` / `GetTeamMembershipParams` / `GetTeamMembershipRow`。

- [ ] **Step 3: 把方法加入 store 接口**

在 `internal/db/store/queries.go` 的 `Queries` 接口里，按字母序附近加入（紧邻其他 `Get...` 方法）：

```go
	GetTeamMembership(ctx context.Context, arg dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error)
```

- [ ] **Step 4: 写失败测试（真库集成）**

创建 `internal/db/team_membership_query_test.go`（参照仓库现有集成测试如何拿到 `*pgxpool.Pool` / test DB；若包内已有 `newTestQueries(t)` 之类 helper，复用它）：

```go
package db_test

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/teams"
)

// TestGetTeamMembershipReturnsRoleForMember 验证默认 team 的成员能查到角色。
func TestGetTeamMembershipReturnsRoleForMember(t *testing.T) {
	q, cleanup := newIntegrationQueries(t) // 复用包内既有 helper；若无则按现有集成测试样式新建
	defer cleanup()
	ctx := context.Background()

	teamID := mustUUID(t, teams.DefaultTeamID)
	userID := seedUser(t, q, "member-a") // 复用现有 seed helper 建 user + team_members(default, user, 'member')

	row, err := q.GetTeamMembership(ctx, membershipParams(teamID, userID))
	if err != nil {
		t.Fatalf("GetTeamMembership: %v", err)
	}
	if row.Role != "member" {
		t.Fatalf("role = %q, want member", row.Role)
	}
}
```

> 说明：`newIntegrationQueries`、`mustUUID`、`seedUser`、`membershipParams` 用包内既有测试辅助；若某个不存在，按 `internal/db` 下现有集成测试的建立方式补最小 helper（不要引入新框架）。

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/db/ -run TestGetTeamMembership -count=1`
Expected: PASS（查询与接口就绪）。

- [ ] **Step 6: 提交**

```bash
git add db/postgres/queries/team_members.sql internal/db/postgres/sqlc/ internal/db/store/queries.go internal/db/team_membership_query_test.go
git commit -m "feat(team): add GetTeamMembership query for membership gate

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 扩展 `teams.Scope`，带上 UserID 与 Role

**Files:**
- Modify: `internal/teams/context.go:19`（`Scope` 结构体）
- Test: `internal/teams/context_test.go`

**Interfaces:**
- Consumes: 无。
- Produces: `teams.Scope{ TeamID, UserID, Role string }`；`IsZero()` 语义不变（仍只看 `TeamID`）。

- [ ] **Step 1: 写失败测试**

在 `internal/teams/context_test.go` 末尾追加：

```go
func TestScopeCarriesUserAndRole(t *testing.T) {
	in := Scope{TeamID: DefaultTeamID, UserID: "u1", Role: "owner"}
	got, err := ScopeFromContext(WithScope(context.Background(), in))
	if err != nil {
		t.Fatalf("ScopeFromContext: %v", err)
	}
	if got.UserID != "u1" || got.Role != "owner" {
		t.Fatalf("scope = %+v, want UserID=u1 Role=owner", got)
	}
}

func TestScopeIsZeroStillOnlyChecksTeamID(t *testing.T) {
	// 只有 UserID/Role 没有 TeamID 时仍视为 zero（向后兼容）。
	if !(Scope{UserID: "u1", Role: "owner"}).IsZero() {
		t.Fatal("scope without TeamID must be zero")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/teams/ -run 'TestScopeCarriesUserAndRole|TestScopeIsZeroStillOnlyChecksTeamID' -count=1`
Expected: 编译失败（`Scope` 无 `UserID`/`Role` 字段）。

- [ ] **Step 3: 扩展结构体**

`internal/teams/context.go`，把：

```go
type Scope struct {
	TeamID string
}
```

改为：

```go
type Scope struct {
	TeamID string
	// UserID is the acting principal; empty for system/background contexts.
	UserID string
	// Role is the caller's team_members.role in TeamID: owner|admin|member.
	Role string
}
```

`IsZero()` 不动（继续只看 `TeamID`）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/teams/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/teams/context.go internal/teams/context_test.go
git commit -m "feat(team): carry acting user and role in teams.Scope

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: `TeamResolver` 接口 + 开源版 `SingleTeamResolver`

**Files:**
- Create: `internal/teams/resolver.go`
- Test: `internal/teams/resolver_test.go`

**Interfaces:**
- Consumes: Task 1 的 `GetTeamMembership`（经一个窄接口，避免 teams 包依赖整个 dbstore）；Task 2 的 `Scope`。
- Produces:
  - `type TeamResolver interface { Resolve(ctx context.Context, userID string) (Scope, error) }`
  - `var ErrNotTeamMember = errors.New("user is not a member of the team")`
  - `func NewSingleTeamResolver(m MembershipReader) *SingleTeamResolver`
  - `type MembershipReader interface { Membership(ctx context.Context, teamID, userID string) (role string, found bool, err error) }`

- [ ] **Step 1: 写失败测试**

创建 `internal/teams/resolver_test.go`：

```go
package teams

import (
	"context"
	"errors"
	"testing"
)

type fakeMembership struct {
	role  string
	found bool
	err   error
	gotTeam, gotUser string
}

func (f *fakeMembership) Membership(_ context.Context, teamID, userID string) (string, bool, error) {
	f.gotTeam, f.gotUser = teamID, userID
	return f.role, f.found, f.err
}

func TestSingleTeamResolverMemberGetsScopedRole(t *testing.T) {
	m := &fakeMembership{role: "admin", found: true}
	got, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TeamID != DefaultTeamID || got.UserID != "u1" || got.Role != "admin" {
		t.Fatalf("scope = %+v", got)
	}
	if m.gotTeam != DefaultTeamID || m.gotUser != "u1" {
		t.Fatalf("membership queried with team=%q user=%q", m.gotTeam, m.gotUser)
	}
}

func TestSingleTeamResolverNonMemberRejected(t *testing.T) {
	m := &fakeMembership{found: false}
	_, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if !errors.Is(err, ErrNotTeamMember) {
		t.Fatalf("err = %v, want ErrNotTeamMember", err)
	}
}

func TestSingleTeamResolverPropagatesLookupError(t *testing.T) {
	sentinel := errors.New("db down")
	m := &fakeMembership{err: sentinel}
	_, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/teams/ -run TestSingleTeamResolver -count=1`
Expected: 编译失败（`TeamResolver`/`SingleTeamResolver`/`ErrNotTeamMember`/`MembershipReader` 未定义）。

- [ ] **Step 3: 实现 resolver**

创建 `internal/teams/resolver.go`：

```go
package teams

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotTeamMember is returned when the caller is not a member of the resolved team.
var ErrNotTeamMember = errors.New("user is not a member of the team")

// MembershipReader looks up a user's role within a team. found=false means the
// user has no membership row (not an error).
type MembershipReader interface {
	Membership(ctx context.Context, teamID, userID string) (role string, found bool, err error)
}

// TeamResolver resolves and authorizes the acting team for a request.
// SaaS supplies its own implementation; open-source uses SingleTeamResolver.
type TeamResolver interface {
	Resolve(ctx context.Context, userID string) (Scope, error)
}

// SingleTeamResolver resolves every caller to the single default team and
// requires the caller to be a member of it.
type SingleTeamResolver struct {
	members MembershipReader
}

func NewSingleTeamResolver(members MembershipReader) *SingleTeamResolver {
	return &SingleTeamResolver{members: members}
}

func (r *SingleTeamResolver) Resolve(ctx context.Context, userID string) (Scope, error) {
	role, found, err := r.members.Membership(ctx, DefaultTeamID, userID)
	if err != nil {
		return Scope{}, fmt.Errorf("resolve team membership: %w", err)
	}
	if !found {
		return Scope{}, ErrNotTeamMember
	}
	return Scope{TeamID: DefaultTeamID, UserID: userID, Role: role}, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/teams/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/teams/resolver.go internal/teams/resolver_test.go
git commit -m "feat(team): add TeamResolver + open-source SingleTeamResolver

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 成员门中间件

**Files:**
- Create: `internal/teams/middleware.go`
- Test: `internal/teams/middleware_test.go`

**Interfaces:**
- Consumes: Task 3 的 `TeamResolver` / `ErrNotTeamMember`；`auth.UserIDFromContext`（`internal/auth`）。
- Produces: `func ResolveTeamMiddleware(resolver TeamResolver, userID func(echo.Context) (string, error), skipper func(echo.Context) bool) echo.MiddlewareFunc`。行为：skipper 命中则透传；否则取 user_id → `resolver.Resolve` → 成功注入 scope，`ErrNotTeamMember` → 403，其它错误 → 500。

- [ ] **Step 1: 写失败测试**

创建 `internal/teams/middleware_test.go`：

```go
package teams

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

type stubResolver struct {
	scope Scope
	err   error
}

func (s stubResolver) Resolve(context.Context, string) (Scope, error) { return s.scope, s.err }

func newCtx(t *testing.T) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bots", nil)
	return e.NewContext(req, httptest.NewRecorder()), httptest.NewRecorder()
}

func fixedUserID(id string, err error) func(echo.Context) (string, error) {
	return func(echo.Context) (string, error) { return id, err }
}

func TestMiddlewareInjectsScopeForMember(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{scope: Scope{TeamID: DefaultTeamID, UserID: "u1", Role: "owner"}}, fixedUserID("u1", nil), nil)
	var got Scope
	err := mw(func(c echo.Context) error {
		s, e := ScopeFromContext(c.Request().Context())
		got = s
		return e
	})(c)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if got.UserID != "u1" || got.Role != "owner" {
		t.Fatalf("scope = %+v", got)
	}
}

func TestMiddlewareNonMemberReturns403(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{err: ErrNotTeamMember}, fixedUserID("u1", nil), nil)
	err := mw(func(echo.Context) error { return nil })(c)
	he := new(echo.HTTPError)
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("err = %v, want 403", err)
	}
}

func TestMiddlewareSkipperBypasses(t *testing.T) {
	c, _ := newCtx(t)
	called := false
	mw := ResolveTeamMiddleware(stubResolver{err: ErrNotTeamMember}, fixedUserID("", errors.New("no user")), func(echo.Context) bool { return true })
	err := mw(func(echo.Context) error { called = true; return nil })(c)
	if err != nil || !called {
		t.Fatalf("skipper should pass through untouched (err=%v called=%v)", err, called)
	}
}

func TestMiddlewareResolverErrorReturns500(t *testing.T) {
	c, _ := newCtx(t)
	mw := ResolveTeamMiddleware(stubResolver{err: errors.New("db down")}, fixedUserID("u1", nil), nil)
	err := mw(func(echo.Context) error { return nil })(c)
	he := new(echo.HTTPError)
	if !errors.As(err, &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("err = %v, want 500", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/teams/ -run TestMiddleware -count=1`
Expected: 编译失败（`ResolveTeamMiddleware` 未定义）。

- [ ] **Step 3: 实现中间件**

创建 `internal/teams/middleware.go`：

```go
package teams

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ResolveTeamMiddleware resolves the acting team for authenticated requests and
// rejects non-members. It must be registered AFTER the auth middleware so
// userID(c) can read the authenticated principal. skipper (may be nil) marks
// unauthenticated routes that carry no user and must pass through untouched.
func ResolveTeamMiddleware(
	resolver TeamResolver,
	userID func(echo.Context) (string, error),
	skipper func(echo.Context) bool,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper != nil && skipper(c) {
				return next(c)
			}
			uid, err := userID(c)
			if err != nil || uid == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "unauthenticated")
			}
			scope, err := resolver.Resolve(c.Request().Context(), uid)
			if err != nil {
				if errors.Is(err, ErrNotTeamMember) {
					return echo.NewHTTPError(http.StatusForbidden, "not a member of this team")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "team resolution failed")
			}
			req := c.Request()
			c.SetRequest(req.WithContext(WithScope(req.Context(), scope)))
			return next(c)
		}
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/teams/ -count=1`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/teams/middleware.go internal/teams/middleware_test.go
git commit -m "feat(team): membership-gate middleware (403 for non-members)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 接线 —— MembershipReader 适配器 + server 挂载

**Files:**
- Create: `internal/server/team_membership.go`（把 `dbstore.Queries` 适配成 `teams.MembershipReader`）
- Modify: `internal/server/server.go`（构造 resolver，在 `auth.JWTMiddleware` 之后挂 `ResolveTeamMiddleware`，复用同一 skipper）
- Test: `internal/server/team_membership_test.go`

**Interfaces:**
- Consumes: Task 1 `GetTeamMembership`；Task 3 `NewSingleTeamResolver`；Task 4 `ResolveTeamMiddleware`；`auth.UserIDFromContext`。
- Produces: `server` 内部接线，无对外新签名。

- [ ] **Step 1: 写失败测试（适配器把 not-found 映射成 found=false）**

创建 `internal/server/team_membership_test.go`：

```go
package server

import (
	"context"
	"errors"
	"testing"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/db"
)

type fakeMembershipQueries struct {
	row dbsqlc.GetTeamMembershipRow
	err error
}

func (f fakeMembershipQueries) GetTeamMembership(context.Context, dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error) {
	return f.row, f.err
}

func TestMembershipReaderFoundReturnsRole(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{row: dbsqlc.GetTeamMembershipRow{Role: "admin"}}}
	role, found, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "u1")
	if err != nil || !found || role != "admin" {
		t.Fatalf("role=%q found=%v err=%v", role, found, err)
	}
}

func TestMembershipReaderNotFoundIsNotError(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{err: db.ErrNotFound}}
	_, found, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "u1")
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want found=false err=nil", found, err)
	}
}
```

> 说明：`membershipReader` 只依赖一个窄接口（含 `GetTeamMembership`），便于用 fake 测试。若 `db.ErrNotFound` 不是 sqlc 返回的 not-found 语义，改用 `pgx.ErrNoRows`（按 Task 1 生成方法 `:one` 的实际 not-found 错误——`internal/db/postgres/sqlc` 的 `:one` 返回 `pgx.ErrNoRows`）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/server/ -run TestMembershipReader -count=1`
Expected: 编译失败（`membershipReader` 未定义）。

- [ ] **Step 3: 实现适配器**

创建 `internal/server/team_membership.go`：

```go
package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// membershipQuery is the narrow slice of dbstore.Queries the reader needs.
type membershipQuery interface {
	GetTeamMembership(ctx context.Context, arg dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error)
}

// membershipReader adapts the generated query to teams.MembershipReader,
// mapping a no-rows result to found=false rather than an error.
type membershipReader struct {
	q membershipQuery
}

func (r membershipReader) Membership(ctx context.Context, teamID, userID string) (string, bool, error) {
	tid, err := dbpkg.ParseUUID(teamID)
	if err != nil {
		return "", false, err
	}
	uid, err := dbpkg.ParseUUID(userID)
	if err != nil {
		return "", false, err
	}
	row, err := r.q.GetTeamMembership(ctx, dbsqlc.GetTeamMembershipParams{TeamID: tid, UserID: uid})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, dbpkg.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return row.Role, true, nil
}
```

> 注：`dbpkg.ErrNotFound` 若不存在则删掉该分支，只保留 `pgx.ErrNoRows`。以 Task 1 生成代码里 `:one` 的实际 not-found 为准。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/server/ -run TestMembershipReader -count=1`
Expected: PASS。

- [ ] **Step 5: 在 server.go 挂中间件**

`internal/server/server.go`：`auth.JWTMiddleware`（约 line 64）**之后**追加（复用同一个 `jwtSkipper`——即传给 `auth.JWTMiddleware` 的那个 skipper 函数；把它提取成命名变量以便共用）：

```go
	// After auth: resolve the acting team and reject non-members. Shares the
	// auth skipper so unauthenticated routes (login, health, webhooks) pass through.
	resolver := teams.NewSingleTeamResolver(membershipReader{q: queries})
	e.Use(teams.ResolveTeamMiddleware(resolver, auth.UserIDFromContext, jwtSkipper))
```

保留 `teams.DefaultMiddleware()`（line 37）用于**未认证路由**（它们被上面的 skipper 跳过，仍需一个默认 scope 兜底，避免 `ScopeOrDefault` 落到别处）。`queries`（`dbstore.Queries`）已是 `NewServer` 的既有依赖；若签名里没有，从既有注入点取（`server.go`/`app.go` 已持有 `queries`）。

- [ ] **Step 6: 全量构建 + 相关包测试 + lint**

Run:
```bash
go build ./... && go test ./internal/teams/... ./internal/server/... ./internal/db/... -count=1 && golangci-lint run ./internal/teams/... ./internal/server/...
```
Expected: 全 PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/server/team_membership.go internal/server/team_membership_test.go internal/server/server.go
git commit -m "feat(team): wire membership gate into the HTTP server

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 端到端冒烟 —— 非成员被拒、成员放行

**Files:**
- Test: `internal/server/membership_gate_e2e_test.go`

**Interfaces:**
- Consumes: 全链路（middleware + resolver + adapter + 真库或 fake queries）。

- [ ] **Step 1: 写端到端测试**

创建 `internal/server/membership_gate_e2e_test.go`（用 fake `membershipQuery` 驱动全链路，不依赖真库；构造一个挂了 `ResolveTeamMiddleware` 的最小 echo 实例）：

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/memohai/memoh/internal/teams"
)

func mount(q membershipQuery, uid string) *echo.Echo {
	e := echo.New()
	resolver := teams.NewSingleTeamResolver(membershipReader{q: q})
	e.Use(teams.ResolveTeamMiddleware(resolver, func(echo.Context) (string, error) { return uid, nil }, nil))
	e.GET("/whoami", func(c echo.Context) error {
		s, _ := teams.ScopeFromContext(c.Request().Context())
		return c.String(http.StatusOK, s.Role)
	})
	return e
}

func TestE2EMemberPasses(t *testing.T) {
	e := mount(fakeMembershipQueries{row: dbsqlc.GetTeamMembershipRow{Role: "owner"}}, "u1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "owner" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestE2ENonMemberForbidden(t *testing.T) {
	e := mount(fakeMembershipQueries{err: pgx.ErrNoRows}, "u1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want 403", rec.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认通过**

Run: `go test ./internal/server/ -run TestE2E -count=1`
Expected: PASS（成员 200+角色；非成员 403）。

- [ ] **Step 3: 提交**

```bash
git add internal/server/membership_gate_e2e_test.go
git commit -m "test(team): e2e membership gate — member passes, non-member 403

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec 覆盖（§4.1 / §4.5 / §7）：**
- §4.1 pluggable resolver + SingleTeamResolver → Task 3 ✓；Scope 带 user+role → Task 2 ✓；成员门（非成员 403）→ Task 4/6 ✓；排在 auth 之后、共用 skipper → Task 5 ✓。
- §4.5 未认证路由不经门 → Task 4 skipper + Task 5 复用 auth skipper ✓。
- §7 非成员 403、缺 scope 场景/解析错误 500 → Task 4 ✓。
- 本阶段范围外（阶段 2/3/4）：`users.role` 删除、`IsAdmin` 改造、RLS、by-id 收口——**不在本计划**，各自成计划。

**占位符扫描：** 无 TBD/TODO。测试辅助（`newIntegrationQueries`/`seedUser` 等）明确要求复用 `internal/db` 既有集成测试 helper，并给了不存在时的最小补法——非占位。not-found 错误语义给了明确判定（以 sqlc `:one` 生成的 `pgx.ErrNoRows` 为准）。

**类型一致性：** `Scope{TeamID,UserID,Role}`、`TeamResolver.Resolve(ctx,userID)(Scope,error)`、`MembershipReader.Membership(ctx,teamID,userID)(string,bool,error)`、`ResolveTeamMiddleware(resolver,userID,skipper)`、`GetTeamMembershipParams{TeamID,UserID}`/`GetTeamMembershipRow{Role}`——跨 Task 1–6 一致。

**后续计划（不在本文件）：** 阶段 2「鉴权收敛（删 users.role、IsAdmin 改读 scope 角色、引导授 owner、People 页管 team_members）」、阶段 3「RLS 强制 + 部署角色」、阶段 4「by-id 查询收口」。
