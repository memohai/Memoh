# Slash Quick Action 与 Skill Slash 方案

本文是 Web quick action、文本 channel slash、以及 skill-as-slash 的当前权威方案。
实现前和每轮修改后都要以本文为准交给 3 个 subagent review；review 意见需要做对抗性
验证，只采纳会导致行为、安全或落地边界不闭合的问题。

当前版本：V2.23
更新时间：2026-07-02

## 当前结论

Slash 不是纯前端输入补全。它是一套跨 Web 和 channel 的后端权威分类与执行入口：

- 固定 quick action 来自现有 command system，但 Web 只暴露明确 allowlist 的结构化 action。
- 动态 skill 可以通过 slash 菜单或文本语法请求加载，但必须通过 safe catalog 和
  opaque `skill_ref`，不能把 `raw`、`content`、`source_path` 暴露给 runtime API。
- 所有 slash 都要在附件读取、附件 normalize、session 创建、message persist、agent
  pipeline 前完成分类。未知 slash 不能回落成普通聊天进入 LLM。
- Requested skills 只进入本轮模型上下文，不拼进用户可见文本，也不进入 history、title、
  memory extraction 或 passive persist。

## MVP 边界

MVP 覆盖：

- Web composer slash 菜单、typed slash、command panel。
- Web 主发送链路的 WS streaming requested skills。
- HTTP quick action / safe catalog 使用 OpenAPI/SDK DTO；WS outbound 使用本地 typed payload。
- 文本 channel slash，包括 Telegram、Discord、Lark、DingTalk、WeChat、Matrix 等适配器
  转入的文本消息。
- `/skill use` 作为特殊 early intent，而不是普通 command handler 的字符串命令。

MVP 不覆盖：

- Slack、Discord、Feishu 等平台原生 slash command 注册。
- 把动态 skill 注册进 Telegram native bot command menu。
- ACP、retry/edit、tool approval continuation、user input continuation、schedule/heartbeat、
  DCP/discuss already-in-context、active-stream inject、`UserMessagePersisted=true` 路径的
  requested skills。
- 把 legacy channel callback / `SyntheticCommand` 全部迁移到 Web structured executor。

## Review 协议

每次修改本文后：

1. 发给 3 个 subagent，分别从 backend/API/channel、skills runtime/security、Web/WS/product
   角度 review。
2. 对每条意见做对抗性验证。
3. Blocking 只有在方案本身不闭合、与现有结构冲突、或会造成安全/行为漏洞时才采纳。
4. 当前代码还没实现本文不是 Blocking；那属于后续实现工作。
5. 采纳后必须先更新本文，再进入下一轮 3-agent review。

## 现状与目标态差异

本文描述目标态，不是现有代码已经具备的能力。实现时必须显式处理这些改造项：

- Web WS 主链路当前直接进入 `StreamChatWS`，绕过 command layer；目标态要在 handler 边界
  server-side classify，unknown slash 不再作为普通文本进 LLM。
- `command.Handler` 当前不是 FX singleton，而是在 channel router provider 中局部创建；
  目标态要提升为 shared dependency，供 channel inbound、LocalChannelHandler、quick-action
  HTTP handler 共享。
- Web slash 菜单当前存在自然语言插入式 skill 雏形，且可能调用返回 `raw/content/source_path`
  的管理端 skill API；目标态必须迁移到 safe catalog + chip / typed payload，并移除
  “Use the skill ...” 类文本插入路径。
- Web 当前可能在发送前先创建/选择 session；目标态中 requested skill 和 slash-shaped Web
  输入必须先完成 server preflight，再创建新 session。失败请求不能留下空 session。
- `supportsMode` 目前不是独立能力位，channel 代码用 local/non-local 判断；目标态由 classifier
  input 显式提供。mode prefix 包括 `/now`、`/btw`、`/next`。
- `RawQuery` 当前参与 display/title 等路径；目标态必须把这些消费点迁移到
  `PersistQuery/UserVisibleText`，不能继续让原始 slash 影响 display、title、memory、dedupe。
- `UserMessagePersisted=true` 不能覆盖所有拒绝路径。schedule/heartbeat、active-stream inject、
  DCP/discuss、continuation 等必须独立拒绝。
- channel `RouteDispatcher` 的 active 状态必须支持多 owner；普通主 stream 与 tool/user-input
  continuation 重叠时，continuation 释放自己的 owner 不能把主 stream 标记为 idle，也不能提前
  drain queue / pending persist。
- undirected group slash 当前可能被 passive persist；目标态是 hard no-op，不回复、不 persist、
  不 ingest 附件、不进 LLM。
- 现有 `/skill` / `/skill list` 是只读 command；目标态只截获 `/skill use` 为 requested-skill
  intent，`/skill` 和 `/skill list` 继续走现有 command。
- session used skills 查询当前只从 assistant `use_skill` tool call 聚合；目标态需要 PG/SQLite
  同步改为结构化 rows，并保留 handler 降级输出能力。
- desktop/local 不能复用公开默认 `jwt_secret` 作为 SkillRef key；目标态必须有专用、
  首启生成并持久化的 key。

## 简化后的交付切片

本文的安全边界不降级，但实现顺序必须收敛成 5 个可验收切片：

1. **共同底座**：shared classifier、safe catalog、SkillRef resolver、reserved metadata guard、
   query split。先保证 slash 不误入 LLM，skill content 不误入 history/title/memory。
2. **Web quick action**：只做 `help`、`skill.list`、command panel、unknown/unsupported slash。
   不把所有 legacy command Web 化。
3. **Web skill chip**：safe catalog chip + WS `requested_skills` + no-session preflight + streaming。
   Web 手打 `/skill use` 始终拒绝。
4. **Channel skill slash**：只支持普通 directed chat 的 `/skill use a,b -- prompt`，其他复杂
   context fail-closed。
5. **审计和展示收尾**：structured audit rows、PG/SQLite parity、session used skills sanitized
   display、i18n 和端到端验收。

每个切片都必须满足对应 fail-closed 需求；不能用“后续补安全边界”的方式先上线前端菜单。

## Shared Classifier

新增 `internal/slash` 包，负责纯分类：

- 不 import `conversation`、`agent`。
- 不硬编码 command 列表。
- `KnownCommandPredicate` / command catalog 由 `command.Handler` 或 command package 注入。
- FX 提供一个 `command.Handler` singleton，同时注入 channel inbound、`LocalChannelHandler`、
  quick-action HTTP handler。不能在 Web/local handler 里 new 第二套 handler。

分类输入：

- `text`
- `hasAttachments`
- `surface`，例如 `web_ws`、`web_rest`、`channel`
- adapter 原始 `directed`
- `supportsMode`
- bot mention aliases
- command predicate / action capability predicate

分类前先计算：

```text
effectiveDirected = directed ||
  leadingMentionMatchesBotAlias(text) ||
  slashCommandSuffixMatchesBotAlias(text)
```

这保证 group 中的 `/help@Bot`、`/skill@Bot use ...` 不会被 undirected no-op 吞掉。

分类顺序：

1. 空文本或非 slash：`NormalChat`。
2. group channel 且 `effectiveDirected=false` 且 slash：`RejectNoop`。必须无回复、无 pipeline、
   无 passive persist、无 LLM。
