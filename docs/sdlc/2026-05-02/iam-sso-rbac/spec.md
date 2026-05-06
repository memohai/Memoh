# IAM SSO RBAC 重构规格

## 问题定义

Memoh 当前认证和权限模型以 `users` 表为中心：

- 本地账号密码直接存在 `users.password_hash`
- 全局权限通过 `users.role = member | admin` 表达
- bot Web/API 访问通过 `isAdmin || bots.owner_user_id == userID` 判断
- 外部渠道触发 bot 使用独立的 `bot_acl_rules`

这个模型无法表达企业 SSO 和用户组权限：

- 一个用户需要同时绑定 password、OIDC、SAML、channel 等多个身份
- OIDC/SAML 登录需要 JIT 创建或绑定已有用户
- SSO group 需要映射到 Memoh group
- user/group 需要获得 system scope 或 bot scope 的 role
- `users.role` 只能表达全局 admin/member，无法表达 per-bot 权限

本次重构目标是一次性引入新的 IAM 域模型，移除 `users.role` 和 `users.password_hash` 的业务语义，支持 OIDC/SAML SSO 登录、用户组、RBAC 授权，并保持 `bot_acl_rules` 作为外部 channel 入口 ACL。

## 非目标

- 不实现 SCIM。
- 不引入 organization / tenant。
- 不支持 group 嵌套。
- 不实现 OAuth2/OIDC Provider。
- 不把权限写入 JWT。
- 不把 `bot_acl_rules` 合并进 IAM RBAC。
- 不保留旧 `users.role` API/UI 兼容层。

## 依赖

使用当前 Go proxy latest 版本：

```text
github.com/zitadel/oidc/v3 v3.47.5
github.com/crewjam/saml v0.5.1
github.com/hashicorp/golang-lru/v2/expirable
```

用途：

- `github.com/zitadel/oidc/v3`: OIDC Relying Party，使用 Authorization Code + PKCE。
- `github.com/crewjam/saml`: SAML Service Provider，处理 AuthnRequest、ACS、SAMLResponse 验证。
- `github.com/hashicorp/golang-lru/v2/expirable`: RBAC permission check 的进程内 LRU TTL cache。

## 命名决策

IAM 域内表统一使用 `iam_*` 前缀。

现有表一次性重命名：

```text
users -> iam_users
channel_identities -> iam_channel_identities
user_channel_bindings -> iam_user_channel_bindings
channel_identity_bind_codes -> iam_channel_identity_bind_codes
user_provider_oauth_tokens -> iam_user_provider_oauth_tokens
```

保留 `bot_acl_rules` 名称。它属于 bot 外部入口规则，不属于 IAM RBAC。

## 数据模型

### iam_users

用户主体，只存 profile、状态和默认展示信息。

```text
id
username
email
display_name
avatar_url
timezone
data_root
is_active
metadata
last_login_at
created_at
updated_at
```

约束：

```text
username UNIQUE
email UNIQUE WHERE email IS NOT NULL AND email <> ''
```

移除：

```text
role
password_hash
```

### iam_identities

所有登录身份和外部身份绑定。

```text
id
user_id
provider_type: password | oidc | saml | channel
provider_id
subject
credential_secret
email
username
display_name
avatar_url
raw_claims
last_login_at
created_at
updated_at
```

约束：

```text
UNIQUE(provider_type, provider_id, subject)
```

本地密码迁移到：

```text
provider_type = password
provider_id = NULL
subject = lower(username)
credential_secret = 原 users.password_hash
```

OIDC subject：

```text
provider_type = oidc
provider_id = iam_sso_providers.id
subject = issuer + "|" + sub
```

SAML subject：

```text
provider_type = saml
provider_id = iam_sso_providers.id
subject = NameID
```

### iam_sessions

服务端 session，用于撤销、禁用用户后立即失效和登录审计。

```text
id
user_id
identity_id
issued_at
expires_at
revoked_at
ip_address
user_agent
metadata
created_at
updated_at
```

JWT claims：

```text
sub
user_id
session_id
iat
exp
```

JWT 不包含：

```text
roles
permissions
groups
```

### iam_login_codes

SSO callback 使用一次性登录 code 交付 token，避免把 JWT 放入 URL。

```text
id
code_hash
user_id
identity_id
session_id
expires_at
used_at
created_at
```

约束：

```text
UNIQUE(code_hash)
expires_at <= created_at + 2 minutes
```

流程：

