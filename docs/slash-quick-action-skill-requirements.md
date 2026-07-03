# Slash Quick Action 与 Skill Slash 需求文档

当前版本：V1.16
更新时间：2026-07-02

## 文档定位

本文描述产品需求：用户需要什么、哪些场景必须支持、哪些行为必须被拒绝、验收标准是什么。

配套实现方案见 `docs/slash-quick-action-skill-plan.md`。当两份文档侧重点不同：

- 本文回答“要做什么”和“做到什么程度算完成”。
- 实现方案回答“怎么做、放在哪些边界、哪些代码路径需要保护”。

## 原始需求来源

Notion 原始任务：

- `im的slash支持引用skill`：
  <https://app.notion.com/p/im-slash-skill-38fd366426d180fc8326fd0218ca3303>
- `webui支持slash/quick action`：
  <https://app.notion.com/p/webui-slash-quick-action-38fd366426d1808ebb0ef6c3d2a2f270>

截至 2026-07-02，Notion fetch 可读取到的信息如下：

- 两个页面都是 `Project Memoh` kanban 任务卡，状态为 `未开始`。
- `im的slash支持引用skill` 页面正文只有项目壳和外部对象占位；没有可读取评论。
- `webui支持slash/quick action` 页面正文包含任务标题、一个截图和外部对象占位；没有可读取评论。
  截图仅作为 `/` slash command 交互参考，不作为额外规范性需求来源。
- external object / PDF 占位未通过当前 Notion fetch 展开。

因此本文把两个 Notion 任务标题作为原始需求锚点，并结合本轮对话中已明确的需求讨论进行结构化。
如果后续能访问 external object / PDF 的完整内容，需要再次校对本文并重新进行 3-agent review。

需求追溯：

| Source | Covered by |
| --- | --- |
| `webui支持slash/quick action` | Web slash discovery、Web quick action、command panel、Web 验收 |
| `im的slash支持引用skill` | Channel skill slash、directed/undirected channel 行为、安全与审计、Channel 验收 |
| 本轮对话中明确的后端触发与安全边界 | safe catalog、opaque ref、metadata denylist、fail-closed、persistence/audit 验收 |

## 背景

Memoh 当前同时存在 Web UI 和多种外部 channel。用户希望用 slash 触发快捷操作，而不是把
所有操作都写成自然语言让模型猜。与此同时，skill 是动态能力，也应该能像 slash 一样被快速
选择和触发。

这个需求不能只做前端输入补全。slash 触发的是一个可审计、可授权、可拒绝的操作入口：

- 固定 quick action 来自 Memoh 内置 command 能力。
- 动态 skill 来自当前 Bot 的 effective skill catalog。
- Web 和 channel 的行为需要一致：slash 控制消息不能误入普通聊天和 LLM。

## 目标

- 用户在 Web 中可以通过 `/` 发现并执行快捷操作。
- 用户在 Web 中可以通过 slash 菜单选择 skill，并把该 skill 作为本轮请求的上下文。
- 用户在文本 channel 中可以使用 slash 触发固定 command。
- 用户在文本 channel 中可以使用 `/skill use ... -- ...` 请求本轮使用指定 skill。
- 未知 slash、非法 slash、带附件的 slash 控制消息必须被明确拒绝或静默忽略，不能进入 LLM。
- 后端是 slash 分类和执行的权威方，前端不能单方面决定一个 slash 已经执行。
- skill 内容不能通过 runtime API 暴露给客户端；客户端只能拿到安全展示信息和 opaque ref。
- “明确拒绝或静默忽略”中，静默忽略只适用于 undirected group slash；directed slash 错误必须
  给用户可见反馈。

## 非目标

- 不要求 MVP 支持 Slack、Discord、Feishu 等平台原生 slash command 注册。
- 不要求把动态 skill 注册进 Telegram native bot command menu。
- 不要求 Web quick action 覆盖所有 legacy channel command。
- 不要求 Web 手打 `/skill use name -- prompt`；Web skill 选择走 safe catalog chip。
- 不要求文本 channel 通过 path、source、hash、raw content 或 pasted `skill_ref` 消歧。
- 不要求 requested skills 替代模型自主使用的 `use_skill` tool。

## 关键定义