3. 识别 `/skill use` / `/skill@Bot use` 为 `SkillIntentCandidate`，优先于 known command。
4. 支持 mode 的 channel 识别 mode prefix，例如 `/now xxx`、`/btw xxx`、`/next xxx`。Web
   `supportsMode=false`，mode slash 返回 `command_error`，不交给 LLM。
5. channel mode prefix strip 后重跑分类；mode + 任何 slash-shaped remainder 一律 reject，
   包括 `/btw /help`、`/now /skill use`、`/next /wat`。
6. known command：
   - channel surface：进入现有 channel command path。
   - Web surface：只有 action schema 明确 `allowed_surfaces` 包含 Web 且可结构化执行时返回
     `CommandAction`；否则返回 `UnsupportedCommand`。
7. `SkillIntentCandidate`：
   - 带附件：`Reject`。
   - Web surface：不解析 name，不返回 `ContinueChatIntent`；直接返回
     `use_skill_chip_required`，要求用户走 safe catalog chip。这样 stale 或恶意 Web client
     不能绕过 chip/ref 路径。
   - 缺少 `--` 或 selector：`invalid_skill_slash_syntax`。
   - 有 `--` 但 prompt 为空：`missing_prompt`。
   - channel surface 语法合法时才返回 `ContinueChatIntent`。
8. unknown slash：`UnknownSlash` / `command_error`，绝不作为普通聊天进入 LLM。

`hasAttachments` 包括直接附件、带附件的 reply/quoted/forward/media group 引用。slash command 和
`/skill use` 带附件都在文件读取、normalize、ingest、persist 前拒绝。这个策略偏严格，
但可以避免“slash control message”夹带上游媒体上下文进入不可审计路径；错误码使用
`slash_attachments_unsupported`。
各 channel adapter 必须在分类前提供可判定信号：直接附件、reply/quote/forward/media group
是否包含附件。无法可靠判定的 adapter 对 slash 控制消息应 fail-closed 或明确降级为“不支持
引用附件判定”的错误路径，不能先 ingest 后再决定。

错误优先级：

- undirected group slash 先 hard no-op，同时不 ingest 附件。
- directed slash-shaped 且带附件时，先返回 `slash_attachments_unsupported`，再考虑
  unknown/unsupported/syntax。
- Web client classifier 只做确定的本地 UX 分流，例如已知 CommandAction、skill chip 提示、
  明确 Web 手打 `/skill use` 的 `use_skill_chip_required`。其余 slash-shaped 输入必须交给
  server-side classifier 兜底，避免客户端 catalog 过期误杀新命令。

## Web Ingress

Web composer 的 `handleSend` 第一阶段必须在下面行为之前运行 client classifier：

- 清 draft
- 读取 FileReader
- 创建 session
- 调用 `chatStore.sendMessage`
- 插入 optimistic turn

对 slash-shaped text 或包含 `requested_skills` 的发送，Web 还必须走 server preflight 语义：
本地 classifier 只能做 UX 提示，不能为了失败请求提前创建新 session。若当前 composer
没有 session，server-side classifier 和 requested skill resolver 必须先通过，成功后才创建
或选择 session 并进入 streaming。

Web 的主发送链路继续使用 WS streaming。带 skill chip 的正常发送必须扩展 WS client message，
而不是降级成非流式 REST。

WS client outbound payload 增加：

```json
{
  "invocation_id": "client-generated uuid for slash/command classification correlation",
  "composer_scope": "client tab or draft scope, optional but required before session exists",
  "session_id": "optional existing session id",
  "text": "user visible prompt",
  "attachments": [],
  "model_id": "optional",
  "reasoning_effort": "optional",
  "base_head_turn_id": "optional",
  "requested_skills": [
    {
      "skill_ref": "v1.kid.nonce.ciphertext",
      "name": "skill-name"
    }
  ]
}
```

WS 继续沿用现有 `text` 字段，不把主链路迁移成 `message`。typed REST 可以使用
HTTP DTO 的 `message` 字段，但 handler 必须在 classifier 前把它映射到同一份用户可见文本。

WS server handler 必须在附件 normalize / ingest、new session ensure、optimistic store 前：

1. 校验 bot access；如果 `session_id` 存在则校验 session/head access，如果不存在则只校验
   bot/head 可见性并延迟 session 创建。
2. 运行 server-side classifier。
3. 调用 `RejectReservedSkillMetadata`。
4. 校验 `requested_skills` 只来自 typed field，不来自 metadata。
5. 解析并校验 `requested_skills`、limits、context size、runtime-usable 状态。
6. 以上 preflight 全部成功后，才允许创建/选择 session 并构造
   `conversation.ChatRequest.RequestedSkills`。

如果第 2-5 步失败，server 返回带 `invocation_id` / `composer_scope` / `session_id` 的
`command_error` 或 typed error；Web 用它归属到当前 command panel，不创建新 session、
turn、assistant stream 或 history。

`base_head_turn_id` 与 requested skills 可以同时出现，但仅表示“在指定 head 后发送一条新的
未持久化用户消息”。retry/edit/regenerate 已有 turn 仍拒绝 requested skills。

后续可新增 typed REST endpoint 作为非 WS fallback 和 SDK contract；不属于本轮 MVP：

```text
Browser URL: POST /api/bots/{bot_id}/web/messages/typed
Echo route may omit /api when the router group/proxy already strips it.
```

如果实现该 endpoint，payload 与 WS typed payload 语义同构；`session_id` 在 body 中可选。
REST 字段名可为 `message`，WS 字段名保持 `text`。它验证 bot/session/head access，
server-side classifier 和 requested skill resolver 先于附件 normalize 与 new session ensure，
typed `requested_skills` 映射到 `conversation.ChatRequest.RequestedSkills`，不经过 local
channel active route 或 metadata。

legacy endpoint：

```text
POST /bots/{bot_id}/web/messages
```

只保留 normal-chat compatibility。它也要 server-side classify：

- slash 返回 `command_result` / `command_error` 或 4xx。
- slash 绝不进入 local inbound / LLM。
- legacy endpoint 不支持 `requested_skills`。
- 如果 payload 携带 `requested_skills` 或 reserved metadata，返回
  `unsupported_legacy_endpoint` / `reserved_skill_metadata`。
- Web runtime 的 typed slash fallback 和普通 REST fallback 必须调用这个 `web` endpoint。
  如果 OpenAPI/SDK 暂时只生成 `/local/messages`，Web runtime 不能误用该生成项；可以用 SDK
  client 的 typed `client.post` 指向 `/bots/{bot_id}/web/messages`。

WS inbound 和 outbound envelope 保持兼容现有结构：

```json
{
  "type": "command_result",
  "invocation_id": "optional",
  "composer_scope": "optional",
  "stream_id": "optional",
  "session_id": "optional",
  "data": {},
  "message": "optional"
}
```

Web parser union、store 分支必须支持 `command_result` 和 `command_error`：

- 进入 command panel。
- 不绑定 assistant stream。
- 不创建 assistant optimistic turn。
- command panel 优先按 `error.code` 使用前端 locale 文案；server `message` 只作为未知 code
  兜底，不能让 zh/ja UI 固定显示英文。

Command panel 的稳定本地事件 payload：

