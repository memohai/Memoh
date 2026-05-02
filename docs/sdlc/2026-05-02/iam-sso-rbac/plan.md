# IAM SSO RBAC 重构实现计划

## 执行状态

- 已完成：数据库 IAM schema/migration、PostgreSQL/SQLite baseline、SQLC 查询与生成代码。
- 已完成：password identity、session JWT、OIDC/SAML SSO 登录、SSO one-time code exchange。
- 已完成：RBAC service、LRU TTL cache、group inherited role、bot-scoped user/group role assignment。
- 已完成：`bot_acl_rules` 保持外部 channel ACL，绑定用户后串联 `bot.chat`。
- 已完成：IAM 管理 API、OpenAPI、SDK、登录 SSO UI、IAM 管理 UI。
- 已完成：OIDC `client_secret` 管理接口响应脱敏，编辑保存脱敏占位值时保留原 secret。
- 已验证：`mise run sqlc-generate`、`mise run swagger-generate`、`./node_modules/.bin/openapi-ts -f openapi-ts.config.ts`、`mise exec -- go test ./internal/... ./cmd/...`、`./apps/web/node_modules/.bin/vite build`。
- 阻断：`mise run lint` 被本机工具链阻断，`pnpm lint` 因 `ERR_PNPM_IGNORED_BUILDS` 停止，`golangci-lint` 因当前二进制使用 Go 1.25 而依赖文件要求 Go 1.26 panic。

## 方案说明

本计划基于 `docs/sdlc/2026-05-02/iam-sso-rbac/spec.md`，一次性完成 IAM 表命名统一、password identity 迁移、RBAC、session、OIDC/SAML SSO 登录、用户组映射、bot 权限接入和前端管理入口。

核心实现策略：

- 数据库先行：PostgreSQL 增量 migration 和 baseline 同步更新，SQLite baseline 和 `0003_iam_sso_rbac` 同步更新，所有数据迁移由 SQL migration 自动完成。
- 权限统一：删除 `users.role` 语义，所有 system/bot 授权通过 `internal/iam/rbac.Service.HasPermission`。
- 登录统一：password/OIDC/SAML 都产出 `iam_sessions` 和 Memoh JWT，JWT 只包含 `user_id/session_id/iat/exp`。
- Channel ACL 保持独立：`bot_acl_rules` 继续控制外部 channel `chat.trigger`，绑定用户时再串联 `bot.chat`。
- 不做兼容层：旧 API/UI 的 `role` 字段删除，前后端和 SDK 一次性更新。

## 子代理并行策略

实现阶段按依赖分批并行推进，写入范围必须互不重叠。关键接口按顺序落地，测试和前端页面在接口稳定后并行。

Batch 1:

- Worker A 数据库 migration/query：`db/postgres/**`、`db/sqlite/**`。
- Worker B DB store 接口草案：等 Worker A 生成 sqlc 后接手 `internal/db/**`。

Batch 2:

- Worker C IAM accounts/auth/rbac：`internal/iam/accounts/**`、`internal/iam/auth/**`、`internal/iam/rbac/**`。
- Worker D SSO core：`internal/iam/sso/**`，不修改 `*_test.go`。
- Worker F 后端单元测试：只改 `internal/iam/**/*_test.go`，包含 accounts/auth/rbac/sso 测试。

Batch 3:

- Worker E Auth handler：`internal/handlers/auth.go`。
- Worker G 权限接入和 IAM 管理 API：`internal/handlers/**` 中除 `auth.go` 外的文件、`internal/bots/**`、`internal/channel/inbound/channel.go`、`internal/command/handler.go`。
- Worker H Channel/IAM 表名适配：`internal/acl/**`、`internal/channel/identities/**`、`internal/bind/**`。

Batch 4:

- Worker I OpenAPI/SDK：`spec/**`、`packages/sdk/**`。
- Worker J Web login：`apps/web/src/pages/login/**`、`apps/web/src/store/user.ts`。
- Worker K Web IAM 管理页面：新增 IAM 管理页面路由、列表、表单、保存、脱敏字段展示和错误态组件。

合并顺序：

1. Worker A 完成 migration/query 后运行 `mise run sqlc-generate`。
2. Worker B 修复 `internal/db/**` 适配生成类型。
3. Worker C/D 并行实现 IAM core 和 SSO core。
4. Worker E/G/H 接入 handlers、bot、channel、command。
5. Worker I 生成 OpenAPI/SDK。
6. Worker J/K 接前端。
7. Worker F 补齐测试并执行最终验证。

## 改动文件

数据库：