- **Slash**：以 `/` 开头的控制输入。
- **Quick Action**：由 slash 触发的固定快捷操作，来源于 Memoh command 能力。
- **Skill Slash**：通过 slash 选择或请求 skill，把 skill 作为本轮模型上下文。
- **Requested Skill**：用户显式要求本轮加载的 skill，不是模型自主调用工具得到的 skill。
- **Safe Catalog**：只包含安全展示字段和 opaque `skill_ref` 的 skill 列表。
- **Runtime-usable Skill**：MVP 中可被用户 requested 的 skill。必须同时满足：
  当前 Bot effective catalog 中唯一可解析、source 已安装/启用、parsed skill body/content 非空、
  可作为模型上下文加载、未被禁用/隐藏/标记为内部或 subagent-only、未被 shadow 导致歧义。
  这些状态必须由服务端 skill registry 统一归一化成 runtime visibility / usability 字段；
  resolver、safe catalog、text parser 不能各自解释 raw frontmatter 或客户端字段。malformed
  或无法判定的 visibility/usability 标记必须 fail-closed，视为 not runtime usable。
  当前 canonical name 匹配为大小写敏感的精确匹配；不做 case-fold 或 Unicode 归一化迁移。
  `Foo` 和 `foo` 是不同 skill。未来若改变大小写/Unicode 规则，必须单独设计迁移和冲突处理。
- **Directed Channel Message**：明确发给 Bot 的 channel 消息，例如私聊、mention、或 `/cmd@Bot`。
- **Undirected Group Slash**：群组里没有指向 Bot 的 slash 消息，MVP 中必须静默 no-op。

## 用户角色

- **Web 用户**：在 Memoh Web UI 中和 Bot 聊天、执行快捷操作、选择 skill。
- **Channel 用户**：在 Telegram、Discord、Lark、DingTalk、WeChat、Matrix 等 channel 中和 Bot
  互动。
- **Bot 管理者**：配置 Bot、skill、channel 和权限，关注能力边界和审计结果。
- **开发者 / 运维者**：需要明确实现边界、错误码和验收用例，避免 slash 被误路由。

## Web 需求

### Slash 发现

- 当 Web 用户在 composer 中输入 `/` 时，应看到可用 quick action 和可选 skill 入口。
- 列表必须区分固定 quick action 和动态 skill。
- skill 展示只包含安全字段，例如 name、display name、description、source kind、state。
- safe catalog 默认只返回当前 Bot runtime-usable/effective skills；禁用、shadowed、不可运行 skill
  不应交给 Web composer 再由客户端过滤。
- skill 展示不得包含 raw content、source path、raw hash、完整 `SKILL.md`。
- 不可执行或不适合 Web 的 command 不应出现在 Web quick action 可执行列表中。
- Web composer / runtime 不得使用返回 raw、content、source_path 的管理端 skill API。
  所有非管理/设置页面的 Web runtime consumer、session 信息面板、共享 skill catalog cache
  都必须迁移到 safe catalog。现有管理端 `ListSkills` 类端点必须只用于管理/设置页面，
  并受 `manage` 权限保护；聊天 composer 和聊天 runtime 不能依赖它。

### Quick Action 执行

- Web quick action 必须由后端执行。
- Web quick action 不应创建普通聊天 turn。
- Web quick action 不应把 slash 文本写入 message history。
- 无 session 的 quick action 结果应显示在当前 composer / tab 对应的 command panel 中。
- session-scoped quick action 结果应归属到对应 session 的 command panel。
- HTTP quick action request 只有在 action schema 声明 `session required = true` 时才允许携带
  `session_id`。MVP 的 `help` 和 `skill.list` 都是 sessionless；如果客户端为它们传入
  `session_id`，后端必须返回 `invalid_quick_action_scope`，且 response 不得回显该 session。
- `command_result`、`command_error`、unknown slash、unsupported Web command 都应归属到同一个
  command panel 机制，不应落入 assistant stream、聊天 turn 或错误 tab。
- 通过 WS fallback / server-side classifier 返回的 command error 也必须有 invocation/session/
  composer 归属，不能变成无法定位到当前 composer 或 tab 的全局错误。
- 执行失败时，用户 draft 不应被清空。
- 未知 slash 应显示明确错误，不应作为普通聊天发给模型。

MVP 必须至少支持这些 Web quick action：

| Action | Scope | Session required | Write action | Required behavior |
| --- | --- | --- | --- | --- |
| `help` | bot/composer | no | no | 显示可用 Web quick action 和 channel command 指引 |
| `skill.list` | bot/composer | no | no | 显示 safe skill catalog 摘要，不返回 raw content/path/hash |

MVP 中这些 known command 在 Web typed slash 中必须返回 `unsupported_web_command`，不进入 LLM：

- `/start`
- `/stop`
- `/status`
- `/context`

例外：Web 现有 `/new` composer shortcut 是 legacy Web-owned session action，不属于本轮
quick action MVP allowlist。它可以继续通过现有 Web session API 触发 session/composer 状态变化，
但 slash + 附件仍必须在附件读取、normalize、ingest 或 session 创建前拒绝。