```text
1. SSO callback 验证成功后创建 iam_sessions。
2. 生成随机 code，只存 code_hash。
3. redirect 到 Web callback 页面，URL 只带 code。
4. 前端调用 POST /auth/sso/exchange。
5. 后端校验 code 未过期且未使用，标记 used_at，返回 JWT。
```

### iam_sso_providers

OIDC/SAML provider 配置。

```text
id
type: oidc | saml
key
name
enabled
config
attribute_mapping
jit_enabled
email_linking_policy
trust_email
created_at
updated_at
```

`email_linking_policy`：

```text
link_existing
reject_existing
```

默认：

```text
jit_enabled = true
email_linking_policy = link_existing
trust_email = false
```

OIDC `config` 示例：

```json
{
  "issuer": "https://accounts.google.com",
  "client_id": "...",
  "client_secret": "...",
  "scopes": ["openid", "profile", "email", "groups"],
  "redirect_url": "https://memoh.example.com/auth/sso/google/callback"
}
```

SAML `config` 示例：

```json
{
  "entity_id": "https://memoh.example.com/auth/sso/acme/saml/metadata",
  "metadata_xml": "...",
  "acs_url": "https://memoh.example.com/auth/sso/acme/saml/acs"
}
```

`attribute_mapping` 示例：

```json
{
  "subject": "sub",
  "email": "email",
  "email_verified": "email_verified",
  "display_name": "name",
  "avatar_url": "picture",
  "groups": ["groups", "roles", "memberOf"]
}
```

### iam_groups

扁平用户组。

```text
id
key
display_name
source: local | sso | scim
external_id
metadata
created_at
updated_at
```

不支持嵌套 group。

### iam_group_members

用户组成员关系。

```text
id
group_id
user_id
source: manual | sso | scim
provider_id
created_at
updated_at
```

约束：

```text
UNIQUE(group_id, user_id, source, provider_id)
```

### iam_sso_group_mappings

SSO 外部 group 到 Memoh group 的显式映射。

```text
id
provider_id
external_group
group_id
created_at
updated_at
```

约束：

```text
UNIQUE(provider_id, external_group)
```

SSO 登录不自动创建 `iam_groups`。只同步已配置 mapping 命中的 group。

### iam_permissions

代码定义的能力点。

```text
id
key
description
is_system
created_at
updated_at
```

内置 permission：

```text
system.login
system.admin
bot.read
bot.chat
bot.update
bot.delete
bot.permissions.manage
```

不允许管理员自定义 permission。

### iam_roles

permission 的组合。

```text
id
key
scope: system | bot
display_name
description
is_system
created_at
updated_at
```

允许管理员自定义 role。

内置 role：

```text
member
admin
bot_viewer
bot_operator
bot_owner
```

### iam_role_permissions

role 到 permission 的映射。

```text
role_id
permission_id
created_at
```

内置映射：

```text
member -> system.login

admin -> system.login
admin -> system.admin

bot_viewer -> bot.read
bot_viewer -> bot.chat

bot_operator -> bot.read
bot_operator -> bot.chat
bot_operator -> bot.update

bot_owner -> bot.read
bot_owner -> bot.chat
bot_owner -> bot.update
bot_owner -> bot.delete
bot_owner -> bot.permissions.manage
```

### iam_principal_roles

user/group 在 system/bot scope 下的 role assignment。

```text
id
principal_type: user | group
principal_id
role_id
resource_type: system | bot
resource_id
source: system | manual | sso | scim
provider_id
created_at
updated_at
```

约束：

```text
resource_type = system -> resource_id IS NULL
resource_type = bot -> resource_id IS NULL OR resource_id = bot id
```

`resource_id = NULL` 表示该 `resource_type` 下的全局授权。

IAM RBAC 不支持 deny。撤权通过删除 assignment 完成。

## 登录流程

### Password

```text
1. 根据 input 查 iam_identities:
   provider_type='password'
   subject=lower(input)
   或 email=lower(input)
2. bcrypt 校验 credential_secret
3. 查 iam_users.is_active
4. 更新 iam_users.last_login_at 和 iam_identities.last_login_at
5. 创建 iam_sessions
6. 签发 Memoh JWT
```

### OIDC

入口：

```text
GET /auth/sso/:provider_id/start
GET /auth/sso/:provider_id/callback
```

流程：