- `db/postgres/migrations/0077_iam_sso_rbac.up.sql`：新增自动数据迁移。
- `db/postgres/migrations/0077_iam_sso_rbac.down.sql`：新增回滚迁移。
- `db/postgres/migrations/0001_init.up.sql`：更新 PostgreSQL baseline 到最终 schema。
- `db/postgres/migrations/0001_init.down.sql`：更新 baseline rollback。
- `db/sqlite/migrations/0001_init.up.sql`：更新 SQLite baseline 到最终 schema。
- `db/sqlite/migrations/0001_init.down.sql`：更新 baseline rollback。
- `db/sqlite/migrations/0003_iam_sso_rbac.up.sql`：新增 SQLite 自动数据迁移。
- `db/sqlite/migrations/0003_iam_sso_rbac.down.sql`：新增 SQLite 回滚迁移。
- `db/postgres/queries/users.sql`：改为 `iam_users` 和 `iam_identities`。
- `db/sqlite/queries/users.sql`：同步改为 `iam_users` 和 `iam_identities`。
- `db/postgres/queries/iam.sql`：新增 RBAC/session/SSO/group 查询。
- `db/sqlite/queries/iam.sql`：新增 SQLite 对应查询。
- `db/postgres/queries/acl.sql`：引用 `iam_channel_identities`、`iam_users`。
- `db/sqlite/queries/acl.sql`：同步更新引用。
- `db/postgres/queries/acl.sql`：改名引用和 list join。
- `db/postgres/queries/bind.sql`：改名引用。
- `db/postgres/queries/channel_identities.sql`：改名引用。
- `db/postgres/queries/channels.sql`：改名引用。
- `db/postgres/queries/messages.sql`：改名 `sender_channel_identity_id` 相关引用。
- `db/postgres/queries/user_provider_oauth.sql`：改名引用。
- `db/postgres/queries/users.sql`：改为 `iam_users` + password `iam_identities`。
- `db/sqlite/queries/acl.sql`：同步更新。
- `db/sqlite/queries/bind.sql`：同步更新。
- `db/sqlite/queries/channel_identities.sql`：同步更新。
- `db/sqlite/queries/channels.sql`：同步更新。
- `db/sqlite/queries/messages.sql`：同步更新。
- `db/sqlite/queries/user_provider_oauth.sql`：同步更新。
- `db/sqlite/queries/users.sql`：同步更新。
- `internal/db/postgres/sqlc/**`：由 `mise run sqlc-generate` 生成。
- `internal/db/sqlite/sqlc/**`：由 `mise run sqlc-generate` 生成。
- `internal/db/store/queries.go`：补充 IAM 查询接口。
- `internal/db/store/contracts.go`：替换 AccountStore 输入输出模型。
- `internal/db/postgres/store/accounts.go`：适配 password identity。
- `internal/db/sqlite/store/accounts.go`：同步适配。
- `internal/db/sqlite/store/queries.go`：同步转换 sqlc 参数。

后端 IAM：

- `internal/iam/accounts/types.go`：新增无 role 的 Account、创建/更新/密码 DTO。
- `internal/iam/accounts/service.go`：实现 password identity 登录、用户 CRUD、密码更新。
- `internal/iam/auth/jwt.go`：新增 session_id claim，保留 chat token 逻辑。
- `internal/iam/auth/session_middleware.go`：校验 `iam_sessions` 和 `iam_users.is_active`。
- `internal/iam/rbac/types.go`：定义 permission、role、resource 常量。
- `internal/iam/rbac/service.go`：实现 `HasPermission`、LRU TTL cache、role assignment 管理。
- `internal/iam/rbac/bootstrap.go`：seed 内置 permissions/roles/role_permissions。
- `internal/iam/sso/types.go`：定义 SSO provider config、attribute mapping、normalized profile。
- `internal/iam/sso/service.go`：实现 JIT/link、group sync、session issuance 和 one-time login code 协调。
- `internal/iam/sso/oidc.go`：实现 OIDC start/callback。
- `internal/iam/sso/saml.go`：实现 SAML start/ACS/metadata。
- `internal/iam/sso/cookie.go`：实现 state/nonce/code_verifier 短期 cookie。
- `internal/iam/sso/login_code.go`：实现 SSO one-time login code 创建和兑换。

后端接入：

