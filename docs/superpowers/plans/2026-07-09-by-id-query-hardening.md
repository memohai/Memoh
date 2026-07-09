# by-id 查询收口 实施计划（阶段 4）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给按主键（`WHERE id = sqlc.arg(id)`）却不带 `team_id` 过滤的查询补上 `AND team_id = sqlc.arg(team_id)`，使应用层自身就正确（不依赖 RLS 兜底），并加契约测试防回退。

**Architecture:** 逐条查询加 `team_id` 谓词 → `mise run sqlc-generate` 让生成方法多出 `TeamID` 参数 → `internal/db/postgres/store/team_compat.go` 里对应包装自动注入 `teamUUIDFromContext(ctx)`（沿用现有模式）；直接调用点按需补 `TeamID`。契约测试断言这批查询都带 `team_id`。

**Tech Stack:** Go、sqlc、`internal/db`。

**依赖：** 与阶段 3（RLS）互补但独立；可在阶段 1/2 之后任意时机做。参照 spec §4.3、§10。

## Global Constraints

- 修改 SQL 后 `mise run sqlc-generate`；`internal/db/postgres/sqlc/` 禁止手改。
- 每条 by-id 查询的 INSERT/UPDATE/DELETE/SELECT 加 `team_id` 后，其生成方法多出 `TeamID` 参数——确保调用路径（compat 包装或直接调用点）都填上，否则运行时按 `team_id = NULL` 命中零行。
- 有极少数按 id 的查询可能是"跨 team 基础设施"路径（例如已经故意做成 all-team 的）——**不在**本清单，勿误加。
- 提交信息结尾加：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 每任务后 `go build ./... && go test ./internal/... -count=1` 通过再提交。

## 待收口清单（审计产出）

来自扫描"含 `sqlc.arg(id)` 且整条查询不含 `team_id`"：

- **tool_approval / user_input：** `ApproveToolApprovalRequest`、`RejectToolApprovalRequest`、`UpdateToolApprovalPromptMessage`、`SubmitUserInputRequest`、`CancelUserInputRequest`、`FailUserInputRequest`、`UpdateUserInputAssistantMessage`、`UpdateUserInputPromptMessage`、`UpdateUserInputToolResultMessage`
- **models / providers：** `GetModelByID`、`DeleteModel`、`GetProviderByID`、`DeleteProvider`、`GetFetchProviderByID`、`DeleteFetchProvider`、`GetSearchProviderByID`、`DeleteSearchProvider`、`GetSpeechModelWithProvider`、`GetTranscriptionModelWithProvider`、`GetVideoModelWithProvider`
- **storage / settings：** `GetStorageProviderByID`、`UpsertBotSettings`

---

### Task 1: 收口 tool_approval / user_input 的 by-id 查询

**Files:**
- Modify: `db/postgres/queries/tool_approval.sql`、`db/postgres/queries/user_input.sql`
- Modify（自动生成）: `internal/db/postgres/sqlc/{tool_approval,user_input}.sql.go`
- Modify: `internal/db/postgres/store/team_compat.go`（对应包装注入 TeamID，或确认已注入）
- Test: `internal/db/postgres/store/team_compat_test.go`（或新建 `internal/db/team_byid_scope_test.go` 契约测试）

**Interfaces:**
- Produces: 上述查询的生成方法带 `TeamID`；compat 包装自动注入。

- [ ] **Step 1: 给每条查询加 team_id 谓词**

对 `tool_approval.sql` / `user_input.sql` 里清单中的每条，`WHERE id = sqlc.arg(id)` 后追加 `AND team_id = sqlc.arg(team_id)::uuid`。例如 `ApproveToolApprovalRequest`：

```sql
-- name: ApproveToolApprovalRequest :one
UPDATE tool_approval_requests
SET status = 'approved', ...
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid
RETURNING *;
```

（对 UPDATE/DELETE/SELECT 同理；保持各自原有 SET/RETURNING 不变，仅在 WHERE 增补。）

- [ ] **Step 2: 重生成**

Run: `mise run sqlc-generate`
Expected: 对应 `*Params` 结构体多出 `TeamID pgtype.UUID`。

- [ ] **Step 3: 确认/补 compat 注入**

`internal/db/postgres/store/team_compat.go`：检查这些方法的包装是否已注入 `TeamID`。若某方法此前是"透传"（因为原查询无 team_id），现在需要包装注入：

```go
func (q *Queries) ApproveToolApprovalRequest(ctx context.Context, arg dbsqlc.ApproveToolApprovalRequestParams) (dbsqlc.ToolApprovalRequest, error) {
	if !arg.TeamID.Valid {
		arg.TeamID = teamUUIDFromContext(ctx)
	}
	return q.Queries.ApproveToolApprovalRequest(ctx, arg)
}
```

（沿用 `CreateSessionEvent` 的既有模式。对每个"新增 TeamID 参数且原本无包装"的方法都加一个这样的薄包装。）

- [ ] **Step 4: 修编译 + 直接调用点**

Run: `go build ./... 2>&1 | head -30`
若某调用点直接构造 `sqlc.XxxParams{...}` 而不经 compat 包装，补上 `TeamID`（用该包既有的 `teams.WithTeamID`/`teamID` 模式）。

- [ ] **Step 5: 构建 + 相关测试**

Run: `go build ./... && go test ./internal/toolapproval/... ./internal/userinput/... ./internal/db/... -count=1`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add db/postgres/queries/tool_approval.sql db/postgres/queries/user_input.sql internal/db/postgres/sqlc/ internal/db/postgres/store/team_compat.go internal/
git commit -m "fix(team): scope tool_approval/user_input by-id queries to team

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 收口 models / providers / storage / settings 的 by-id 查询