其他未进入 Web allowlist 的 known command 也返回 `unsupported_web_command`。后续如果要支持
更多 Web quick action，必须先补 action 级需求和验收。

### Web Skill Slash

- Web skill 选择必须通过 safe catalog 产生 chip / structured payload。
- Web 用户必须输入普通 prompt；只选择 skill 不发送 prompt 应被拒绝。
- Web 用户手打 `/skill use name -- prompt` 应被拒绝，提示使用 skill chip。
- Web 不应把 skill 选择转换成自然语言文本插入 composer。
- 现有或历史上的 “Use the {skill} skill:” 这类自然语言插入式 slash 菜单必须被替换；
  实现不得在该路径上叠加 structured payload。
- requested skill 应只影响本轮模型上下文。
- requested skill full content 不应进入用户可见文本、history、title、memory extraction。
- 请求失败时，用户 draft、skill chip 状态和附件状态应可恢复或保持可重试。
- skill chip + prompt 是普通聊天发送体验，必须保持 WS streaming：loading、cancel、retry、
  stream 归属与普通聊天一致，不降级成非流式 REST。
- 如果后端在 preflight 阶段拒绝 requested skill，例如 tampered/stale ref、disabled skill、
  not runtime usable、context too large，错误必须用户可见并归属当前 composer/session。
  此类错误不创建 session、turn、assistant stream 或 history，且 draft、skill chip、附件保持可重试。
- 对无 session composer，requested skill 的服务端 preflight 必须发生在创建 session 之前；
  失败不得留下空 session 或 draft session。preflight 成功后才允许创建/选择 session 并进入
  WS streaming 主链路。

## Channel 需求

### 固定 Slash Command

- Directed channel slash 应按固定 command 语义处理。
- Undirected group slash 必须静默 no-op：无回复、无持久化、无 pipeline、无 LLM。
- `/cmd@Bot` 这类 suffix mention 应被视为 directed。
- Unknown slash 不应进入 LLM。
- Mode prefix slash 属于 channel 行为：`/now`、`/btw`、`/next` 可以用于普通 prompt。
  如果 mode prefix 后的 remainder 仍是 slash-shaped，例如 `/btw /help`、`/now /skill use ...`、
  `/next /wat`，必须拒绝，不进入 LLM。

### Channel Skill Slash

MVP 文本语法：

```text
/skill use <skill-name>[,<skill-name>...] -- <prompt>
/skill@Bot use <skill-name>[,<skill-name>...] -- <prompt>
```

需求：

- `--` delimiter 必须存在。
- `<prompt>` trim 后必须非空。
- `<skill-name>` 使用 canonical skill name，不使用 display name。
- 多个 skill 使用英文逗号分隔。
- catalog 同名/多来源/歧义、缺失、禁用、不可运行 skill 都应明确拒绝。
- channel 文本中不得接受 path、source、hash、raw content、pasted `skill_ref`。
- 服务端负责按当前 Bot 的 effective catalog 解析 name，并生成/校验 ref。
- 成功后，用户实际 prompt 是 `<prompt>`，原始 `/skill use ...` 不应写入 history、memory、title。
- ACL denied、no-trigger、unsupported context 时，原始 slash 不应 passive persist，也不应创建 session。
- directed/replyable slash 错误必须给用户可见反馈；只有 undirected group slash 静默 no-op。

解析规则：

- skill name 两侧空格 trim。
- 逗号两侧空格允许。
- 空段、`.`、`..`、非法 canonical name 必须拒绝。
- 大小写和 Unicode 规则以 catalog canonical name validator 为准，不能在 parser 和 resolver 间各自实现。
  MVP 使用大小写敏感精确匹配，不做 lower-case matching；用户输入 `foo` 不匹配 canonical name `Foo`。
- 重复 skill name 按出现顺序 deterministic de-dupe。

MVP context 支持矩阵：

| Context | `/skill use` behavior |
| --- | --- |
| Directed private chat, normal chat turn, ACL allowed, no attachment | supported |
| Directed group mention or `/skill@Bot`, normal chat turn, ACL allowed, no attachment | supported |
| Undirected group slash | silent no-op |
| ACL denied or no-trigger | reject with user-visible feedback when directed; no session, no passive persist |
| Direct attachment or referenced attachment | reject before attachment ingest |
| Active stream / inject into running stream | reject `unsupported_skill_slash_context` |
| Discuss / DCP / pre-persisted pipeline path | reject `unsupported_skill_slash_context` |
| Retry/edit/regenerate existing turn | reject `unsupported_skill_slash_context` |
| Tool approval or user input continuation | reject `unsupported_skill_slash_context` |
| Schedule/heartbeat/background trigger | reject `unsupported_skill_slash_context` |
| Already-in-context or already-persisted user message | reject `unsupported_skill_slash_context` |
| ACP-backed session or external ACP runtime | reject `unsupported_skill_slash_context` |

