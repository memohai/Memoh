# 团队多租户：鉴权 + RLS 强制隔离 —— 设计文档

- **日期：** 2026-07-09
- **分支：** `codex/team-multitenancy`
- **状态：** 提案（设计已通过头脑风暴确认；待书面 spec 评审）

## 1. 问题

团队多租户的基础工作已经给每张租户表加了 `team_id` 列、给大部分 sqlc 查询加了
按 team 的过滤谓词，但隔离并没有端到端真正生效：

1. **team 解析是空壳。** `teams.DefaultMiddleware()`（server.go:37）是目前唯一挂载
   的 team 中间件，它给每个请求**无条件塞默认 team**。没有按用户的 team 解析，也
   不校验调用者是否属于该 team。`teams.Scope` 里只有 `TeamID`——没有用户、没有角色。
2. **鉴权是全局的，不是按 team 的。** `users.role`（枚举 `user_role`：`member|admin`）
   是一个全局标志。`accountService.IsAdmin(...)` 读它，门卫了 ~15 处 handler。按 team
   的角色（`team_members.role`：`owner|admin|member`）虽然存在，但没有任何鉴权在读它。
3. **应用层隔离靠约定、且不完整。** 43 个查询文件里 35 个带 `team_id` 谓词，但部分
   按主键的查询没有（例如 `GetToolApprovalRequest` = `WHERE id = $1`），于是一行数据
   可以凭 id 被跨 team 读到。这类缺口本轮反复被发现和修复；正确性依赖于每条查询都被
   手工接对。
4. **RLS 形同虚设。** 策略是建了（`ENABLE ROW LEVEL SECURITY` +
   `team_isolation USING team_id = current_setting('app.team_id')`），但 RLS **没有**
   `FORCE`，而应用用角色 `memoh` 连接——它**既是表 owner 又是 superuser**，两者都绕过
   非 FORCE 的 RLS。实测已证明：设一个不匹配的 `app.team_id`，应用角色仍读到全部行；
   换一个普通非 owner 角色则正确返回零行。所以当前**没有**数据库层的兜底。

## 2. 目标 / 非目标

**目标**

- 每个请求解析出"当前 team"，并拒绝不属于该 team 的调用者（租户级成员资格门）。
- 让 `team_members.role` 成为权限的唯一来源；移除全局 `users.role`。
- 让 RLS 真正强制生效，作为数据库层兜底，默认在所有部署里都开。
- 保持机制多 team 可扩展，使 SaaS 产品（开源 Memoh 的上游消费者）能在其上叠加真多
  team；而开源版只暴露一个 team。

**非目标**

- 开源版里做用户可见的多 team 切换 / team 管理 UI（这归 SaaS）。
- 替换现有资源级鉴权（bot ACL 规则、`bot_user_grants`、channelaccess 的 Manage）。
  它们保留，并在 team **内部**运作。
- 开源版里做跨 team 的"平台超管"概念（SaaS 真需要可以自己加；它绝不能挂在开源版的
  `users` 表上）。

## 3. 已锁定的设计决策

1. **完整三层：** team 解析 + 成员/角色鉴权 + RLS 强制生效。
2. **用户↔team：** 数据模型是多对多（`team_members`）。开源版只暴露一个 team；SaaS 是
   上游那一层、暴露多个。team 解析是一个**可替换的注入点**——开源版接到"单 team"，
   SaaS 覆盖它。
3. **鉴权 = 成员资格门。** 校验已登录用户是否是所解析 team 的成员；非成员拒绝。资源级
   ACL/grants 不变，二者互补。
4. **`team_members.role` 是权限的唯一来源。** `users.role` + `user_role` 枚举移除。
   `users` 保持为**全局身份**表（无 `team_id`；不被 team 拥有）。team 拥有的是*成员关系*
   和*资源*，不是*身份*。`IsAdmin` 改为读调用者在所解析 team 里的角色。引导流程让首个
   用户成为 team `owner`。
5. **RLS 默认处处强制（方案 A）。** 运行时用一个专用的、非 owner、非 superuser 的角色
   连接；迁移用拥有/DDL 角色跑；所有租户表加 `FORCE ROW LEVEL SECURITY`；`app.team_id`
   按每个连接从请求 scope 注入。

## 4. 架构

四个协作的层，每层都可独立测试。

### 4.1 team 解析 + 成员门（请求边缘）

一个新中间件在**已认证路由**上取代写死的 `teams.DefaultMiddleware()`。

- **排在 `auth.JWTMiddleware`（server.go:64）之后**，因为它需要已认证的 `user_id`
  （`auth.UserIDFromContext`）。它和认证共用同一个 public 路由 skipper（登录、健康检查、
  inbound webhook 是未认证的、没有用户——见 4.5）。
- 使用可插拔接口：

  ```go
  // TeamResolver 解析并授权一个请求的"当前 team"。
  type TeamResolver interface {
      // Resolve 返回该用户正在操作的 team；若用户不是成员/未授权则返回错误。
      Resolve(ctx context.Context, userID string) (teams.Scope, error)
  }
  ```