**Files:**
- Modify: `db/postgres/queries/{models,providers,fetch_providers,search_providers,storage,settings}.sql`（按清单，实际文件名以 `grep -l` 定位）
- Modify（自动生成）: 对应 `internal/db/postgres/sqlc/*.sql.go`
- Modify: `internal/db/postgres/store/team_compat.go`
- Modify: 相关直接调用点（`internal/models`、`internal/providers`、`internal/fetchproviders`、`internal/searchproviders`、`internal/settings`、`internal/storage`）
- Test: 各包既有测试 + 契约测试

**Interfaces:**
- Consumes: 无新接口。

- [ ] **Step 1: 定位每条查询所在文件**

Run: `for q in GetModelByID DeleteModel GetProviderByID DeleteProvider GetFetchProviderByID DeleteFetchProvider GetSearchProviderByID DeleteSearchProvider GetSpeechModelWithProvider GetTranscriptionModelWithProvider GetVideoModelWithProvider GetStorageProviderByID UpsertBotSettings; do echo "$q: $(grep -rl "name: $q " db/postgres/queries/)"; done`

- [ ] **Step 2: 逐条加 team_id 谓词**

对每条 `WHERE id = sqlc.arg(id)`（或 `WHERE provider_id = ...` 等主键）后追加 `AND team_id = sqlc.arg(team_id)::uuid`。`Get*WithProvider` 这类含 JOIN 的，在**主表**（model/provider 所属）上加 `AND <alias>.team_id = sqlc.arg(team_id)::uuid`。

- [ ] **Step 3: 重生成**

Run: `mise run sqlc-generate`

- [ ] **Step 4: compat 注入 + 直接调用点**

同 Task 1 Step 3/4：为新增 TeamID 参数的方法加薄包装（`if !arg.TeamID.Valid { arg.TeamID = teamUUIDFromContext(ctx) }`）；直接调用点补 `TeamID`。

- [ ] **Step 5: 构建 + 测试**

Run: `go build ./... && go test ./internal/models/... ./internal/providers/... ./internal/fetchproviders/... ./internal/searchproviders/... ./internal/settings/... ./internal/db/... -count=1`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add db/postgres/queries/ internal/db/postgres/sqlc/ internal/db/postgres/store/team_compat.go internal/
git commit -m "fix(team): scope models/providers/storage/settings by-id queries to team

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 契约测试防回退

**Files:**
- Create: `internal/db/byid_team_scope_test.go`

**Interfaces:**
- Consumes: 无。

- [ ] **Step 1: 写契约测试**

`internal/db/byid_team_scope_test.go`——断言清单里的每条查询块都含 `team_id`（防止将来有人删掉谓词）：

```go
package db

import (
	"strings"
	"testing"

	embeddeddb "github.com/memohai/memoh/db"
)

func TestByIDQueriesAreTeamScoped(t *testing.T) {
	// 这些 by-id 查询必须带 team_id 过滤（阶段 4 收口，防回退）。
	want := map[string][]string{
		"postgres/queries/tool_approval.sql": {"ApproveToolApprovalRequest", "RejectToolApprovalRequest", "UpdateToolApprovalPromptMessage"},
		"postgres/queries/user_input.sql":    {"SubmitUserInputRequest", "CancelUserInputRequest", "FailUserInputRequest", "UpdateUserInputAssistantMessage", "UpdateUserInputPromptMessage", "UpdateUserInputToolResultMessage"},
		// models/providers/storage/settings 文件名以 Task 2 Step 1 结果为准，逐一列入
	}
	for path, names := range want {
		data, err := embeddeddb.MigrationsFS.ReadFile(path) // 若 queries 不在 embed FS，改用 os.ReadFile 相对路径
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		blocks := strings.Split(string(data), "-- name: ")
		for _, n := range names {
			var found bool
			for _, b := range blocks[1:] {
				if strings.HasPrefix(b, n+" ") {
					found = true
					if !strings.Contains(b, "team_id") {
						t.Errorf("%s query %s missing team_id predicate", path, n)
					}
				}
			}
			if !found {
				t.Errorf("%s query %s not found", path, n)
			}
		}
	}
}
```

> `db/postgres/queries/` 是否在 `embeddeddb.MigrationsFS`？若不是，用 `os.ReadFile(filepath.Join("..","..","db","postgres","queries",name))`（参照 `internal/acl/sql_team_scope_test.go` 的 `readQueryFile`）。

- [ ] **Step 2: 跑测试确认通过**

Run: `go test ./internal/db/ -run TestByIDQueriesAreTeamScoped -count=1`
Expected: PASS。

- [ ] **Step 3: 提交**

```bash
git add internal/db/byid_team_scope_test.go
git commit -m "test(team): contract test locking by-id queries to team scope

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec 覆盖（§4.3）：** 22 条 by-id 查询补 team_id（Task 1/2）✓；契约测试防回退（Task 3）✓。
**占位符扫描：** 无 TBD。Task 2 用 `grep -l` 动态定位文件名（真实命令，非占位）；契约测试的 embed vs os.ReadFile 给了明确判据。
**类型一致性：** 各方法新增 `TeamID pgtype.UUID` 参数，compat 注入模式统一沿用 `teamUUIDFromContext(ctx)`。
**风险：** 极少数 by-id 查询若实为跨 team 基础设施路径（不应加 team_id），Task 2 Step 1 定位后需人工确认其调用点确实是"某个 team 内"语义再加；若发现是 all-team 路径则从清单剔除并在提交说明里记录。