```text
1. start 使用 Authorization Code + PKCE 生成 auth URL
2. state / nonce / code_verifier 使用安全短期存储
3. callback 校验 state
4. exchange code
5. 校验 id_token issuer / audience / nonce / signature / exp
6. 提取 sub / email / email_verified / profile / groups
7. 用 provider_id + issuer + sub 查 iam_identities
8. 找到则更新 identity snapshot 和 user profile
9. 未找到则执行 JIT/link
10. 同步 SSO group mappings
11. 创建 iam_sessions
12. 创建一次性 iam_login_codes
13. redirect 到 Web callback 页面
14. 前端调用 /auth/sso/exchange 换取 Memoh JWT
```

Email 自动绑定规则：

```text
email_linking_policy = link_existing
trust_email = true
email_verified = true
iam_users.email 命中
```

不满足时：

```text
email_linking_policy = reject_existing -> 拒绝登录
email 不存在且 jit_enabled = true -> 创建新用户
email 不存在且 jit_enabled = false -> 拒绝登录
```

### SAML

入口：

```text
GET  /auth/sso/:provider_id/saml/start
POST /auth/sso/:provider_id/saml/acs
GET  /auth/sso/:provider_id/saml/metadata
```

流程：

```text
1. start 生成 SP-initiated AuthnRequest
2. ACS 校验 SAMLResponse 签名、Audience、Destination、NotBefore、NotOnOrAfter
3. 提取 NameID / email / profile / groups
4. 用 provider_id + NameID 查 iam_identities
5. 找到则更新 identity snapshot 和 user profile
6. 未找到则执行 JIT/link
7. 同步 SSO group mappings
8. 创建 iam_sessions
9. 创建一次性 iam_login_codes
10. redirect 到 Web callback 页面
11. 前端调用 /auth/sso/exchange 换取 Memoh JWT
```

SAML 没有标准 `email_verified`。是否信任 email 只由 provider 的 `trust_email` 控制。

## SSO Group 同步

登录时执行：

```text
1. 从 OIDC claims 或 SAML attributes 提取 external groups
2. 使用 provider_id + external_group 查 iam_sso_group_mappings
3. 只保留命中的 iam_groups
4. upsert iam_group_members(source='sso', provider_id=当前 provider)
5. 删除该 provider 下本次未命中的 source='sso' membership
6. 不影响 source='manual'、source='scim' 或其他 provider 的 membership
```

## 授权模型

统一入口：

```text
HasPermission(ctx, userID, permissionKey, resourceType, resourceID)
```

查询主体：

```text
1. user direct role assignments
2. user group inherited role assignments
```

查询资源：

```text
resource_type = system, resource_id = NULL
resource_type = bot, resource_id = botID
resource_type = bot, resource_id = NULL
```

`system.admin` 可视为全局管理员能力。业务代码不再读取 `users.role`。

缓存：

```text
cache key = user_id + permission_key + resource_type + resource_id
ttl = 30s
max entries = 10000
```

权限变更不做分布式失效。30s TTL 作为撤权延迟上限。

## bot_acl_rules 分层

`bot_acl_rules` 保留。它控制外部 channel 消息能否触发 bot。

现有语义：

```text
action = chat.trigger
effect = allow | deny
subject_kind = all | channel_identity | channel_type
priority first-match-wins
bots.acl_default_effect fallback
source_conversation_type/source_conversation_id/source_thread_id 做入口 scope
```

它不是 IAM RBAC，因为它的主体是 channel identity / channel type，不是 iam_user / iam_group。

调用规则：

```text
Web/API chat:
  只判断 iam_rbac bot.chat

外部 channel chat:
  1. 判断 bot_acl_rules chat.trigger
  2. 如果 iam_channel_identities.user_id 非空，再判断 iam_rbac bot.chat
  3. 如果未绑定 iam_user，只判断 bot_acl_rules
```

## 管理权限

`system.admin`：

```text
管理 iam_sso_providers
管理 iam_sso_group_mappings
管理 iam_roles
管理 iam_role_permissions
管理 system scope principal roles
管理 bot global principal roles(resource_type='bot', resource_id=NULL)
```

`bot.permissions.manage`：

```text
管理指定 bot 的 user/group role assignment
只能授予 bot scope role
resource_type='bot'
resource_id=指定 bot_id
```

## Bootstrap Admin

保留配置文件 `[admin]` bootstrap 机制，但不写 `users.role`。

启动时确保：

```text
1. iam_users 中存在 admin user
2. iam_identities 中存在 password identity
3. iam_roles / iam_permissions / iam_role_permissions 中存在内置 RBAC seed
4. iam_principal_roles 中存在 admin user -> admin role -> system
```