- `internal/handlers/auth.go`：接入 password login/logout/refresh/SSO routes，移除 role response。
- `internal/handlers/users.go`：用 `rbac.HasPermission` 替换 `IsAdmin`，移除 role 请求/响应。
- `internal/handlers/handler_helpers.go`：改为 permission helper。
- `internal/handlers/message.go`：替换 admin 判断。
- `internal/bots/service.go`：`AuthorizeAccess` 改为 permission-based，创建/转移 owner 同步 `iam_principal_roles`。
- `internal/acl/service.go`：保留 ACL 逻辑，适配 `iam_channel_identities`。
- `internal/channel/inbound/channel.go`：外部 channel 入口 ACL allow 后，绑定用户再检查 `bot.chat`。
- `internal/command/handler.go`：把 write command 的 owner 字符串判断改为 `bot.update` 或更细 permission 判断。
- `internal/channel/identities/service.go`：适配 `iam_channel_identities.user_id`。
- `internal/bind/service.go`：适配 `iam_channel_identity_bind_codes` 和 `iam_user_channel_bindings`。
- `cmd/agent/app.go`：FX wiring 新增 IAM 服务，重写 bootstrap admin。
- `cmd/memoh/login.go`：适配 login response 移除 role。
- `internal/tui/api.go`：适配 login response 移除 role。
- `go.mod` / `go.sum`：新增 OIDC/SAML/LRU 依赖。

前端：

- `apps/web/src/store/user.ts`：移除 `role`，保留 profile 和 token。
- `apps/web/src/pages/login/index.vue`：增加 SSO provider 列表和跳转，password login 不再读 role。
- `apps/web/src/pages/login/sso-callback.vue`：用一次性 code 调 `POST /auth/sso/exchange`，拿到 JWT 后写入 store。
- `apps/web/src/lib/api-client.ts`：确认 Authorization header 行为不变。
- `apps/web/src/pages/**`：移除 role 字段使用。
- 新增 IAM 管理页面：SSO providers、groups、group mappings、bot permissions。
- `spec/swagger.json` / `spec/swagger.yaml`：由 `mise run swagger-generate` 更新。
- `packages/sdk/**`：由 `mise run sdk-generate` 更新。

测试：

- `internal/iam/accounts/service_test.go`
- `internal/iam/auth/jwt_test.go`
- `internal/iam/auth/session_middleware_test.go`
- `internal/iam/rbac/service_test.go`
- `internal/iam/sso/service_test.go`
- `internal/iam/sso/oidc_test.go`
- `internal/iam/sso/saml_test.go`
- `internal/handlers/auth_test.go`
- `internal/handlers/users_test.go`
- `internal/bots/service_test.go`
- `internal/acl/service_test.go`
- `internal/db/migrate_test.go`
- `apps/web/src/store/user.test.ts`
- `apps/web/src/pages/login/index.test.ts`

## 代码片段

JWT 只携带 session 信息：

```go
claims := jwt.MapClaims{
    "sub":        userID,
    "user_id":    userID,
    "session_id": sessionID,
    "iat":        now.Unix(),
    "exp":        expiresAt.Unix(),
}
```

RBAC 统一入口：

```go
allowed, err := rbacService.HasPermission(ctx, rbac.Check{
    UserID:        userID,
    PermissionKey: rbac.PermissionBotChat,
    ResourceType:  rbac.ResourceBot,
    ResourceID:    botID,
})
if err != nil {
    return err
}
if !allowed {
    return ErrBotAccessDenied
}
```

外部 channel 串联判断：

```go
allowedByACL, err := aclService.Evaluate(ctx, aclReq)
if err != nil || !allowedByACL {
    return allowedByACL, err
}
if linkedUserID != "" {
    return rbacService.HasPermission(ctx, rbac.Check{
        UserID:        linkedUserID,
        PermissionKey: rbac.PermissionBotChat,
        ResourceType:  rbac.ResourceBot,
        ResourceID:    botID,
    })
}
return true, nil
```

SSO email linking：

```go
if provider.EmailLinkingPolicy == LinkExisting && provider.TrustEmail && profile.EmailVerified {
    user, err := users.GetByEmail(ctx, profile.Email)
    if err == nil {
        return identities.Link(ctx, user.ID, profile)
    }
}
```

这些片段必须保持最小示意，不直接复制为最终实现。最终实现以 sqlc 生成类型和现有错误处理风格为准。

## 考量与权衡

- 当前仓库没有 `.codex/rules/` 或 `rules/workflow.md` 文件，根目录 `AGENTS.md` 和本任务 `spec.md` 是约束来源。
- 数据迁移必须由 `golang-migrate` 自动执行，不能要求管理员手动搬数据。
- PostgreSQL 和 SQLite 必须同步更新 schema、query 和生成代码。
- `iam_principal_roles` 只支持 allow，避免 deny 冲突；外部入口 deny 继续由 `bot_acl_rules` 承担。
- 不在 JWT 中放权限，撤权延迟由 30s LRU TTL 控制。
- SSO provider 私钥保存在 `config` JSON，API 响应必须脱敏。
- 删除旧 `role` 字段是破坏性 API/UI 变更，本 PR 不提供兼容字段。
- 这个 PR 面向官方主分支，必须包含 migration、单元测试、集成测试、OpenAPI/SDK 生成和前端验证。