```json
{
  "type": "command_result",
  "invocation_id": "uuid",
  "panel_key": "computed by Web",
  "action_id": "help",
  "terminal": true,
  "result": {
    "kind": "text|list|error",
    "title": "optional",
    "items": [],
    "text": "optional"
  }
}
```

```json
{
  "type": "command_error",
  "invocation_id": "uuid",
  "panel_key": "computed by Web",
  "action_id": "optional",
  "terminal": true,
  "error": {
    "code": "unknown_slash",
    "message": "user safe message"
  }
}
```

Server-origin WS command events may omit `panel_key`; Web computes it from echoed `invocation_id` /
`composer_scope` when present, from the local send attempt context, or from `session_id` for session-scoped
events. Server-side classifier errors for no-session Web sends must echo `invocation_id` and
`composer_scope`; the server must not guess a Web-only tab or composer scope.

OpenAPI / SDK 必须先生成，Web 的 HTTP quick action / safe catalog 使用 SDK DTO，不手写
`any` 影子类型。WS payload 不属于 OpenAPI 生成范围，本轮可保留本地 TS 类型，但字段必须与
后端 DTO/文档同构。

## Web Quick Action / Command Execute

Web `CommandAction` 不走 `chatStore.sendMessage`：

- 不创建 session。
- 不创建 turn。
- 不写 history。
- 不插 optimistic assistant stream。
- MVP 只走 HTTP：`POST /api/bots/{bot_id}/quick-actions/execute`。

HTTP request DTO：

```json
{
  "action_id": "help",
  "params": {},
  "invocation_id": "client-generated uuid",
  "composer_scope": "client tab or draft scope",
  "session_id": "optional"
}
```

Rules:

- `invocation_id` 由 Web 生成，server 原样 echo。
- `composer_scope` 是 Web 本地归属信息，server 可 echo，但不把它当权限边界。
- `session_id` 只有 `requires_session=true` 的 action 才允许。
- `requires_session=false` 的 action 收到非空 `session_id` 时必须返回
  `invalid_quick_action_scope`，且 response 不回显该 session；MVP `help` / `skill.list`
  都是 sessionless。
- request 不包含 raw slash；server 也不接受 raw slash fallback。
- HTTP response 复用 `command_result` / `command_error` 的 result/error 结构；Web 用
  `bot_id + composer_scope + invocation_id` 或 `bot_id + session_id + invocation_id`
  生成本地 `panel_key`。

command package 对外提供：

- known predicate
- safe catalog DTO
- action schema
- structured executor

Web catalog 是 action-level default-deny。schema 必须声明：

- params kind，例如 `Args`、`Page`、`SelectID`、`Range`、`ID`、`PlainArgs`
- `requires_session`
- `requires_write`
- `allowed_surfaces`
- result shape

MVP action schemas：

| action_id | params | requires_session | requires_write | allowed_surfaces | result |
| --- | --- | --- | --- | --- | --- |
| `help` | none | false | false | `web` | `text` / `list` |
| `skill.list` | optional `page` | false | false | `web` | `list` from safe catalog |

`skill.list` 的数据来源必须是 runtime safe catalog，不得调用返回 `raw/content/source_path` 的
管理端 skill list endpoint。

`ExecuteStructured` 只根据 schema dispatch：

- 不把结构化参数拼回 slash 字符串。
- 不 fallback 到 parser。
- 输出只允许 `text` / `list` / `error`。
- 递归剥离 `Interactive`、`ItemAction`、callback、picker 等 channel-only 结构。

Route-aware native commands 的归属：

- `/start`、`/stop`、`/status`、`/context` 等依赖 route/session/active stream 的命令
  不是普通 registry executor。
- MVP 中它们不进入 Web quick-action allowlist，除非单独实现 Web-owned action schema 并显式
  注入所需 route/session 能力。
- Web typed `/stop` 这类 known-but-not-web-executable command 返回 `unsupported_web_command`，
  不进入 LLM。
- 例外：Web 现有 `/new` composer shortcut 是 legacy Web-owned session action，不属于本轮
  quick action MVP；实现可以继续走既有 Web session/composer API，但必须遵守 slash + 附件
  先拒绝、不进 LLM、不写入 history 的控制消息边界。
- Channel surface 继续使用现有 channel command path 处理这些 native command。

授权语义：

- Quick action execute 复用现有 command access / bot access 语义。
- 再叠加 action schema 的 `requires_write`、`requires_session`、`allowed_surfaces`。
- 不能成为第二套绕过 channel / bot 权限的授权模型。

Panel key：

- 无 session command：`botId + composerScope + invocationId`
- session command：`botId + sessionId + invocationId`

## Skill Slash: Web

Web skill 菜单只产生 chip / typed payload，不做自然语言插入。

规则：

- skill-only send 被拒绝；必须有用户 prompt。
- requested skills 通过 WS typed payload 主链路传给后端。
- Web 用户手打 `/skill use name -- prompt` 一律 reject，错误码 `use_skill_chip_required`。
  Web 的 skill-as-slash 只能通过 safe catalog 选择 chip，不能走 name-only 文本解析，也不能
  由客户端把 raw slash 文本持久化。
- unknown slash、slash+attachments、command error 都不清 draft、不读文件、不创建 session。
- no-session requested-skills 的 late failure 清理 created session 时，`deletedSession` 必须使用
  发送时的 `composerScope` 作为 fallback；普通 WS `error` 没有 `command_error` payload，也必须
  能把 promoted panel 原地恢复为 draft。
- safe catalog endpoint 只要求 chat-level 访问权限即可暴露 name/description/ref，因为可聊天用户
  已能通过模型能力间接获知可用 skill 名称；但继续禁止 raw/content/path/hash。
- 安全假设：chat-level 用户可以请求把 runtime-usable skill content 放进本轮模型上下文，
  因此 skill content 不应被当成对聊天用户保密的材料。runtime API 仍然不得把 full content
  直接返回给客户端。

Runtime requested skill API 使用 safe catalog，不复用管理 API 中的
`raw`、`content`、`source_path`。

迁移要求：

- Web composer slash 菜单、quick action panel、session 信息面板、聊天 runtime 中的 used-skills
  展示、以及共享 skill catalog cache 必须停止调用管理端 full skill list。
- 所有非管理/设置页面的 Web runtime consumer 都只能使用 safe catalog 或服务端生成的
  sanitized audit summary。
- 管理端 full skill list 如果继续返回 `raw/content/source_path`，只能用于管理/设置页面，并受
  `manage` 权限保护。
- 现有自然语言插入式 skill 选择路径必须移除或替换；选择 skill 只能生成 chip / typed payload。

Safe catalog DTO：

```json
{
  "skill_ref": "v1.kid.nonce.ciphertext",
  "name": "skill-name",
  "display_name": "Skill Name",
  "description": "short safe description",
  "source_kind": "managed|legacy|compat|plugin or mapped display enum",
  "state": "effective"
}
```

Safe catalog 只返回当前 Bot runtime-usable/effective skills；disabled、shadowed、ambiguous、
not runtime usable entries 不返回给 Web composer。

Request DTO：

```json
{
  "skill_ref": "v1.kid.nonce.ciphertext",
  "name": "skill-name"
}
```