`base_head_turn_id` / Turn DAG 场景中，如果这是未持久化的新用户消息，只是选择某个历史 head
作为上下文分支，则视为 normal chat turn，允许 requested skills。retry/edit/regenerate
已有 turn 仍必须拒绝。

## 附件与媒体需求

- slash 控制消息带直接附件时必须拒绝。
- slash 控制消息引用带附件的消息时必须拒绝。
- 拒绝必须发生在附件 normalize、ingest、persist 前。
- 结构化 skill chip 加普通 prompt 的 Web 正常聊天可以携带普通附件，前提是该路径仍走正常附件安全处理。

## 安全与隐私需求

- 后端必须是 slash 分类和执行的权威方。
- 客户端不得通过 metadata 注入 requested/applied/model-context skill 状态。
- message metadata、content-part metadata、attachment metadata、reply attachment metadata 中的
  reserved skill key 都必须 fail-closed，不能落库或进入 LLM。
- runtime safe catalog 不得返回 raw skill content、source path、raw hash。
- safe catalog 和 requested skill API 面向有 Bot chat 权限的用户。用户请求 runtime-usable skill
  时，skill full content 会进入本轮模型上下文，因此不能承诺该 content 对该用户绝对保密。
  “不暴露给 runtime API”只表示客户端不能直接通过 API 读取 full content/path/hash。
- requested skill content 必须作为 lower-priority、delimited、untrusted context 注入模型。
  它不能覆盖 system/developer/security/session mode 指令。
- `skill_ref` 必须是 opaque token，客户端不可解码、不可构造、不可依赖内部结构。
- `skill_ref` 必须绑定当前 Bot、当前 effective catalog、skill name、source 和 content 版本。
- `skill_ref` 设计必须能稳定区分格式/篡改/wrong-bot 与 catalog/content stale，才能满足
  `invalid_skill_ref` 和 `stale_skill_ref` 两个错误码契约。MVP 不接受只有 MAC、没有 payload
  或服务端 lookup handle 的 token 设计。
- stale、tampered、wrong-bot、disabled、shadowed skill request 必须失败。
- requested skill context 超限必须 fail-closed，不能截断后继续。
- 所有 requested skills 都是 required；任意一个失败，整次请求失败。

默认限制：

| Limit | Default | Measurement |
| --- | --- | --- |
| requested skill count | 5 | 去重后的 skill 数 |
| single skill context size | 64 KiB | 解析后的 skill body/content UTF-8 bytes |
| total requested skill context size | 256 KiB | 本轮所有 requested skill body/content UTF-8 bytes 总和 |

限制可以由 server config 调整，但必须可测试、可观测，并且超限时整次请求失败。

## 持久化与审计需求

- 普通聊天 history 只记录用户 prompt，不记录原始 slash control text。
- requested skill full content 不进入 history、title、memory extraction、display text。
- 审计 metadata 只能由服务端 resolver/store 写入。
- 审计 metadata 只能记录 allow-list 字段：skill name、source kind、requested/use_skill 来源、
  opaque `skill_ref` 或 safe catalog 展示字段摘要。
- 审计 metadata 不得包含 full content、source path、raw hash。
- session used skills 可以展示 requested skill 和模型自主 `use_skill` 的结果，但内部应能区分来源。

## 错误与用户反馈

必须有稳定 machine code，至少覆盖：

| Code | Meaning |
| --- | --- |
| `unknown_slash` | 未知 slash，不进入 LLM |
| `unsupported_web_command` | known command 不在 Web quick action allowlist |
| `use_skill_chip_required` | Web 手打 `/skill use ...`，要求使用 skill chip |
| `invalid_skill_slash_syntax` | channel `/skill use` 语法错误 |
| `missing_prompt` | slash skill 请求缺少用户 prompt |
| `requested_skill_not_found` | skill name 不存在 |
| `requested_skill_ambiguous` | skill name 多来源或 shadow 造成歧义 |
| `requested_skill_disabled` | skill 已禁用 |
| `requested_skill_not_runtime_usable` | skill 不允许作为 runtime requested skill |
| `invalid_skill_ref` | `skill_ref` 格式错误、被篡改或属于错误 Bot |
| `stale_skill_ref` | `skill_ref` 对应的 catalog/content 已变化 |
| `too_many_requested_skills` | requested skill 数量超过限制 |
| `requested_skill_context_too_large` | 单个或总 skill context 超限 |
| `slash_attachments_unsupported` | slash 控制消息携带或引用附件 |
| `unsupported_skill_slash_context` | 当前上下文不支持 requested skills |
| `unsupported_legacy_endpoint` | legacy Web message endpoint 不支持 requested skills |
| `invalid_quick_action_scope` | quick action request 携带了 action schema 不允许的 session scope |
| `permission_denied` | directed slash 权限不足 |
| `reserved_skill_metadata` | 客户端或 hook 试图注入保留 skill metadata |