- **开源版实现（`SingleTeamResolver`）：** 解析到唯一 team（默认 team），并校验
  `team_members` 里存在 `(team, user)`；不存在则返回"非成员"错误 → `403`。把成员的
  `role` 载入 scope。
- **SaaS** 提供自己的 resolver（当前 team 来自 JWT claim / 请求，成员 + 权限校验）。
  不在本仓库。
- `teams.Scope` 增加操作用户和角色，让下游鉴权无需再查库：

  ```go
  type Scope struct {
      TeamID string
      UserID string       // 操作用户（系统/后台上下文为空）
      Role   string       // 该 team 内的 team_members.role：owner|admin|member
  }
  ```

- 解析出的 scope 注入请求 context（一如现在的 `teams.WithScope`），并为 RLS 驱动每连接
  的 `app.team_id`（见 4.4）。

### 4.2 鉴权收敛（`users.role` → `team_members.role`）

- **删除** `users.role` 和 `user_role` 枚举（迁移，见 5）。
- **`accountService.IsAdmin`** 从"读 `users.role`"变为"所解析 scope 的角色是否为
  `owner` 或 `admin`"。签名从 `IsAdmin(ctx, channelIdentityID)` 改为读
  `teams.ScopeFromContext(ctx)`（那 ~15 处调用点已经跑在带该 scope 的请求 context 里）。
  一个薄 helper：

  ```go
  func (s *Service) IsAdmin(ctx context.Context) (bool, error) // 角色 ∈ {owner, admin}
  ```

  `internal/handlers/{users,session,message,acp_runtime,...}.go` 里的调用点切到 ctx 形式。
  这是本次改动最大的机械面。
- **引导流程（`ensureAdminUser`）：** 首个用户被注册进默认 team、角色为 `owner`，而不是写
  `users.role='admin'`。`teams.EnsureDefault` 已经在补成员，扩展它给引导 admin 授予
  `owner`。
- **People 页**（`internal/handlers/users.go`）语义："加人"= 确保存在全局 `users` 行 +
  在当前 team 建一条带角色的 `team_members`；"删人"= 删该 `team_members` 链接（开源单
  team 下 ≈ 移除此人）。由当前 team 的 `owner|admin` 把关。没有任何成员关系的全局 `users`
  行是惰性的（解析不出 team ⇒ 过不了成员门）。

### 4.3 by-id 查询收口（与 RLS 纵深互补）

- 审计每条查询，找出漏 `team_id` 谓词的（`GetToolApprovalRequest` 那类）。RLS 生效后
  （见 4.4）这些其实已经被兜住，但仍显式补上谓词，让应用层自身正确、错误语义清晰
  （not-found vs forbidden）。产出：一份需要补 `AND team_id = sqlc.arg(team_id)` 的
  by-id 查询清单。

### 4.4 RLS 强制生效（数据库兜底）

- **迁移（DDL 角色）：**
  - 对所有租户表 `ALTER TABLE ... FORCE ROW LEVEL SECURITY`（`team_isolation` 策略已存在）。
  - 建角色 `memoh_app`：`LOGIN`，**非** superuser，**非** owner；对所有租户表 + `teams`、
    `team_members`、`users` `GRANT SELECT, INSERT, UPDATE, DELETE`；对序列
    `GRANT USAGE, SELECT`；对 schema `public` `GRANT USAGE`。表 owner 仍是 DDL 角色。
- **运行时连接：** 应用用 `memoh_app` 连接。迁移继续用拥有/DDL 角色跑。
- **每连接注入 scope：** 在 pgx 连接池构造处（`internal/db/db.go`）加
  `AfterConnect`/`BeforeAcquire`，让每个取出的连接执行
  `SELECT set_config('app.team_id', $1, false)`，值来自请求 scope 的 `TeamID`。
  `team_dbtx` 现有的事务内 `SET LOCAL` 保留；之前重构成直接跑在池上的 singleton 路径由
  池钩子拿到 scope。
- **进程级 / 后台上下文**（启动 reconcile、bootstrap、all-team 基础设施查询）没有按请求
  的 team，且天然要跨 team。在 `FORCE` RLS 下这是个陷阱：策略是
  `team_id = NULLIF(current_setting('app.team_id', true), '')::uuid`，所以受限角色在
  `app.team_id` **未设**时，`team_id = NULL` 永不成立，每条 all-team 查询返回**零行**。
  因此这些路径必须走一个**绕过 RLS** 的连接——一个带 `BYPASSRLS` 的专用维护角色（或
  拥有/DDL 角色）——仅用于那 4 条 all-team 基础设施查询
  （`ListAutoStartContainers`、`ListEnabledSchedules`、`ListHeartbeatEnabledBots`、
  `ListBotChannelConfigsByType`）和启动 bootstrap。这是一个小而枚举的白名单，放在独立的
  连接池上，不是通用查询路径。下游再在受限池上按每行的 `team_id` 重新 scope。

### 4.5 未认证入口

- **登录、健康检查、静态资源：** 无 team、无 scope；不设门。
- **inbound 渠道 webhook：** 没有用户。它跨 team 解析出目标 bot/config（那 4 条基础设施
  查询已是 all-team），再在该资源的 `team_id` 下运作。webhook 路径的 `app.team_id` 来自
  解析出的资源行，而不是用户 scope。