## 影响的测试

必须新增或更新：

- Password login 从 `iam_identities` 校验 bcrypt。
- JWT 包含 `session_id`，session revoked 后请求 401。
- 禁用 `iam_users.is_active=false` 后请求 401。
- Bootstrap admin 创建 admin user、password identity、system admin assignment。
- Migration 将旧 `users.role/password_hash` 迁到 RBAC 和 password identity。
- RBAC user direct assignment 命中。
- RBAC group inherited assignment 命中。
- `resource_id=NULL` bot global assignment 命中。
- bot owner migration 后拥有 `bot_owner` 权限。
- `bot_acl_rules` 未绑定用户只看 ACL。
- `bot_acl_rules` 绑定用户时额外检查 `bot.chat`。
- OIDC state/nonce 校验、email linking policy、group mapping sync。
- SAML ACS 校验、NameID subject、group mapping sync。
- SSO callback URL 不暴露 JWT，one-time code exchange 成功后才返回 JWT。
- 前端 password login 不再依赖 role。
- 前端 SSO provider 列表和 redirect 行为。

## Todo List

### Phase 1: 数据库 schema 与 migration

- [ ] 阅读 `db/postgres/migrations/0076_container_workspace_backend.up.sql` 和 `db/sqlite/migrations/0002_container_workspace_backend.up.sql`，确认下一版本号和迁移风格。
- [ ] 在 `db/postgres/migrations/0077_iam_sso_rbac.up.sql` 添加 `CREATE TYPE` / `ALTER TYPE` 预备语句。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 先重命名 `users -> iam_users`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 重命名 `channel_identities -> iam_channel_identities`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 重命名 `user_channel_bindings -> iam_user_channel_bindings`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 重命名 `channel_identity_bind_codes -> iam_channel_identity_bind_codes`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 重命名 `user_provider_oauth_tokens -> iam_user_provider_oauth_tokens`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 调整 `iam_users` indexes 和 constraints 名称。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_identities`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_sessions`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_login_codes`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_sso_providers`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_groups`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_group_members`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_sso_group_mappings`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_permissions`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_roles`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_role_permissions`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 创建全新表 `iam_principal_roles`。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 写入内置 permission seed。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 写入内置 role seed。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 写入 role_permissions seed。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 迁移 password identity。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 迁移 `users.role` 到 system role assignment。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 迁移 `bots.owner_user_id` 到 bot owner assignment。
- [ ] 在 `0077_iam_sso_rbac.up.sql` 删除 `iam_users.password_hash`、`iam_users.role`、`user_role` enum。
- [ ] 在 `db/postgres/migrations/0077_iam_sso_rbac.down.sql` 先恢复 `user_role` enum。
- [ ] 在 `0077_iam_sso_rbac.down.sql` 给 `iam_users` 恢复 `role` 和 `password_hash` 列。
- [ ] 在 `0077_iam_sso_rbac.down.sql` 从 `iam_identities` 写回 password hash。
- [ ] 在 `0077_iam_sso_rbac.down.sql` 从 system role assignment 写回 `users.role`。
- [ ] 在 `0077_iam_sso_rbac.down.sql` drop 新增 IAM 表。
- [ ] 在 `0077_iam_sso_rbac.down.sql` rename `iam_*` 旧表回原名。
- [ ] 更新 `db/postgres/migrations/0001_init.up.sql` 到最终 schema。
- [ ] 更新 `db/postgres/migrations/0001_init.down.sql` 的 drop 顺序。
- [ ] 在 `db/sqlite/migrations/0003_iam_sso_rbac.up.sql` 按 SQLite rebuild-table 方式迁移 `users -> iam_users`。
- [ ] 在 `0003_iam_sso_rbac.up.sql` 创建 SQLite 全新 IAM 表。
- [ ] 在 `0003_iam_sso_rbac.up.sql` 迁移 password identity、system role、bot owner role。
- [ ] 在 `db/sqlite/migrations/0003_iam_sso_rbac.down.sql` 写 SQLite rebuild-table 回滚迁移。
- [ ] 更新 `db/sqlite/migrations/0001_init.up.sql` 到最终 schema。
- [ ] 更新 `db/sqlite/migrations/0001_init.down.sql` 的 drop 顺序。

### Phase 2: SQL queries 与生成代码