Request DTO 是 strict typed shape；客户端不得提交 `source`、`source_path`、`raw`、`content`、
`hash`、`audit` 等展示或审计字段。若边界层无法 strict reject unknown fields，也必须在
resolver 前丢弃，且不得写入 audit/display metadata。审计 source kind、effective state、
opaque ref 摘要只能由 resolver 从服务端 catalog 生成。
resolved context 使用解析后的 skill body/content，不使用 raw `SKILL.md` 文件、frontmatter
metadata、source path 或 raw hash 作为模型上下文。

Runtime-usable 判定：

- 单一权威来源是 skill registry 归一化后的 `RuntimeUsable` / visibility flags 或等价内部字段；
  safe catalog、Web resolver、text resolver 只能读取该结构，不能各自解析 raw frontmatter。
- 当前 Bot effective catalog 中唯一可解析。
- source kind 为 `managed`、`legacy`、`compat`、`plugin` 中可在当前 Bot runtime 加载的来源。
- plugin source 必须已安装且启用。
- parsed skill body/content 非空。
- 未 disabled、hidden、internal-only、subagent-only。
- 未 shadowed 或同名多来源造成歧义。
- ACP/external agent 侧 skill 不属于 Memoh requested skill runtime catalog，MVP 一律拒绝。
- hidden/internal-only/subagent-only 的权威标记来自 registry parser 支持的 manifest/frontmatter
  字段或 plugin manifest 字段，并在 `skills.Entry` 中归一化。字段 malformed、类型不对、
  值无法判定、或 plugin manifest 与 parsed skill body 冲突时，MVP fail-closed：
  safe catalog 不返回，resolver 返回 `requested_skill_not_runtime_usable`。

## Skill Slash: Text Channel

文本 channel 没有安全的 `skill_ref` chip，因此不能要求用户粘贴 opaque ref，也不能接受
path/hash/source。

MVP 文本语法：

```text
/skill use <skill-name>[,<skill-name>...] -- <prompt>
/skill@Bot use <skill-name>[,<skill-name>...] -- <prompt>
```

语法规则：

- `--` 是必需 delimiter。
- `<prompt>` trim 后必须非空。
- `<skill-name>` 使用 catalog canonical name，不使用 display name。
- skill name 校验必须复用 catalog canonical normalization / name validator；`[A-Za-z0-9_.-]+`
  只是 MVP 的外层字符集上限，仍要拒绝空段、`.`、`..`、非法 normalized name。
- MVP 不做 case-fold 或 Unicode normalization 迁移；匹配是大小写敏感的 exact canonical name。
  `Foo` 和 `foo` 是不同 skill。
- 多个 name 用英文逗号分隔。
- 重复 name 按出现顺序 deterministic de-dupe。
- 不接受 path、source、hash、content、`skill_ref`。
- 超过最大 skill 数返回 `too_many_requested_skills`。

Stage 1：channel identity / effectiveDirected 检测后、attachment ingest 前调用 classifier。

- undirected group slash：hard no-op。
- command slash：进入 channel command handler。
- directed `/skill use`：只记录 pending skill intent，不执行、不 persist、不进 pipeline。
- 如果带附件或引用带附件的消息：立即 reject，且不 ingest。

Stage 2 是 pre-session preflight：在 route candidate、trigger/ACL/context 判断可用后处理
pending intent，但必须早于 `EnsureActiveSession`、DCP `PersistEvent`、passive persist、
pipeline `AdaptInbound`、active-stream inject。

ACL denied / no-trigger 特殊规则：

- 如果 pending intent 是 `/skill use`，即使 ACL denied 或 no-trigger，也必须在 passive persist
  前丢弃或返回安全错误，并且不能创建/选择 session。
- 原始 slash 不进入 passive history、memory、title。
- 对 undirected group 仍保持 no-op；对 directed denied 可按现有 channel policy 返回
  `permission_denied`，但不持久化原文。

Stage 2 成功条件：

1. 调用 `SkillCatalogAuthority.ResolveTextRequestedSkills(botID, names)`。
2. 服务端重新计算当前 bot effective catalog。
3. 先在完整 normalized catalog 中按 canonical name 找 candidate，用于区分 missing、
   disabled、shadowed/ambiguous、not-runtime-usable。
4. 只有 candidate 唯一且 runtime-usable 时，才算成功匹配；disabled 返回
   `requested_skill_disabled`，shadowed/同名多 source 返回 `requested_skill_ambiguous`，
   not-runtime-usable 返回 `requested_skill_not_runtime_usable`。
5. 服务端现场 mint 当前 `skill_ref`，并走与 Web requested skill 相同的 resolver 校验链。
6. 校验 requested skill count 和 context size limits。
7. 确认这是普通 chat route，且不是 DCP/discuss pre-persist、already-in-context，或已存在
   active stream 时的 inject/continuation。`ChatRequest.InjectCh != nil` 不能作为拒绝判据：
   普通 IM channel 新 stream 也会带 dispatcher inject channel，用于后续 `/btw` 注入。

只有上述全部成功后，才允许进入 session ensure / selection。session 确定后：

1. 在任何 history/passive persist 前，把 local visible text 改为 `<prompt>`。
2. 构造 `ChatRequest{PersistQuery/UserVisibleText: prompt, RequestedSkills: refs}`。
3. 原始 slash 只可作为非用户可见、非 memory/title 输入的 debug trace；默认不保留。

稳定错误码：

- `invalid_skill_slash_syntax`
- `missing_prompt`
- `requested_skill_not_found`
- `requested_skill_ambiguous`
- `requested_skill_disabled`
- `requested_skill_not_runtime_usable`
- `invalid_skill_ref`
- `stale_skill_ref`
- `too_many_requested_skills`
- `slash_attachments_unsupported`
- `unsupported_skill_slash_context`
- `permission_denied`
- `invalid_quick_action_scope`

MVP 仅支持非 discuss、非 active-stream、非 inject、非 pre-persist 的普通 chat route。
如果当前路径需要 DCP/discuss pre-persist、已经 `userMessageAlreadyInContext`、active stream
inject、或者无法保证 slash 不被预持久化，则 reject。

## SkillRef / Resolver

唯一可信入口是 `skills.RefResolver` / `SkillCatalogAuthority`：

- Web request：先解密/校验 `skill_ref`，再重新计算当前 bot effective catalog，并要求
  `name + decrypted payload` 在完整 normalized catalog 中定位 candidate。先按 identity
  判断 missing/stale，再按当前状态映射 disabled、shadowed/ambiguous、not-runtime-usable；
  只有最终 candidate 唯一且 runtime-usable 时才成功。
- Text channel request：先按 canonical name 唯一匹配，再由服务端 mint `skill_ref`，
  最后走同一 resolver 校验链。

`skills.Entry` 内部扩展：

- `SourceKind`
- `EffectiveState`
- `OpaqueSourceID`
- `RawContentHash` / `ContentHash`
- `CatalogScope`

这些字段只在服务端使用。safe catalog 只输出 token 和安全展示信息。

SkillRef v1 是 authenticated encrypted opaque token，不是 MAC-only token：

```text
v1.<kid>.<nonce_b64url>.<ciphertext_b64url>
```