## 数据库迁移

迁移不是人工操作。项目使用：

```text
github.com/golang-migrate/migrate/v4 v4.19.1
cmd/agent migrate up
internal/db/migrate.go
```

### PostgreSQL

新增：

```text
db/postgres/migrations/0077_iam_sso_rbac.up.sql
db/postgres/migrations/0077_iam_sso_rbac.down.sql
```

`0077_iam_sso_rbac.up.sql` 自动执行：

```text
1. 重命名 IAM 旧表为 iam_*。
2. 创建 iam_identities / iam_sessions / iam_login_codes / iam_sso_providers / iam_groups / iam_group_members / iam_roles / iam_permissions / iam_role_permissions / iam_principal_roles / iam_sso_group_mappings。
3. seed 内置 permissions、roles、role_permissions。
4. 将原 users.password_hash 写入 iam_identities(provider_type='password')。
5. 将原 users.role='admin' 写入 iam_principal_roles(system admin)。
6. 将原 users.role='member' 写入 iam_principal_roles(system member)。
7. 将 bots.owner_user_id 写入 iam_principal_roles(bot_owner, resource_type='bot', resource_id=bots.id)。
8. 删除 iam_users.password_hash。
9. 删除 iam_users.role。
10. 删除 user_role enum。
11. 修正所有 FK、index、constraint、query 引用。
```

同时更新 PostgreSQL baseline：

```text
db/postgres/migrations/0001_init.up.sql
db/postgres/migrations/0001_init.down.sql
```

### SQLite

更新 SQLite baseline：

```text
db/sqlite/migrations/0001_init.up.sql
db/sqlite/migrations/0001_init.down.sql
```

同时新增自动迁移：

```text
db/sqlite/migrations/0003_iam_sso_rbac.up.sql
db/sqlite/migrations/0003_iam_sso_rbac.down.sql
```

SQLite migration 也必须自动迁移已有数据，不能要求管理员手工搬迁。

### sqlc

所有 schema/query 变更同时更新：

```text
db/postgres/queries/*.sql
db/sqlite/queries/*.sql
```

然后运行：

```bash
mise run sqlc-generate
```

## API 影响

认证：

```text
POST /auth/login
POST /auth/logout
POST /auth/refresh
GET  /auth/sso/providers
GET  /auth/sso/:provider_id/start
GET  /auth/sso/:provider_id/callback
GET  /auth/sso/:provider_id/saml/start
POST /auth/sso/:provider_id/saml/acs
GET  /auth/sso/:provider_id/saml/metadata
POST /auth/sso/exchange
```

管理：

```text
iam users
iam identities
iam sessions
iam sso providers
iam groups
iam group members
iam roles
iam role permissions
iam principal roles
iam sso group mappings
```

旧 API/UI 中的 `role` 字段移除，不提供兼容字段。

## 代码影响范围

后端：

```text
internal/accounts -> internal/iam/accounts
internal/auth -> internal/iam/auth
internal/rbac
internal/sso
internal/handlers/auth.go
internal/bots/service.go
internal/acl
internal/db/store
internal/db/postgres/sqlc
internal/db/sqlite/sqlc
cmd/agent bootstrap wiring
cmd/memoh login
internal/tui api
```

前端：

```text
apps/web/src/pages/login
apps/web/src/store/user.ts
apps/web/src/lib/api-client.ts
SSO provider 管理 UI
Group 管理 UI
Bot permission 管理 UI
```

生成物：

```text
spec/swagger.*
packages/sdk
```

## 验证要求

数据库：

```bash
mise run sqlc-generate
go run ./cmd/agent migrate up
```

需要覆盖 PostgreSQL 和 SQLite migration track。

后端测试：

```text
password login
OIDC callback
SAML ACS
email linking policy
SSO group mapping sync
RBAC HasPermission direct user role
RBAC HasPermission group inherited role
bot owner migration
system admin bootstrap
bot_acl_rules + iam_rbac 串联
session revoke
inactive user request denied
```

前端验证：

```text
password login
SSO button list
SSO callback one-time code exchange
user store 不再依赖 role
bot permission management
group mapping management
```

## 开放问题

1. SAML SP signing/encryption certificate 的存储位置在本阶段定为 `iam_sso_providers.config`。私钥字段必须按敏感配置处理，API 响应默认脱敏。
2. OIDC state/nonce/code_verifier 使用短期、HttpOnly、Secure、SameSite=Lax cookie。callback 成功或失败后清理 cookie。