- [ ] 更新 `db/postgres/queries/users.sql`，改为 `iam_users` + password `iam_identities`。
- [ ] 更新 `db/sqlite/queries/users.sql`，保持 PostgreSQL 等价语义。
- [ ] 新增 `db/postgres/queries/iam.sql` 的 session 查询。
- [ ] 新增 `db/sqlite/queries/iam.sql` 的 session 查询。
- [ ] 在 `iam.sql` 添加 RBAC permission check 查询。
- [ ] 在 `iam.sql` 添加 one-time login code create/exchange 查询。
- [ ] 在 `iam.sql` 添加 role/permission seed upsert 查询。
- [ ] 在 `iam.sql` 添加 principal role assignment CRUD 查询。
- [ ] 在 `iam.sql` 添加 SSO provider CRUD 查询。
- [ ] 在 `iam.sql` 添加 identity lookup/link 查询。
- [ ] 在 `iam.sql` 添加 group/group member/group mapping 查询。
- [ ] 更新 `db/postgres/queries/acl.sql` 的 `iam_channel_identities` / `iam_users` join。
- [ ] 更新 `db/sqlite/queries/acl.sql` 的等价 join。
- [ ] 更新 `db/postgres/queries/bind.sql` 的 bind code 和 channel identity 表名。
- [ ] 更新 `db/sqlite/queries/bind.sql` 的等价表名。
- [ ] 更新 `db/postgres/queries/channel_identities.sql` 的表名。
- [ ] 更新 `db/sqlite/queries/channel_identities.sql` 的表名。
- [ ] 更新 `db/postgres/queries/channels.sql` 的 channel identity 表名。
- [ ] 更新 `db/sqlite/queries/channels.sql` 的 channel identity 表名。
- [ ] 更新 `db/postgres/queries/messages.sql` 的 channel identity FK/join 表名。
- [ ] 更新 `db/sqlite/queries/messages.sql` 的 channel identity FK/join 表名。
- [ ] 更新 `db/postgres/queries/user_provider_oauth.sql` 的表名。
- [ ] 更新 `db/sqlite/queries/user_provider_oauth.sql` 的表名。
- [ ] 运行 `mise run sqlc-generate`。
- [ ] 修复 `internal/db/store/queries.go` 中 sqlc 接口编译错误。
- [ ] 修复 `internal/db/sqlite/store/queries.go` 中 PostgreSQL/SQLite 类型转换错误。
- [ ] 修复 `internal/db/store/contracts.go` 的 AccountStore 模型。

### Phase 3: IAM auth/accounts/session 核心

- [ ] 创建 `internal/iam/accounts/types.go`，移除 role 字段。
- [ ] 创建 `internal/iam/accounts/service.go`，迁移 `internal/accounts/service.go` 中 profile CRUD。
- [ ] 在 accounts service 实现 password identity 登录。
- [ ] 在 accounts service 实现本地用户创建并写 password identity。
- [ ] 在 accounts service 实现用户密码更新到 `iam_identities.credential_secret`。
- [ ] 在 accounts service 实现管理员重置 password identity。
- [ ] 创建 `internal/iam/auth/jwt.go`，加入 `session_id` claim。
- [ ] 保留 chat token claims，避免 channel route token 回归。
- [ ] 创建 `internal/iam/auth/session_middleware.go`，验证 session 和 active user。
- [ ] 修改 `internal/server/server.go`，JWT middleware 串接 session 验证。
- [ ] 更新 `internal/auth/jwt_test.go` 到 `internal/iam/auth/jwt_test.go`。
- [ ] 新增 session middleware revoked/inactive/expired 测试。
- [ ] 删除或迁移旧 `internal/accounts` 和 `internal/auth` 引用。

### Phase 4: RBAC 核心

- [ ] 创建 `internal/iam/rbac/types.go`，定义 permission、role、resource 常量。
- [ ] 创建 `internal/iam/rbac/service.go`，实现 `HasPermission`。
- [ ] 在 RBAC service 中接入 `expirable.LRU`，TTL 30s、max entries 10000。
- [ ] 实现 user direct role 查询路径。
- [ ] 实现 group inherited role 查询路径。
- [ ] 实现 bot `resource_id=NULL` global assignment 查询路径。
- [ ] 实现 `system.admin` 全局管理员判断。
- [ ] 创建 `internal/iam/rbac/bootstrap.go`，封装内置 seed 校验。
- [ ] 新增 role assignment 管理方法，限制 bot owner 只能授予 bot scope role。
- [ ] 新增 RBAC 单元测试覆盖 direct/group/global/system.admin。
- [ ] 新增缓存 TTL 和权限变更延迟测试。