MVP 使用 AEAD 或等价的 authenticated encryption。允许的等价替代是服务端持久化 opaque
handle，但必须同样跨重启、多实例可用，并能区分 invalid 与 stale。禁止使用只有 MAC、
没有可恢复 payload 或服务端 lookup handle 的 token 设计，因为那无法稳定实现
`invalid_skill_ref` / `stale_skill_ref` 的错误码契约。

加密 payload 字段：

- version
- bot_id
- catalog_scope
- normalized skill name
- source_kind
- opaque_source_id
- raw_content_hash

Canonical encoding：

- payload canonical encoding 使用 UTF-8，并在加密前确定化。
- UUID / bot id 使用 lower-case canonical string。
- skill name 使用 skill registry 的 canonical normalized name。
- 使用 length-prefixed field buffer，避免分隔符歧义。
- AEAD associated data 至少包含 `version`、`kid`、token purpose label，例如
  `skill-ref:v1`。
- nonce 必须满足所选 AEAD 的唯一性要求；MVP 推荐由服务端 CSPRNG 生成，不参与客户端输入。

错误码判定：

- token 格式错误、unknown/expired kid、decrypt/auth failure：`invalid_skill_ref`。
- decrypt 成功但 `bot_id` 不等于当前 bot：`invalid_skill_ref`。
- decrypt 成功且 bot 匹配，但当前 effective catalog 中对应 name/source/content hash
  不再匹配：`stale_skill_ref`。
- decrypt 成功且 catalog 匹配，但当前 entry disabled：`requested_skill_disabled`。
- decrypt 成功且 catalog 匹配，但 shadowed/ambiguous/not runtime usable：
  `requested_skill_ambiguous` 或 `requested_skill_not_runtime_usable`。

`opaque_source_id` 由服务端 keyring 对内部 source tuple 派生，例如 source kind、source root、
source path、installation id。MVP 使用 SkillRef keyring 的同一组 keyed material，但必须带
独立 domain separation label，例如 `opaque-source-id:v1`，不能直接复用 SkillRef token
payload 或 ciphertext。
客户端不可见。

SkillRef key lifecycle：

- keyring 来自 server config / secret store，例如 `skill_ref_keys` 和 `current_kid`。
- key 必须跨重启稳定，多实例一致。
- 不能复用 JWT secret、`app.local.toml` 中的公开 dev secret，或任何已知默认值。
- deploy/server 模式缺少专用 keyring 时启动 fail-fast；不允许临时生成内存 secret，也不允许
  静默降级为可伪造 token。
- desktop/local 首次启动必须生成专用随机 keyring 并持久化到 local app data config；后续重启复用。
- 支持 current + previous kids。
- old kid grace window 由 keyring 配置控制；token 本身不需要 exp。
- `opaque_source_id` 派生使用同一 keyring 和 kid 生命周期。校验旧 token 时按 token kid 使用
  对应 key 派生 source id；old kid 过期后，对应 ref 和 opaque source binding 一起失效。

Reject 条件：

- tamper
- wrong bot
- stale content
- disabled
- shadowed
- same name different source without explicit unique match
- ACP-sourced 或不可用于 runtime
- old kid expired

所有 requested skills required；任意一个失败，整次请求失败。

Available skills 仍是完整 effective catalog。requested skills 只是强制加载进本轮模型上下文，
不替代 `use_skill` tool。

## Requested Skill Limits

Limits 必须 fail-closed：

- max requested skills count：配置项，MVP 默认 5。
- max single skill context bytes：配置项，MVP 默认 64 KiB。
- max total requested skill context bytes：配置项，MVP 默认 256 KiB。
- deterministic ordering：按用户选择顺序，去重后保持顺序。

如果任一 skill 或总 context 超限：

- 在 model call 前整体 reject。
- 返回稳定错误码 `requested_skill_context_too_large`。
- 不截断后继续。
- 不写 user message。
- 不写 `applied_skills` / `model_context_skills` audit。

这与“所有 requested skills required”的语义一致。

## ChatRequest / Query Split

不变量：skill full content 永远不拼入 `Query`，也不进入持久化 user message。

建议字段：

- `Query`：兼容字段，作为用户可见文本 alias；不含 skill content。
- `RawQuery`：如果保留，只能用于受控 debug/telemetry；成功转换的 `/skill use` 原始 slash
  不得作为 display、title、memory、dedupe 或 persist 输入。
- `PersistQuery`：写 history/user content、attachments association、dedupe/prependUserMessage、
  title、memory extraction 的默认来源。
- `UserVisibleText` / `DisplayText`：UI 展示，默认等于 `PersistQuery`。
- `ModelUserText`：用户 prompt 经 mode/header/hook 后给模型看的可见文本，不含 skill full content。
- `RequestedSkills`：用户请求的 refs。
- `RequestedSkillContext` / `ModelUserContext`：resolver 加载的 full skill content，只进入模型消息
  assembly。

替换表：

| 用途 | 字段 |
| --- | --- |
| store user message content | `PersistQuery` |
| display_text | `UserVisibleText` |
| title generation | `PersistQuery` |
| memory extraction/update | `PersistQuery` |
| dedupe / prependUserMessage | `PersistQuery` |
| attachment metadata association | `PersistQuery` / `UserVisibleText` |
| model message assembly | `ModelUserText` + delimited `RequestedSkillContext` |

`RunConfig` 不再用一个 ambiguous `Query` 承载所有含义。引入
`RunConfig.UserVisibleQuery`、`RunConfig.ModelUserText`、`RunConfig.RequestedSkillContext`
或等价明确结构。

Hooks：

- 只能改 visible prompt。
- hook 后再次运行 reserved metadata denylist。
- hooks 默认拿不到 skill full content，除非以后新增显式 model-context hook。
- message metadata、content-part metadata、attachment metadata、reply attachment metadata 以及
  flow `ChatRequest.Attachments/ReplyAttachments` 都必须检查 reserved key；发现后 fail-closed，
  不能 persist、ingest 到 history，或进入 LLM。

Requested skill block：

- 放在当前用户消息邻近位置。
- 明确标记为 untrusted user-requested context。
- system/developer/security/session mode 优先级更高。
- 使用 delimiter。
- 受 requested skill limits 约束。

## Guards / Metadata / Audit

`validateRequestedSkillsAllowed` 必须在所有 public entry 最前调用，早于 ACP/agent/pipeline 分流：

- `Chat`
- `StreamChat`
- `StreamChatWS`
- typed REST
- WS
- channel-converted `/skill use`
- retry/rewrite
- tool approval
- user input
- schedule/heartbeat

拒绝 requested skills 的路径：

- ACP
- retry/edit
- tool approval / user input continuation
- schedule/heartbeat
- `UserMessagePersisted=true`
- pipeline already-in-context
- DCP/discuss pre-persist
- active stream inject
- 任何 pre-persisted path

这些拒绝条件必须独立检查；`UserMessagePersisted=true` 只是其中一个信号，不能覆盖
schedule/heartbeat、active-stream inject、DCP/discuss、continuation 等路径。

单一共享函数：

```go
RejectReservedSkillMetadata(map[string]any) error
```

调用点：

- HTTP boundary
- WS boundary
- channel boundary
- legacy metadata entry
- attachment/content-part metadata entry，如果存在用户可控 metadata map
- hook 后
- resolver entry
- persist 前