## 5. 数据模型改动

新迁移 `0106_team_authz_rls`：

- `ALTER TABLE users` → **删** `users.role`；`DROP TYPE user_role`（先移除依赖对象）。
  down：重建枚举 + 列，从 `team_members` 回填（`owner|admin` → `admin`，否则 `member`）。
- 对每张租户表 `ALTER TABLE <tenant> FORCE ROW LEVEL SECURITY`（对照 0105 里的 ENABLE
  清单）。
- `CREATE ROLE memoh_app ...` + 授权（带幂等守卫）。
- 按仓库"canonical schema"规则，`0001_init.up.sql` 更新到最终状态（删
  `users.role`/枚举、FORCE RLS、角色 + 授权）。
- 无新增列。`users` 保持全局（无 `team_id`）。

## 6. 部署 / 配置改动

- **配置：** `PostgresConfig` 增加一个运行时凭据（`memoh_app`），与迁移凭据（owner）区分。
  两个运行时连接池：**主受限池**（`memoh_app`，RLS 强制）承载所有请求/按 team 的工作；
  一个小的**维护池**用 owner/`BYPASSRLS` 的 DSN，仅用于那 4 条枚举的 all-team 启动查询
  （见 4.4）。迁移用 owner DSN。
- **docker-compose / entrypoints：** （经迁移）创建 `memoh_app` 并把它的凭据传给 `server`
  服务；`migrate` 服务保留 owner 凭据。
- **示例配置**（`conf/app.example.toml`、docker/apple/windows）更新运行时角色。
- 因为方案 A 让这成为默认，开源版 `docker compose up` 路径必须自动 provision 该角色
  （迁移建角色 + compose env），保持自托管一条命令搞定。

## 7. 错误处理

- 不是所解析 team 的成员 → `403 Forbidden`（区别于未认证的 `401`）。
- 角色不足以执行某 admin 操作 → `403`。
- 该有 scope 却缺（bug：已认证路由没解析出）→ `500`，记日志；已认证路由上**绝不**静默
  回退到默认 team。
- 被 RLS 过滤掉的 by-id 查询返回 not-found，而不是别的 team 的行。

## 8. 测试策略

- **resolver/中间件：** 单测——成员通过、scope 带角色；非成员 403；public 路由跳过。
- **鉴权：** `IsAdmin` 读 scope 角色；owner/admin 通过、member 失败；那 ~15 个调用点在有
  handler 测试处覆盖。
- **RLS（集成，真 Postgres）：** 以 `memoh_app`、`app.team_id` = team A，无法读/改/删
  team B 的行（INSERT/SELECT/UPDATE/DELETE）；以 owner 角色迁移仍正常。扩展现有
  `team_dbtx`/契约测试。**这是证明"形同虚设"被修好的那个测试。**
- **by-id 收口：** 契约测试断言被审计的 by-id 查询都带 `team_id`。
- **迁移：** scratch 库上 up/down/up 循环（同 CI）；验证 `users.role` 已删、枚举已删、
  FORCE 已设、角色已建；down 能还原。
- 全量 `go build` / `go vet` / `go test` / `golangci-lint` / sqlc 幂等。

## 9. 分阶段（有序；每阶段可独立发布）

1. **Scope 带上 user+role；resolver + 成员门中间件**（开源单 team resolver）。挂在认证之后；
   public 路由保持原行为。
2. **鉴权收敛：** 删 `users.role`/枚举（迁移 0106 第一部分），改造 `IsAdmin` + ~15 处调用点，
   引导流程给首用户授 owner。
3. **RLS 强制：** FORCE + `memoh_app` 角色 + 授权（迁移 0106 第二部分），连接池 `app.team_id`
   钩子，配置/compose 运行时角色，后台上下文处理。
4. **by-id 查询收口** + 契约测试。

阶段 1–2 是应用层、部署风险低。阶段 3 带部署变更（角色/连接），合入前需要 RLS 集成测试。

## 10. 风险 / 待定问题

- **`IsAdmin` 波及面（~15 处）。** 机械但面广；有几处拿 `channelIdentityID` 用不同方式解析
  用户——逐个核实能否干净映射到请求 scope。
- **后台/webhook 的 `app.team_id`。** FORCE RLS 下受限连接不设 `app.team_id` 会返回零行，
  所以启动 reconcile/bootstrap 必须用 `BYPASSRLS`（或 owner）维护池跑那 4 条枚举的 all-team
  查询（见 4.4）。webhook 路由从解析出的资源行取 team、再为后续受限池工作设 `app.team_id`
  （见 4.5）。确认没有任何被强制 RLS 的查询坐在"无 scope 的受限连接"上。
- **托管 Postgres。** 用迁移建角色假定有足够权限；为托管库记录 owner/DDL 凭据的要求。
- **开源单 team 下 RLS 实际收益≈0**（没有第二个租户可挡）；方案 A 接受这份多出来的自托管
  复杂度，换取机制统一、SaaS 免费继承真隔离。若自托管摩擦成为支持负担，重新权衡这一取舍。