### Phase 5: Bootstrap 与 FX wiring

- [ ] 修改 `cmd/agent/app.go` provider，注册 IAM accounts/auth/rbac/sso 服务。
- [ ] 重写 `ensureAdminUser` 为 `ensureIAMBootstrap`。
- [ ] 在 bootstrap 中确保内置 permissions/roles/role_permissions。
- [ ] 在 bootstrap 中确保 admin `iam_users`。
- [ ] 在 bootstrap 中确保 admin password `iam_identities`。
- [ ] 在 bootstrap 中确保 admin system role assignment。
- [ ] 更新相关构造函数参数，消除旧 `accounts.Service` 编译错误。
- [ ] 新增 bootstrap 测试覆盖空库和已有库。

### Phase 6: SSO service

- [ ] 创建 `internal/iam/sso/types.go`，定义 OIDC/SAML config 和 mapping structs。
- [ ] 创建 `internal/iam/sso/cookie.go`，实现 state/nonce/code_verifier cookie。
- [ ] 在 `cookie.go` 实现 `SetOIDCStateCookie`。
- [ ] 在 `cookie.go` 实现 `ReadAndClearOIDCStateCookie`。
- [ ] 创建 `internal/iam/sso/login_code.go`，实现随机 code 生成和 hash。
- [ ] 在 `login_code.go` 实现 `CreateLoginCode` 写入 `iam_login_codes`。
- [ ] 在 `login_code.go` 实现 `ExchangeLoginCode` 的过期和 used_at 校验。
- [ ] 创建 `internal/iam/sso/service.go`，实现 provider lookup。
- [ ] 在 `service.go` 实现 `FindOrProvisionUser` 主流程。
- [ ] 在 `service.go` 实现 `LinkExistingUserByEmail`。
- [ ] 在 `service.go` 实现 `CreateJITUser`。
- [ ] 在 `service.go` 实现 `UpdateIdentitySnapshot`。
- [ ] 在 `service.go` 实现 `SyncMappedGroups`。
- [ ] 在 `SyncMappedGroups` 中只删除当前 provider 的 `source=sso` membership。
- [ ] 创建 `internal/iam/sso/oidc.go`，实现 `BuildOIDCAuthRedirect`。
- [ ] 在 `oidc.go` 实现 `ExchangeOIDCCode`。
- [ ] 在 `oidc.go` 实现 `VerifyOIDCIDToken`。
- [ ] 在 `oidc.go` 实现 `NormalizeOIDCClaims`。
- [ ] 在 `oidc.go` 串接 callback 到 `FindOrProvisionUser` 和 `CreateLoginCode`。
- [ ] 创建 `internal/iam/sso/saml.go`，实现 `BuildSAMLMetadata`。
- [ ] 在 `saml.go` 实现 `BuildSAMLAuthRedirect`。
- [ ] 在 `saml.go` 实现 `ParseAndValidateSAMLResponse`。
- [ ] 在 `saml.go` 实现 `NormalizeSAMLAssertion`。
- [ ] 在 `saml.go` 串接 ACS 到 `FindOrProvisionUser` 和 `CreateLoginCode`。
- [ ] 新增 OIDC 单元测试，使用 `httptest` fake issuer 和 JWKS。
- [ ] 新增 OIDC nonce/audience/issuer 错误测试。
- [ ] 新增 SAML 单元测试，使用固定测试证书和 signed response fixture。
- [ ] 新增 SAML Audience/ACS URL/signature 错误测试。
- [ ] 新增 group mapping sync 测试。

### Phase 7: Auth handlers

- [ ] 修改 `internal/handlers/auth.go` imports 到 `internal/iam/accounts` 和 `internal/iam/auth`。
- [ ] 修改 `LoginResponse`，移除 `role`，增加 `session_id`。
- [ ] 修改 password `Login`，创建 session 后签发 JWT。
- [ ] 新增 `POST /auth/logout`，撤销当前 session。
- [ ] 修改 `POST /auth/refresh`，校验 session 后续期并签发新 JWT。
- [ ] 新增 `GET /auth/sso/providers`，只返回 enabled providers 和脱敏信息。
- [ ] 新增 OIDC start/callback handlers。
- [ ] 新增 SAML start/ACS/metadata handlers。
- [ ] 新增 `POST /auth/sso/exchange` handler，使用 one-time login code 返回 JWT。
- [ ] 修改 `internal/server/server.go::shouldSkipJWT`，放行 SSO start/callback/metadata/exchange。
- [ ] 新增 auth handler tests 覆盖 login/logout/refresh/SSO routes/exchange。