Reserved keys 使用规则式匹配：对 key 做 lower-case，并移除 `_`、`-`、`.` 后再比对，覆盖
snake_case、camelCase、kebab-case 和大小写变体。至少包括：

- `requested_skills`
- `requestedSkills`
- `applied_skills`
- `model_used_skills`
- `model_context_skills`
- `loaded_skills`

外部输入统一返回稳定 machine code `reserved_skill_metadata`；HTTP 可用 400/422 状态码，
Web/channel 用户可见错误必须走该 code 的 i18n。内部 hook 返回 typed error，并在 public
boundary 映射为 `reserved_skill_metadata`，不能退化成泛化 400。

所有用户可见错误码必须补 en/zh/ja i18n 文案，并走现有 channel/Web 本地化路径；测试需要覆盖
至少一个 channel directed 错误和一个 Web composer 错误的本地化输出。

Skill audit metadata 只能由 resolver/store 写：

- requested/applied 写 original user message。
- model_context/model_used 写 terminal assistant 或显式 audit hook。
- 不包含 full content、source_path、raw hash。

`storeRound` 新增显式：

- `UserMessageMetadata`
- `AssistantTerminalMetadata`

Skill audit 禁止使用 `MessageMetadataByIndex`。旧 index metadata 可继续用于无关 legacy 路径，
但不能承载 skill audit。

PG/SQLite session info 改为内部安全去重聚合，不在本轮改变 public API 形状。
内部解析时保留来源与稳定 identity 概念：

```json
{
  "key": "requested|applied|model_context|tool_call",
  "identity": "source|normalized-name|source-kind|opaque-source-id",
  "name": "skill-name",
  "source": "requested|use_skill",
  "skill_ref": "optional"
}
```

来源：

- user metadata requested/applied/model_context
- assistant `use_skill` tool calls

当前 `SessionInfoResponse.skills` 继续输出 `[]string` 的安全 skill name，handler 不返回
source、path、raw hash、opaque source id 或 skill_ref。聚合层按服务端 resolver 产生的稳定内部
`identity` 去重，例如 source + normalized name + source_kind + opaque_source_id。MVP 不持久化
content hash；如果后续需要 content-version 去重，必须先设计不可公开的 keyed
`content_version_id`。不要按 `skill_ref` 去重；AEAD token 使用 nonce，同一 skill version
多次 mint 可能得到不同 token。历史 `use_skill` tool call 只有 name 时，
`identity` 可降级为 normalized name，并只用于 legacy 展示去重。handler 可降级输出 `[]string`。

## Tests / Acceptance

Classifier：

- direct/group
- directed/undirected
- `effectiveDirected` suffix mention
- `supportsMode=false`
- mode+slash reject
- `/now`、`/btw`、`/next` mode prefix
- `/skill@Bot use`
- `/help@Bot`
- unknown slash
- slash+attachments

Web：

- `handleSend` 分类在清 draft/FileReader/session 前。
- slash-shaped text 或 requested skills 的 server preflight 在 new session ensure 前；失败不留下
  空 session、turn、assistant stream 或 history。
- command no session/no turn。
- unknown slash 不清 draft。
- unknown slash / unsupported command 经 WS fallback 返回时，带 `invocation_id` / `composer_scope`
  并进入当前 command panel。
- requested skill chip 通过 WS `text + requested_skills` typed payload 发送。
- 手打 `/skill use name -- prompt` 返回 `use_skill_chip_required`，不清 draft。
- stale/malicious Web typed `/skill use name -- prompt` 由 server classifier 返回
  `use_skill_chip_required`，不会进入 channel `ContinueChatIntent`。
- existing natural-language skill insertion path removed/replaced by chip payload.
- Web composer and all non-management runtime skill consumers use safe catalog, not management full skill list.
- skill chip + prompt + ordinary attachments keeps normal chat attachment behavior and is not rejected as a
  slash control message.
- skill-only send returns `missing_prompt`.
- tampered/stale/disabled/not-runtime-usable/oversize requested skill preflight failures keep draft/chip/
  attachments and do not create session/turn/history/audit applied metadata.
- `help` and `skill.list` quick actions execute through structured HTTP without raw slash.
- quick-action HTTP request 带 `invocation_id` / `composer_scope`，Web 本地计算 `panel_key`。

REST：

- legacy slash 不进入 local inbound。
- legacy `requested_skills` reject。
- legacy requested skills returns `unsupported_legacy_endpoint`.
- legacy reserved metadata returns `reserved_skill_metadata`.
- quick-action execute 与 safe skill catalog 使用 OpenAPI/SDK DTO。
- typed REST chat fallback is out of MVP; if added later, it must perform the same preflight before new
  session ensure and map its `message` field to WS/internal `text` semantics.

WS：

- client outbound 支持 `text + requested_skills`。
- client outbound supports `invocation_id` / `composer_scope` for slash classification errors.
- server guard 在附件 normalize 前运行。
- server preflight runs before new session ensure for no-session composer.
- `command_result` / `command_error` parser/store 进入 command panel。
- command event 不建 assistant stream。

Channel：

- undirected group slash hard no-op。
- suffix mention slash 不被 no-op 吞掉。
- `/skill use` grammar 成功。
- missing delimiter returns `invalid_skill_slash_syntax`; empty prompt returns `missing_prompt`.
- ambiguous / missing / disabled skill reject。
- non-runtime-usable skill reject.
- ACL denied pending skill slash 不 passive persist、不创建 session。
- DCP/discuss/pre-persist reject；已存在 active stream 时的 inject/continuation reject。
- ACP-backed session reject.
- 原始 slash 不进 history/memory/title。

SkillRef：

- tamper
- wrong bot
- stale content
- disabled
- shadowed
- same name diff source
- old kid grace / expired
- multi-instance stable
- text channel server-minted ref goes through same resolver

Query split：

- skill content 不进 storage。
- skill content 不进 title。
- skill content 不进 memory。
- skill content 不进 dedupe/display。
- skill content 只进 model assembly。
- oversize fail-closed 不写 audit。

Metadata：

- boundary reject returns stable `reserved_skill_metadata`。
- post-hook reject。
- resolver entry reject。
- explicit store metadata。
- PG/SQLite session info parity。

## Review Log

### V2.8 Review 采纳项

- Channel 文本 `/skill use` 没有 `skill_ref`，必须定义文本语法，并由服务端在 Stage 2
  重新计算 effective catalog、唯一匹配、现场 mint ref。
- requested skill context 超限必须 fail-closed，不能截断后继续。
- Web 主发送路径是 WS，必须扩展 WS client outbound payload 支持 `requested_skills`；
  typed REST 只能作为 fallback / SDK contract。
- route-aware native commands 不能默认进入 Web structured executor；MVP 对 Web 返回
  `unsupported_web_command`，除非单独实现 Web-owned action schema。
- group slash 必须先计算 `effectiveDirected`，再决定 undirected no-op。
- ACL denied / no-trigger 的 pending `/skill use` 也不能进入 passive persist。

### V2.8 Review 不采纳项

- 不因当前代码还没实现本方案而判失败。
- 不把 Slack/Discord/Feishu native slash command 注册纳入 MVP。
- 不把 legacy channel callback / `SyntheticCommand` 强行迁移到 Web structured executor。
- 不暴露 skill path/raw/content 来解决文本 channel disambiguation。
- 不让 unknown slash 回落成普通聊天。
- 不要求 requested skills 替代现有 `use_skill` tool。