错误反馈要求：

- Web 错误不清 draft。
- Web 错误不创建 session / turn / assistant stream。
- Web command 错误归属到当前 composer/session 的 command panel，不落入聊天流。
- Web command panel 必须优先按稳定 machine code 使用前端 locale 文案展示错误；
  server 返回的英文 message 只能作为未知 code 的兜底，不能让 zh/ja UI 固定显示英文。
- channel directed/replyable 错误必须给用户可见反馈，同时不创建 session、不 persist。
- undirected group slash 错误不回复。
- undirected group slash 优先 silent no-op，同时不得 ingest 附件。directed slash-shaped 消息若带
  直接或引用附件，优先返回 `slash_attachments_unsupported`，再判断 unknown/unsupported/syntax。
- 所有用户可见错误必须提供 en/zh/ja 本地化文案。

## 成功指标

- Web 用户能发现并执行可用 quick action。
- Web 用户能通过 skill chip 指定本轮使用的 skill。
- Channel 用户能通过 `/skill use skill -- prompt` 请求本轮使用 skill。
- Unknown slash 不再误进 LLM。
- slash 控制消息不再误写入 history / memory / title。
- 相关安全测试能证明客户端无法伪造 requested skill、source path、raw content 或 metadata。

## 验收标准

### Web

- 输入 `/` 能展示 quick action 和 skill 入口。
- MVP Web quick action 至少包含 `help` 和 `skill.list`。
- `/start`、`/stop`、`/status`、`/context` 在 Web typed slash 返回
  `unsupported_web_command`。
- Web legacy `/new` composer shortcut 不属于 quick action allowlist；带附件时返回
  `slash_attachments_unsupported` 且保留 draft/附件。
- 执行 Web quick action 不创建聊天 turn 或 assistant stream。
- quick action 成功显示当前 composer/session 归属的 command panel。
- quick action 失败显示当前 composer/session 归属的 command panel，且不清 draft。
- unknown slash 返回 `unknown_slash`，不清 draft、不进 LLM、不创建 session。
- WS fallback / server-side classifier 返回的 unknown slash 或 unsupported command 错误能稳定归属
  当前 composer/session command panel。
- 手打 `/skill use name -- prompt` 返回 `use_skill_chip_required`，不清 draft。
- Web slash + 直接附件返回 `slash_attachments_unsupported`，附件不 ingest。
- Web skill chip + prompt + 普通附件应保持普通聊天附件体验，不应被当作 slash 控制消息附件拒绝。
- skill chip + prompt 通过 typed payload 进入后端。
- skill chip + prompt 保持普通聊天 WS streaming 体验。
- skill-only send 返回 `missing_prompt`。
- Web skill chip 的 stale/tampered/disabled/not runtime usable/context too large 等 preflight 错误
  归属当前 composer/session，不创建 session、turn、assistant stream 或 history，并保留 draft/chip/附件。
- 无 session composer 中的 failed requested skill preflight 不得留下空 session 或 draft session。
- requested skill full content 不出现在 UI 文本、history、title、memory。
- legacy Web message endpoint 携带 requested skills 返回 `unsupported_legacy_endpoint`。

### Channel

- 私聊 `/help` 进入 command。
- 群组 undirected `/help` 静默 no-op。
- 群组 `/help@Bot` 进入 command。
- `/now hello`、`/btw hello`、`/next hello` 按 mode prompt 处理。
- `/btw /help`、`/now /skill use a -- x`、`/next /wat` 被拒绝，不进入 LLM。
- `/skill use a -- do x` 成功转成 prompt `do x` + requested skill `a`。
- `/skill use a` 返回 `invalid_skill_slash_syntax`。
- `/skill use a --` 返回 `missing_prompt`。
- `/skill use a,b -- do x` 成功请求两个 skills。
- `/skill use a,a -- do x` deterministic de-dupe 后成功请求一个 skill。
- `/skill use a,,b -- do x` 返回 `invalid_skill_slash_syntax`。
- `/skill use . -- do x`、`/skill use .. -- do x`、非法 canonical name 均被拒绝。
- `/skill@Bot use a -- do x` 在 directed group 中成功。
- `/skill use missing -- do x` 被拒绝。
- `/skill use ambiguous -- do x` 被拒绝。
- directed unknown slash 给用户可见错误，不创建 session、不 persist、不进 LLM。
- ACL denied 的 `/skill use ...` 给用户可见错误，不创建 session、不 passive persist。
- 带附件或引用带附件的 slash 被拒绝且附件不 ingest。
- active stream、discuss/DCP、retry/edit、tool approval/user input continuation、schedule/heartbeat
  场景中的 requested skills 返回 `unsupported_skill_slash_context`。