### Phase 8: 用户与 bot handler 权限替换

- [ ] 修改 `internal/handlers/handler_helpers.go`，新增 `RequirePermission` helper。
- [ ] 在 `internal/handlers/users.go::ListUsers` 使用 `system.admin`。
- [ ] 在 `GetUser` 保留 self 访问，非 self 使用 `system.admin`。
- [ ] 在 `UpdateUser`、`ResetUserPassword`、`CreateUser` 使用 `system.admin`。
- [ ] 在 `CreateBot` owner override 使用 `system.admin`。
- [ ] 在 `ListBots` owner filter 使用 `system.admin`。
- [ ] 修改 `GetBot` 使用 `bot.read`。
- [ ] 修改 `UpdateBot` 使用 `bot.update`。
- [ ] 修改 `DeleteBot` 使用 `bot.delete`。
- [ ] 修改 `TransferBotOwner` 使用 `bot.permissions.manage` 或 `system.admin`。
- [ ] 修改 channel config handlers 使用 `bot.update`。
- [ ] 修改 send/chat handlers 使用 `bot.chat`。
- [ ] 修改 `internal/handlers/message.go` 中 admin 判断为 RBAC。
- [ ] 更新 `internal/command/handler.go::MemberRoleResolver` 为 permission resolver。
- [ ] 更新 `internal/command/handler.go::ExecuteWithInput`，write command 使用 permission 判断。
- [ ] 更新 users handler tests 覆盖 forbidden/allowed。

### Phase 9: Bot service 与 ACL 串联

- [ ] 修改 `internal/bots/service.go::AuthorizeAccess` 签名，移除 `isAdmin` 参数。
- [ ] 在 `AuthorizeAccess` 中调用 RBAC `bot.read`。
- [ ] 在 `Create` 成功后写 `bot_owner` role assignment。
- [ ] 在 `TransferOwner` 中同步删除旧 owner assignment 并写新 owner assignment。
- [ ] 在 `ListAccessible` 中改为 RBAC 查询可见 bot。
- [ ] 保留 `bots.owner_user_id` 字段作为归属和查询字段。
- [ ] 修改 `internal/acl/service.go` 只做 channel ACL，不引入 IAM。
- [ ] 修改 `internal/channel/inbound/channel.go` 的 ACL 判断点，ACL allow 后读取 linked user。
- [ ] 在 `internal/channel/inbound/channel.go` 绑定用户分支增加 `bot.chat` RBAC 检查。
- [ ] 在 `internal/channel/inbound/channel.go` 未绑定用户分支保持只看 `bot_acl_rules`。
- [ ] 更新 channel inbound 相关测试覆盖绑定/未绑定两条路径。
- [ ] 更新 `internal/bots/service_test.go`。
- [ ] 更新 `internal/acl/service_test.go` 覆盖重命名表和串联策略。

### Phase 10: IAM 管理 API

- [ ] 新增 `internal/handlers/iam.go` 注册 IAM 管理 routes。
- [ ] 添加 SSO provider CRUD API，要求 `system.admin`。
- [ ] 添加 group CRUD API，要求 `system.admin`。
- [ ] 添加 group member 管理 API，要求 `system.admin`。
- [ ] 添加 SSO group mapping API，要求 `system.admin`。
- [ ] 添加 role list/detail API，要求 `system.admin`。
- [ ] 添加 role permission 管理 API，要求 `system.admin`。
- [ ] 添加 principal role assignment API，system/global bot 要求 `system.admin`。
- [ ] 添加 bot scoped principal role assignment API，要求 `bot.permissions.manage`。
- [ ] API 响应中对 SAML private key、OIDC client_secret 脱敏。
- [ ] 新增 IAM handler tests 覆盖权限和脱敏。

### Phase 11: 表名改名影响修复

- [ ] 更新 `internal/channel/identities/service.go` 到 `iam_channel_identities` sqlc 类型。
- [ ] 更新 `internal/bind/service.go` 到 `iam_channel_identity_bind_codes`。
- [ ] 更新 user provider OAuth 相关 store/handler 到 `iam_user_provider_oauth_tokens`。
- [ ] 更新 provider OAuth、email OAuth、MCP OAuth 不属于 IAM 的表引用，确认未误改。
- [ ] 全库 `rg \"\\busers\\b|channel_identities|user_channel_bindings|channel_identity_bind_codes|user_provider_oauth_tokens\"`，逐项确认 SQL 和 Go 引用。
- [ ] 全库 `rg \"\\.Role|role:\" internal apps/web/src packages/sdk`，移除旧 user role 依赖。

### Phase 12: OpenAPI、SDK 与前端