### V2.9 Review 采纳项

- WS 主发送链路沿用现有 `text` 字段；typed REST 的 `message` 只做 handler 映射。
- Channel `/skill use` 的 reject/no-trigger/permission denied 必须发生在 session ensure 前，
  不能为了失败请求创建 session。
- 无 session command panel 的归属由 Web 通过 `composer_scope + invocation_id` 本地计算；
  server 不猜 tab id。
- Web 手打 `/skill use name -- prompt` 一律 reject；Web skill slash 只能来自 safe catalog chip
  和 typed `skill_ref`。
- 补充 parsed skill context、`RawQuery` 不得参与 display/title/memory、skill name 复用 catalog
  validator、以及 chat-level skill context 安全假设。

### V2.9 Review 不采纳项

- 不把当前早期 Web-only draft 当成方案失败。
- 不把 startup failure 下 chip 恢复体验作为阻塞架构问题；实现时应与 draft/attachments 一起恢复。
- 不要求 MVP 支持文本 channel 粘贴 `skill_ref` 或 source/path/hash 消歧。

### V2.10 Follow-up Review 采纳项

- 补充“现状与目标态差异”，明确 command.Handler FX singleton、supportsMode、RawQuery、
  Web WS slash guard、session info SQL 等都是改造项。
- 定义 runtime-usable skill，并明确 ACP/external agent 侧 skill 不属于 requested skill runtime catalog。
- 明确 Web composer/runtime 必须迁移到 safe catalog，管理端 full skill list 不能用于聊天 composer。
- 明确现有自然语言插入式 skill 选择路径必须移除或替换。
- desktop/local 使用专用 SkillRef key，首启生成并持久化；deploy/server 缺 keyring fail-fast。
- 明确 canonical name MVP 使用大小写敏感 exact match，不做 case-fold 或 Unicode normalization 迁移。
- 对齐 `missing_prompt`、`skill.list`、`unsupported_legacy_endpoint`、`/next` mode prefix、
  ACP-backed session reject、limits 默认值、PG/SQLite session info 同步改造。
- 明确 adapter 需要在分类前提供引用/转发附件信号，错误优先级和用户可见错误 i18n 要求。

### V2.10 Follow-up Review 不采纳项

- 不把平台原生 slash command 注册纳入 MVP。
- 不把 legacy channel callback / `SyntheticCommand` 强制迁到 Web structured executor。
- 不通过暴露 path/raw/content/hash 来解决 skill 歧义。

### V2.11 Follow-up Review 采纳项

- 明确 Web surface 的 `/skill use ... -- ...` 在 server classifier 中返回
  `use_skill_chip_required`，只有 channel surface 可产生 `ContinueChatIntent`。
- 明确 slash-shaped text / requested skills 的 Web server preflight 必须早于 new session ensure，
  failed preflight 不留下空 session。
- WS typed payload 增加 `invocation_id` / `composer_scope` correlation，server-side classifier
  errors 必须可归属 command panel。
- 扩大 safe catalog 迁移范围到所有非管理/设置页面的 Web runtime consumer、session 信息面板和共享 cache。
- 移除 Request DTO 中客户端 `source` 字段；审计 source/display summary 只能由 resolver 生成。
- `opaque_source_id` 使用 SkillRef keyring 的 domain-separated 派生，并共享 kid / rotation lifecycle。
- runtime-usable visibility flags 由 registry 统一归一化；malformed 或无法判定时 fail-closed。
- `reserved_skill_metadata` 在方案中落为稳定 machine code，并补 Web/REST/WS/metadata 验收。
- 补充 Web/WS 验收：chip+附件正常聊天、skill-only `missing_prompt`、preflight 失败保留 draft/chip/附件。

### V2.11 Follow-up Review 不采纳项

- 不保留当前 Web 先建 session 再发送的限制；目标态要求 failed requested skill preflight 不创建新 session。

### V2.12 Follow-up Review 采纳项

- 将 SkillRef v1 从 MAC-only token 改为 authenticated encrypted opaque token：
  `v1.<kid>.<nonce_b64url>.<ciphertext_b64url>`。
- 明确 MAC-only 不能满足 `invalid_skill_ref` / `stale_skill_ref` 的错误码契约；若不用 AEAD，
  必须使用等价的服务端持久化 opaque handle。
- 补充 encrypted payload 字段、AEAD associated data、invalid/stale/disabled/ambiguous/not-runtime
  的错误优先级。
- 补充“简化后的交付切片”，把实现收敛为共同底座、Web quick action、Web skill chip、
  Channel skill slash、审计展示收尾五段，但不降低安全和行为需求。

### V2.12 Follow-up Review 不采纳项

- 不合并 `invalid_skill_ref` 与 `stale_skill_ref`；需求保留两个稳定错误码，因此 token
  设计必须支持区分。

### V2.13 Final Review 采纳项

- 明确 resolver 先在完整 normalized catalog 中定位 candidate，再映射 stale、disabled、
  ambiguous、not-runtime-usable；不能先过滤 runtime-usable 后误判错误码。
- session used skills 去重改为服务端稳定内部 identity，例如 source + normalized name +
  source_kind + opaque_source_id；MVP 不持久化 content hash，不再按 AEAD `skill_ref` token 去重。
- 补齐当前 V2.13 版本 Review Log，保持版本号和 review 记录一致。

### V2.13 Final Review 不采纳项

- 不恢复 deterministic MAC token 或按 token 字符串去重；AEAD nonce 允许同一 skill/version
  多次 mint 出不同 token。

### V2.14 Implementation Review 采纳项

- Web slash + 附件在附件读取/normalize/ingest 之前 fail-closed，并进入 command_error panel。
- requested skills 增加 session/spec guard，只允许普通 model chat；channel 自动 discuss/ACP
  session 和 Web 既有 discuss/ACP session 都返回 `unsupported_skill_slash_context`。
- `skill_ref` provider 显式拒绝公开占位符；install upgrade 对旧 config 追加或替换 keyring。
- legacy local message DTO 不再在 OpenAPI/SDK 暴露 `requested_skills`，但 raw JSON key 仍被拒绝。
- PG/SQLite `GetSessionUsedSkills` 的 assistant `use_skill` tool-call 解析形态对齐。
- `model_requested_skills` 纳入 reserved metadata denylist，legacy metadata 不能伪造 used-skills audit。
- flow 层 requested skills guard 同时覆盖 `SessionType`、pre-persisted user message、active inject、
  ACP/discuss，以及直接调用 `resolve()` 的内部路径。
- Web WS requested-skills 增加 handler 级 bot/session preflight reservation，关闭跨 WebSocket
  连接并发 resolve refs 的竞态窗口。
- no-session Web WS late failure 清理 created session 后，command error 降级到 composer-scope-only
  panel；用户已切换 session 时不恢复旧 prompt/chip/附件到当前 composer。
- tool approval、user input continuation 和普通 Web WS message stream 都进入同一 handler 级
  active registry，普通 message stream 在 requested-skills resolve refs 前完成 active 标记。