- ACP-backed session 中 requested skills 返回 `unsupported_skill_slash_context`。

### 安全

- 客户端不能用 metadata 伪造 requested skills。
- 客户端不能提交 source path/raw content/hash 作为 runtime requested skill。
- tampered `skill_ref` 返回 `invalid_skill_ref`。
- wrong-bot `skill_ref` 返回 `invalid_skill_ref`。
- stale content `skill_ref` 返回 `stale_skill_ref`。
- disabled/shadowed skill 被拒绝。
- not runtime usable skill 返回 `requested_skill_not_runtime_usable`。
- safe catalog 响应不包含 raw content、source path、raw hash。
- safe catalog 响应不包含 disabled、shadowed、not runtime usable skills。
- 所有非管理/设置页面的 Web skill consumer 都不再调用返回 raw/content/source_path 的 full skill
  list；管理端 full skill list 若保留这些字段，必须受 `manage` 权限保护。
- runtime visibility/usability 标记由服务端 registry 统一归一化；malformed 或无法判定时
  safe catalog 和 resolver 都 fail-closed。
- 客户端提交 source/source_path/raw/hash/content 等展示或审计字段不能进入服务端 audit metadata。
- requested skill context 超限时整次请求失败，且不写 user message / audit applied。
- 所有 failed requested skill preflight 都不得写入 `applied_skills` 或 `model_context_skills`。
- requested skill 数量超过 5 默认返回 `too_many_requested_skills`。
- 单 skill 超过 64 KiB 或总 context 超过 256 KiB 默认返回 `requested_skill_context_too_large`。
- requested skill context 以 lower-priority、delimited、untrusted context 进入模型，不能覆盖
  system/developer/security/session mode。
- 审计 metadata 只包含 allow-list 字段，并能区分 requested skill 与模型自主 `use_skill`。

## 与实现方案的关系

本文不规定具体 Go/Vue 文件、内部接口名、FX 注入方式或数据库 SQL 结构。实现这些内容时，以
`docs/slash-quick-action-skill-plan.md` 为准。

如果实现方案后续改变了产品行为，必须同步更新本文，并重新进行 3-agent review。

## Review Log

### V1.0 Review 采纳项

- 补充 Web quick action MVP 可验收清单：`help`、`skill.list`，并明确 route-aware known
  commands 在 Web 返回 `unsupported_web_command`。
- 补充 channel `/skill use` supported / unsupported context 矩阵。
- 补充 requested skill 默认数量和 context size 限制。
- 将自然语言错误标签改成稳定 machine code。
- 明确 Web command/result/error 必须归属 command panel，不进入 assistant stream 或聊天 turn。
- 明确 directed/replyable channel slash 错误必须给用户可见反馈；静默只适用于 undirected group。
- 补充 chat 权限下 requested skill content 进入模型上下文的隐私承诺边界。
- 补充 requested skill 作为 lower-priority、delimited、untrusted context 注入模型。
- 补充审计 metadata allow-list 和服务端写入边界。

### V1.0 Review 不采纳项

- 不要求 MVP 支持平台原生 slash command 注册。
- 不要求 Web 手打 `/skill use name -- prompt`。
- 不要求文本 channel 通过 path/source/hash/raw content/pasted `skill_ref` 消歧。
- 不在需求文档中复制 token payload、keyring、DB row、Go 接口等实现细节。

### V1.2 Review 采纳项

- 补充 Web 验收中的 `/start`，与正文 route-aware known command 列表保持一致。
- 补充 Web skill chip preflight 错误的归属：必须用户可见、关联当前 composer/session，
  不创建 session、turn、assistant stream 或 history，并保留 draft/chip/附件可重试。
- 补充 skill chip + prompt 必须保持普通聊天 WS streaming 体验。
- 补充 safe catalog 响应不包含 raw content、source path、raw hash 的安全验收。
- 补充 not runtime usable skill 的显式验收。

### V1.2 Review 不采纳项