- [ ] 更新 swaggo annotations，移除 LoginResponse role。
- [ ] 添加 auth SSO endpoints annotations。
- [ ] 添加 IAM management endpoints annotations。
- [ ] 运行 `mise run swagger-generate`。
- [ ] 运行 `mise run sdk-generate`。
- [ ] 修改 `apps/web/src/store/user.ts`，移除 role。
- [ ] 修改 `apps/web/src/pages/login/index.vue`，password login 不再读 role。
- [ ] 在 login 页面加载 enabled SSO providers。
- [ ] 在 login 页面添加 SSO provider 按钮和 redirect。
- [ ] 新增 SSO callback 页面，从 URL 读取 one-time code。
- [ ] 在 SSO callback 页面调用 `POST /auth/sso/exchange`。
- [ ] SSO exchange 成功后写入 `useUserStore` token。
- [ ] SSO exchange 失败后清理本地 token 并回到 login。
- [ ] 新增 IAM 管理路由和导航入口。
- [ ] 新增 SSO provider 列表页面。
- [ ] 新增 SSO provider 创建/编辑表单。
- [ ] SSO provider 表单支持 OIDC config 字段。
- [ ] SSO provider 表单支持 SAML metadata/config 字段。
- [ ] SSO provider 详情和列表中脱敏 `client_secret` / private key 字段。
- [ ] 新增 SSO provider 保存错误态和 loading 态。
- [ ] 新增 IAM group 列表页面。
- [ ] 新增 IAM group 创建/编辑表单。
- [ ] 新增 SSO group mapping 列表页面。
- [ ] 新增 SSO group mapping 创建/编辑表单。
- [ ] 新增 bot permissions 页面入口。
- [ ] 新增 bot permissions user/group assignment 列表。
- [ ] 新增 bot permissions role assignment 表单。
- [ ] bot permissions 页面限制角色选择为 bot scope roles。
- [ ] 新增 IAM 管理页面空态、错误态和 loading 态。
- [ ] 更新前端 i18n 文案。
- [ ] 新增或更新 Vitest 测试。

### Phase 13: 迁移与集成测试

- [ ] 新增 PostgreSQL migration 测试 fixture：旧 users role/password_hash/bots owner。
- [ ] 验证 PostgreSQL `0077 up` 后 identity、roles、principal roles 正确。
- [ ] 验证 PostgreSQL `0077 down` 能恢复旧 schema。
- [ ] 新增 SQLite migration 测试 fixture。
- [ ] 验证 SQLite `0003 up` 后 identity、roles、principal roles 正确。
- [ ] 验证 password login 使用迁移后的 password identity。
- [ ] 验证 session revoke 后 protected endpoint 401。
- [ ] 验证 inactive user protected endpoint 401。
- [ ] 验证 admin bootstrap 空库创建完整 RBAC。
- [ ] 验证 bot owner 可 read/update/delete 自己 bot。
- [ ] 验证 group bot_viewer 只能 read/chat。
- [ ] 验证 bot global role 对所有 bot 生效。
- [ ] 验证 SSO group mapping 只同步命中 group。
- [ ] 验证未映射 external group 不创建本地 group。
- [ ] 验证 OIDC callback URL 不包含 JWT。
- [ ] 验证 SSO one-time code 只能 exchange 一次。
- [ ] 验证过期 SSO one-time code exchange 返回 401。

### Phase 14: 最终验证

- [ ] 运行 `mise run sqlc-generate`，确认无 diff 或提交生成物。
- [ ] 运行 `mise run swagger-generate`。
- [ ] 运行 `mise run sdk-generate`。
- [ ] 运行 `go test ./internal/iam/...`。
- [ ] 运行 `rg "github.com/memohai/memoh/internal/(accounts|auth)" internal cmd`，确认旧包 import 已清空。
- [ ] 运行 `go test ./internal/handlers ./internal/bots ./internal/acl`。
- [ ] 运行 `go test ./internal/db/...`。
- [ ] 运行 `go test ./cmd/agent`。
- [ ] 运行前端相关 Vitest。
- [ ] 运行 `mise run lint`。
- [ ] 在 PostgreSQL dev 环境执行 `go run ./cmd/agent migrate up`。
- [ ] 在 SQLite dev 环境执行 `go run ./cmd/agent migrate up`。
- [ ] 手动验证 password login、SSO provider list、bot permissions 管理、channel ACL 串联。
- [ ] 全库 `rg "role"`，确认剩余命中不是旧 `users.role` 语义。
- [ ] 全库 `rg "users|channel_identities"`，确认剩余命中不是旧表名。