- Web command panel event 改为按 session/composer scope 存储，off-scope late event 不覆盖当前
  composer 的 command result/error。
- requested-skills MVP audit identity 不持久化 content hash；如需版本级去重，后续另设不可公开的
  keyed `content_version_id`。

### V2.14 Implementation Review 不采纳项

- 不把现有 Web `/new` composer shortcut 迁入本轮 quick action executor；它作为 legacy Web-owned
  session action 保留，但遵守 slash + 附件拒绝边界。
- 不在本轮把 `SessionInfoResponse.skills` 从 `[]string` 扩展为结构化 API；持久化 metadata 保留
  source/identity 信息，当前展示层继续输出去重后的安全名称。

### V2.15 Implementation Review 采纳项

- flow requested-skills guard 不再把 `InjectCh != nil` 视为 unsupported context；普通 IM channel
  新 stream 需要携带 dispatcher inject channel，同时仍由 channel/WS active-stream guard 拒绝真正
  的 active inject/continuation。
- Channel `/skill use` 增加非 local channel + dispatcher 的正向测试，确认成功进入 `StreamChat`
  且 `RequestedSkills` 与 `InjectCh` 同时存在。
- Web no-session requested-skills 失败清理 created session 时，必须发出既有 `deletedSession`
  信号，让 workspace tabs 关闭/重置已删除 session pane，避免 command panel scope 丢失。
- deferred draft 的普通 WS startup error 如果到达时用户已切到其他 bot/session，不再写入
  `startupSendFailure`；active pane 发现 failure scope 不匹配时也会消费掉该 failure，避免之后
  污染 draft storage。

### V2.15 Implementation Review 不采纳项

- 不新增 `InjectedIntoActiveStream` 字段；当前 active-stream inject 的拒绝点已经在 channel/WS
  边界，移除错误的 `InjectCh` 判据即可满足需求并减少重复状态。

### V2.16 Implementation Review 采纳项

- Web failed deferred-session cleanup 的 `deletedSession` 信号携带 `composer_scope`；workspace tabs
  匹配到原 promoted chat panel 时原地 reset 为 draft，保留 panel id / composer scope，确保
  command/error panel 可见。普通手动删除 session 仍关闭对应 chat panel。
- Channel tool approval / user input continuation streaming 期间，非 local channel 通过
  `RouteDispatcher.MarkActive/MarkDone` 标记 route active；期间新的 `/skill use` 走既有
  `unsupported_skill_slash_context` active-stream 拒绝。
- `skill_ref` decode 在 AEAD `Open` 前显式校验 nonce 长度；错误 nonce 长度返回
  `invalid_skill_ref`，不能 panic。
- Web WS existing-session requested-skills 先完成 session authorization，再检查 active stream /
  reserve requested-skill turn，避免无权 session 因 active 状态暴露不同错误优先级。

### V2.16 Implementation Review 不采纳项

- 不为 failed deferred-session cleanup 新增全局 workspace command-event bus；复用 composer scope
  并保留原 panel id 足以满足可见性和隔离要求。

### V2.17 Implementation Review 采纳项

- Web failed deferred-session cleanup 不能只依赖后端 `command_error` 携带的 `composer_scope`；
  普通 WS `error` 也会失败清理 created session，因此必须使用发送时的 composer scope 作为 fallback。
- `RouteDispatcher` active 状态改为 owner-counted；普通主 stream 与 continuation 重叠时，先结束的
  continuation 只释放自己的 owner，不得清空 active 状态、queue 或 pending persist。

### V2.17 Implementation Review 不采纳项

- 不把 continuation 改成完全绕过 dispatcher active 标记；它仍然需要让期间的新 `/skill use`
  按 active-stream context 拒绝。

### V2.18 Implementation Review 采纳项

- Web WS `message` 在 existing-session 场景下必须先校验 `stream_id` 和 session authorization，
  再运行 slash classifier；`/help`、`/skill list` 等 command action 不能绕过无权 session。
- `/skill use` 的 `--` delimiter 必须是带空白边界的 token；canonical skill name 内部的 `--`
  不能被误切成 selector/prompt 分隔符。
- 管理端 full skill list 如果返回 `raw/content/source_path`，必须要求 `manage` 权限；runtime
  safe catalog 继续只暴露安全字段。
- reserved metadata guard 覆盖 message/content-part/attachment/reply attachment metadata，并在
  Web REST、Web WS、channel inbound 和 flow `ChatRequest` 入口 fail-closed。
- runtime usability 写入 skill `Entry` 归一化字段；safe catalog、Web resolver、text resolver 只
  消费归一化字段，malformed metadata 仍 fail-closed。

### V2.18 Implementation Review 不采纳项

- 不禁止 canonical skill name 中出现双连字符；既有 name validator 允许 `-`，因此 parser 必须
  正确识别 delimiter，而不是收紧 skill 命名规则。

### V2.19 Implementation Review 采纳项

- HTTP quick action executor 按 action schema 校验 request scope；sessionless action 收到
  `session_id` 返回 `invalid_quick_action_scope`，并避免把任意客户端提供的 session id echo 成
  权威归属。
- Web typed slash 调用 MVP sessionless quick action 时不再传当前 selected session id；结果仍由
  Web 根据本地 composer/session UI context 归属到 command panel。

### V2.19 Implementation Review 不采纳项

- 不把 `help` / `skill.list` 提升为 session-scoped action 来绕过 scope 校验；后续 session action
  必须显式声明 `requires_session=true`、权限和结果归属。

### V2.20 Implementation Review 采纳项

- Directed/private channel 的 fixed slash command 在 command access 拒绝时返回
  `permission_denied`，不再静默返回；undirected group slash 仍是唯一 hard no-op。
- Flow reserved metadata guard 覆盖 `ChatRequest.Messages` 的 content-part metadata，补齐
  Web REST/WS/channel 入口之外的 flow 边界。
- Web typed slash fallback 和普通 REST fallback 改用实际注册的 `/bots/{bot_id}/web/messages`
  endpoint；不再使用 SDK 当前生成的 `/local/messages`。
- Web command panel 按 `error.code` 做 en/zh/ja 本地化，server message 只做未知 code 兜底。

### V2.20 Implementation Review 不采纳项

- 不要求 server 为 Web command error 直接返回 zh/ja 文案；server 缺少可靠 Web locale 上下文，
  前端按稳定 code 本地化更符合现有 Web i18n 架构。

### V2.21 Implementation Review 采纳项

- Skill parser 在 frontmatter 或 `metadata` 字段 malformed 时写入 registry-authoritative
  runtime-unusable 状态；管理端仍可看到该 skill 方便修复，但 safe catalog、SkillRef resolver 和
  text resolver 都必须按 `metadata` 原因 fail-closed。

### V2.22 Implementation Review 采纳项

- Web quick action executor 和 typed slash `/web/messages` fallback 的 transport/schema 异常在
  store 层转换为 scoped `command_error` 和 startup failure；chat-pane 继续走现有 restore path，
  保留 draft、附件与 requested skill chips。

### V2.23 Implementation Review 采纳项

- Web outbound WS `requested_skills` 使用独立 request DTO，只发送 `{ skill_ref, name }`；
  safe catalog 的展示字段只停留在 composer chip/UI 状态，不进入后端 request 或 audit 输入面。