- 不把当前代码未实现作为 blocker。
- 不把平台原生 slash command 注册纳入 MVP。
- 不把 Notion external object/PDF 当前不可展开作为 blocker；后续拿到完整原始内容再重校。
- 不从用户指定的两个 Notion 任务以外合并相邻 ACP/slash 任务，避免扩大需求边界。

### V1.3 Follow-up Review 采纳项

- 定义 runtime-usable skill，并明确 MVP 使用大小写敏感的 canonical name 精确匹配。
- 明确 Web composer/runtime 必须迁移到 safe catalog，不能继续使用返回 raw/content/source_path 的管理端 skill API。
- 明确现有自然语言插入式 skill slash 菜单必须替换为 chip / structured payload。
- 补充 mode prefix 行为：`/now`、`/btw`、`/next` 支持普通 prompt，mode + slash remainder 拒绝。
- 补充 ACP-backed session 中 requested skills 拒绝。
- 对齐 legacy endpoint、错误优先级、i18n、base_head_turn_id 普通新 turn 语义。

### V1.3 Follow-up Review 不采纳项

- 不把相邻 ACP slash 任务并入本需求。
- 不把 Notion 截图作为额外规范性需求来源。
- 不要求 Web 手打 `/skill use name -- prompt`。

### V1.4 Follow-up Review 采纳项

- 明确 safe catalog 迁移范围覆盖所有非管理/设置页面的 Web runtime consumer 和共享 cache。
- 明确 Web WS fallback / server-side classifier 错误必须带 invocation/session/composer 归属。
- 明确无 session composer 的 requested skill preflight 必须先于 session 创建，失败不留下空 session。
- 明确 runtime visibility/usability 标记由服务端 registry 统一归一化，malformed 或无法判定时 fail-closed。
- 明确客户端 source/path/raw/hash/content 等展示或审计字段不能进入服务端 audit metadata。

### V1.4 Follow-up Review 不采纳项

- 不把已有空 session 行为作为需求保留；MVP 目标仍是失败请求不创建 session。

### V1.5 Follow-up Review 采纳项

- 明确 `skill_ref` 不能是 MAC-only token；必须具备 authenticated payload 或服务端 lookup
  handle，才能区分 `invalid_skill_ref` 与 `stale_skill_ref`。

### V1.5 Follow-up Review 不采纳项

- 不合并 `invalid_skill_ref` 和 `stale_skill_ref`；需求仍保留两个稳定错误码。

### V1.6 Final Review 采纳项

- 确认当前需求保留 encrypted/lookup-handle `skill_ref`，不接受 MAC-only token。
- 确认简化只发生在交付切片，不降低 unknown slash、safe catalog、preflight、query split、
  audit 和 fail-closed 需求。

### V1.6 Final Review 不采纳项

- 不为了简化实现而放宽 `invalid_skill_ref` / `stale_skill_ref` 的可区分错误码契约。

### V1.7 Implementation Review 采纳项

- 明确 Web slash + 附件必须在附件读取/ingest/session 创建前拒绝，并归属 command panel。
- 明确 requested skills 只允许普通 model chat；channel 新 discuss/ACP session 和 Web 既有
  discuss/ACP session 都必须 fail-closed。
- 明确 deploy/server `skill_ref` 占位符必须 fail-fast，installer upgrade 需要补齐 keyring。

### V1.7 Implementation Review 不采纳项

- 不把现有 Web `/new` composer shortcut 改成本轮 quick action MVP；它是 legacy Web-owned
  session action，但必须遵守 slash + 附件拒绝边界。

### V1.8 Implementation Review 采纳项

- 明确普通 IM channel 新 stream 即使携带 dispatcher inject channel，也必须支持 `/skill use`；
  只有已存在 active stream、准备注入/continuation 的场景才拒绝 requested skills。
- 明确 Web no-session requested-skills 失败清理已创建 session 时，必须通知 workspace/tab 层该
  session 已删除，错误面板仍归属原 composer。
- 明确 deferred draft 的 late startup failure 在用户已切到其他 bot/session 后不得再恢复旧
  prompt、附件或 skill chip，也不得之后写回 draft storage。

### V1.8 Implementation Review 不采纳项

- 不把普通 channel stream 的 dispatcher `InjectCh` 视为 active inject；它只是后续消息注入通道。

### V1.9 Implementation Review 采纳项

- no-session requested-skills 失败后，如果 Web draft panel 曾被 promoted 为 created session，
  cleanup 必须保留原 panel/composer scope 并把它恢复为 draft，让 command/error panel 仍可见；
  普通 WS startup error 和 command_error 都必须满足该行为。
- Channel tool approval / user input continuation 运行期间，新的 `/skill use` 必须按 active-stream
  context 拒绝，不能误开另一条 requested-skills stream。
- malformed `skill_ref` 包括错误 nonce 长度都必须返回 `invalid_skill_ref`，不能 panic。
- Web WS existing-session requested-skills 必须先做 session authorization，再做 active-stream
  检查和 reservation，避免无权 session 泄露 active 状态。

### V1.9 Implementation Review 不采纳项

- 不改变普通手动删除 session 的 tab 行为；只有 failed deferred-session cleanup 需要原地恢复 draft。

### V1.10 Implementation Review 采纳项

- Web no-session requested-skills 的普通 WS startup error 也必须携带原 composer scope 完成 cleanup，
  不能只在 `command_error` 路径恢复 promoted draft panel。
- Channel continuation 与普通主 stream 重叠时，continuation 结束不能让 route 变成 idle；新的
  `/skill use` 仍要在主 stream 期间被拒绝，queued/pending work 只能在最后一个 active owner 结束后处理。

### V1.10 Implementation Review 不采纳项

- 不放宽 continuation 期间的 `/skill use` 拒绝要求；这是 requested skills 只允许普通新 chat turn
  的一部分。

### V1.11 Implementation Review 采纳项

- Web WS existing-session slash command 必须先过 session authorization，再返回 command result/error。
- `/skill use` delimiter 必须是带空白边界的 `--` token；合法 skill name 内部的 `--` 不应被切分。
- full skill list 若返回 raw/content/source_path，必须要求 `manage` 权限；chat/runtime 只用 safe catalog。
- reserved metadata denylist 覆盖 message/content-part/attachment/reply attachment metadata，并在
  Web REST、Web WS、channel inbound、flow 入口 fail-closed。
- runtime visibility/usability 由服务端 `Entry` 归一化字段承载；safe catalog 和 resolver 读取归一化结果。

### V1.11 Implementation Review 不采纳项

- 不通过禁止双连字符来规避 `/skill use` 解析问题；canonical name 仍按既有 validator。

### V1.12 Implementation Review 采纳项

- HTTP quick action 必须按 action schema 校验 scope；sessionless action（MVP 的 `help`、
  `skill.list`）收到 `session_id` 时返回 `invalid_quick_action_scope`，避免任意客户端伪造
  session-scoped command result。
- Web composer 调用 sessionless quick action 时不发送当前 selected session id；本地 command
  panel 仍可按 composer/session UI 上下文展示结果，但后端不会把未定义的 session scope 当作
  权威归属。

### V1.12 Implementation Review 不采纳项

- 不把 MVP `help` / `skill.list` 改成 session-scoped action；它们的数据来源是 bot/composer
  级别，未来如需 session action 必须先补 action schema、权限和验收。

### V1.13 Implementation Review 采纳项

- Directed/private channel 的 fixed slash command 在 command access 拒绝时必须返回
  `permission_denied` 可见反馈，不能只写日志后静默返回；只有 undirected group slash 才允许
  hard no-op。
- Flow 入口的 reserved skill metadata denylist 必须覆盖 `ChatRequest.Messages` 的 content-part
  metadata，避免调用方伪造 requested/applied/model-context skill 状态进入模型上下文。
- Web typed slash fallback 和普通 REST fallback 必须调用实际注册的 Web local-channel endpoint
  `/bots/{bot_id}/web/messages`，不能误打 SDK 当前生成的 `/local/messages`。
- Web command panel 按 `error.code` 使用 en/zh/ja 本地化文案；server `message` 只作为未知 code
  兜底。

### V1.13 Implementation Review 不采纳项

- 不把 server-side `message` 作为 Web 本地化来源；server 当前没有 Web 用户 locale，前端按
  machine code 本地化更稳定。

### V1.14 Implementation Review 采纳项

- Skill frontmatter 或 `metadata` 字段在 parser 层 malformed 时，registry 必须保留可管理的
  skill entry 但将 runtime usability 判定为 fail-closed；safe catalog 和 resolver 不得把这类
  skill 当作 runtime-usable。

### V1.15 Implementation Review 采纳项

- Web quick action 和 typed slash fallback 的 HTTP/SDK transport failure 必须收敛为当前
  composer/session scope 的 `command_error`，并返回 startup failure；不能因为异常绕过
  `sendMessage` 恢复逻辑而清掉 draft、附件或 skill chip。

### V1.16 Implementation Review 采纳项

- Web requested skill chip 可以在本地保留 `display_name`、`description`、`source_kind`、
  `state` 等展示字段，但发给后端的 `requested_skills` DTO 必须只包含 `{ skill_ref, name }`；
  audit/display 字段只能由服务端 resolver/store 生成。
