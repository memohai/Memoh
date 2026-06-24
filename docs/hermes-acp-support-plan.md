# Hermes ACP 支持方案

Hermes 不适合按“单一模型 Provider”接入 Memoh。它更像一个二级 Agent
Runtime：Hermes 自己有 profile、模型路由、记忆、skills、cron、session
日志和工具系统。Memoh 要做的是通过 ACP 启动 Hermes，同时给每个 Bot 明确
状态边界、配置边界和权限边界。

本文是完整实现方案，不是只做一个 profile 的临时方案。实现可以按 Phase 1-4 分步合入，
但最终目标是把 Hermes 做成和 Codex/Claude Code 一样的一等 ACP Agent：本地和容器都能
启动、托管配置能保存并生效、危险操作走 Memoh permission、密钥不会被 backup/export
泄漏、配置变更后 runtime 会正确重启。Phase 1 的 `local + self` 只是第一步可用路径，
不是完整终态；完整终态以 Phase 4 的 toolkit 容器路径通过验收为准。

## 当前实现落点

本次实现已覆盖核心终态路径，不再停留在 Phase 1；仍建议把真实 provider/permission
smoke 放进 PR CI、nightly 或 release gate：

- ACP Profile 注册 `hermes`，命令为 `hermes-acp`，支持 `local` / `container`
  backend，支持 `self` / `api_key` setup mode。
- Managed Hermes 使用 Bot 级 `HERMES_HOME`：容器为 `/data/.memoh-hermes`，
  本地为 `<local-data-root>/acp/hermes/<bot-id>`。
- `provider` / `model` / `base_url` / `api_key` 按 Hermes 官方文档的
  `model.provider` / `model.default`、`providers.*` named custom provider 和
  `.env` secret 形态写入 Hermes `config.yaml` 和 `.env`；第一批只承诺
  `openrouter`、`openai-api`、`custom`，
  并接受 `openai` 作为 `openai-api` 的兼容别名。
- Session 启动和配置写入共用 `ResolvedSessionContext`，避免写入路径和运行路径漂移。
- 本地 managed config 由 server 直接写入 app data 目录；容器 managed config 才通过
  workspace bridge 写入 `/data/.memoh-hermes`，不要求 local bridge 放开 workspace 外绝对路径。
- 容器 managed Hermes 使用 clean env，并显式清理 `HERMES_*` 和常见 Provider Key；
  Hermes 创建的 terminal tool 也继承同一套 env 约束。
- Bot metadata 更新后会关闭旧 ACP runtime；managed config 字段发生变化时，写入失败会在
  保存请求中暴露。无关 metadata 更新不强依赖 workspace bridge 可达，避免容器停机时阻断
  ACL、display name 等普通保存。
- Backup/export 默认 scrub ACP managed secret，workspace tar.gz 默认排除
  `.memoh-hermes/`、`.hermes/`、`.codex/` 下的 `.env` / `auth.json`
  和嵌套 `.env` / `auth.json`。
- Toolkit 增加 `hermes-acp` wrapper，默认只走离线 `uv tool run --offline`，
  wrapper 会先把 toolkit 内的 uv seed cache/tool/python 复制到可写 runtime 目录，
  避免在只读 `/opt/memoh/toolkit` 下写锁或生成元数据；只有显式设置
  `MEMOH_HERMES_ALLOW_NETWORK_INSTALL=1` 才允许运行时联网安装。
- 前端 metadata 校验支持 Hermes provider/model/custom base_url 规则。

## 当前验收记录

- Go / Web / shell / diff 校验已通过：`go test ./internal/...`、`go test ./cmd/...`、
  `pnpm --filter @memohai/web build`、Hermes metadata Vitest、相关 Web ESLint、
  `sh -n docker/toolkit/install.sh`、`sh -n docker/toolkit/bin/hermes-acp`、
  `git diff --check`。
- Linux/arm64 Docker toolkit smoke 已通过：在 `alpine:3.23` 容器内完整运行
  `docker/toolkit/install.sh`，生成 Linux arm64 toolkit 后，`hermes-acp --help`
  可以通过离线 `uv tool run --offline` 启动。
- 只读 toolkit smoke 已通过：将 `/tmp/memoh-toolkit` 改成只读后，用非 root 用户运行
  `docker/toolkit/bin/hermes-acp --help`，wrapper 会把 uv cache/tool/python seed 复制到
  可写 runtime 目录并成功启动。
- 仍建议在 PR CI 或 release gate 中补 `linux/amd64` offline smoke，以及带 fake 或真实
  OpenAI-compatible provider 的 session smoke 和 permission end-to-end smoke；这些是发布质量
  gate，不改变当前代码 schema 的依据。

## 目标

- 把 Hermes 加为 Memoh 的一等 ACP Profile。
- 复用现有 ACP Runtime 模型：Memoh 作为 ACP Client，Hermes 作为工作区中的
  ACP stdio Agent 进程。
- 先支持本地工作区 self 模式，再逐步支持 managed config 和容器工作区。
- 在 Memoh 托管配置时，为每个 Bot 隔离 Hermes profile 和状态。
- 不把 Hermes 的完整配置系统复制进 Memoh UI。
- 不把 API Key 放到进程参数或长期进程环境变量里。
- 不绕过 Memoh 现有 ACP permission callback 和 workspace 边界。

## 非目标

- 不重做 Hermes 的完整 Provider Registry、模型选择器、toolset 配置、memory
  配置、MCP 配置、cron 配置或 profile UI。
- 第一版不接 Hermes OAuth / Portal 交互式授权。
- 不把 Hermes 设为默认 ACP Agent。
- 不承诺 Hermes 自带浏览器、skills、delegate、execute_code 等高级能力在第一版
  已完整托管。

## PR 合入口径

这份文档本身可以作为设计 PR 提交。后续实现 PR 可以按阶段拆，但拆分只代表实施顺序，
不代表后续能力可选。最终完整实现必须满足：

- 本地 workspace 可以启用 Hermes 并完成真实 session。
- 容器 workspace 不要求用户在镜像里手动安装 Hermes，而是通过 Memoh toolkit 的
  `hermes-acp` wrapper 启动。
- managed 模式由 Memoh 写入 Bot 级配置和 secret，保存失败要在保存请求中暴露，不能等到
  第一次 session 才失败。
- 危险工具调用走 Memoh permission/approval，stream event 和审批标题映射到统一语义。
- profile、settings、onboarding、backup/export、runtime restart、错误提示和现有 ACP
  agent 使用同一套产品行为。

如果第一批实现 PR 只做到 Phase 1，PR 标题和描述必须明确这是完整方案的第一阶段，
例如 `Add Hermes ACP local self support`，不能宣称完整 Hermes ACP support 已完成。
完整 Hermes ACP support 的 PR 或 release note 只有在 Phase 1-4 的验收都通过后才成立。

### 关键工程项含义

这里的“安全隔离、Permission、Backup、Runtime”不是抽象概念，也不是可不做的加分项。
它们分别对应下面这些必须实现的工程边界：

- **安全隔离**：managed Hermes 必须使用 Bot 级 `HERMES_HOME`，不能共享宿主
  `~/.hermes`；ACP 进程和它发起的 terminal tool 不能继承 server、宿主 shell 或镜像里的
  Provider Key；容器模式必须优先使用 toolkit 内置 `hermes-acp`，不能被 PATH 上的安装
  偶然掩盖。
- **Permission**：Hermes 通过 ACP 请求文件写入、编辑、shell、`execute_code` 等危险操作时，
  必须进入 Memoh 现有 approval 流程。Memoh 要把 Hermes 的 permission event 映射成统一的
  `write/edit/exec` 审批和 stream event；映射不到就 fail closed。
- **Backup/export**：Bot metadata 里的 `managed.api_key`、`HERMES_HOME/.env`、
  `auth.json` 等 secret 默认不能导出明文。导入后要提示用户重新填写 key，并重建空的
  Hermes home。
- **Runtime start/restart**：启动 Hermes 时，config writer 和 Runner 必须使用同一份
  resolved path/context；Bot 的 ACP metadata、managed config 或 secret 变化后，要关闭旧
  runtime 或通过 fingerprint 强制重建，不能复用旧进程继续跑旧配置。

## 调研依据

Hermes 的 ACP 入口包括：

- `hermes acp`
- `hermes-acp`
- `python -m acp_adapter`

Hermes stdout 保留给 ACP JSON-RPC，普通日志写 stderr。Hermes 的 PyPI 包通过
`hermes-agent[acp]` 暴露 `hermes-acp` 脚本，但 Memoh managed Hermes 必须使用
`hermes-agent[acp,mcp]`：`acp` 只提供 ACP 入口，`mcp` 才会安装 Hermes 运行 ACP
传入的 HTTP MCP server 所需的 Python SDK。上游 ACP Registry 在调研时使用 pinned
`uvx` 分发方式：

```text
uvx --from 'hermes-agent[acp,mcp]==0.17.0' hermes-acp
```

Hermes 使用 `HERMES_HOME` 作为 profile 和状态边界。一个 Hermes profile 通常
包含：

```text
config.yaml
.env
auth.json
SOUL.md
memories/
skills/
cron/
sessions/
logs/
state.db
```

Hermes 的配置优先级是：

1. CLI 参数。
2. `HERMES_HOME/config.yaml`。
3. `HERMES_HOME/.env`。
4. 内置默认值。

关键点：`HERMES_HOME` 管 Hermes 自己的状态；`HOME` 仍然是普通系统用户 home，
会影响 `git`、`ssh`、`gh`、`npm`、cloud CLI 等外部工具。

参考资料：

- `https://hermes-agent.nousresearch.com/docs/user-guide/features/acp`
- `https://hermes-agent.nousresearch.com/docs/developer-guide/acp-internals`
- `https://hermes-agent.nousresearch.com/docs/user-guide/configuration`
- `https://hermes-agent.nousresearch.com/docs/user-guide/profiles`
- `https://raw.githubusercontent.com/NousResearch/hermes-agent/main/pyproject.toml`
- `https://raw.githubusercontent.com/NousResearch/hermes-agent/main/acp_registry/agent.json`

## 现有 Memoh ACP 模型

ACP Agent 在 `internal/acpprofile/profile.go` 注册。一个 Profile 定义：

- Agent ID 和显示名。
- 容器工作区命令 `Command`。
- 本地工作区命令 `LocalCommand`。
- 托管字段 `ManagedFields`。
- 支持的工作区后端 `SupportedBackends`。
- 支持的配置方式 `SetupModes`。
- 可选的 Session Mode 固定值。
- 可选的工具标题兼容规则 `ToolQuirks`。

`/acp/profiles` API 只返回安全元数据：显示名、描述、托管字段 schema、支持的
后端、配置方式。它不应暴露可执行命令、包名、环境变量名或密钥。

运行时由 `internal/acpagent.SessionPool` 负责：

- 从 Bot metadata 读取 `metadata.acp.agents.<agent_id>`。
- 解析 setup mode 和 managed fields。
- 校验必填托管字段。
- 准备 Agent 专属托管配置。
- 使用 Profile 命令和环境启动 `acpclient.Session`。
- 把 Runtime 绑定到 Chat Session 和 Project Path。

`internal/acpclient.Runner` 负责：

- 解析工作区后端和项目路径。
- 通过 workspace bridge 启动 ACP Agent stdio 进程。
- 初始化 ACP 连接。
- 创建带 `cwd` 的 ACP Session。
- 向 Agent 暴露 Memoh 的文件和终端 Client Capabilities。
- 如果 Agent 支持 HTTP MCP Server，就把 Memoh Tools 通过 HTTP MCP Bridge
  暴露给 Agent。

容器工作区里的 ACP 命令来自 `/opt/memoh/toolkit/bin`。该目录由 Memoh
workspace runtime 只读挂载进容器。现有 `codex-acp` 和 `claude-agent-acp`
wrapper 优先使用 toolkit 内置包，内置包缺失时才 fallback 到 `PATH` 上的同名
命令。

本地工作区不同。本地工作区通过 local bridge 在宿主机上运行命令，所以 Profile
可以提供 `LocalCommand` 和 `LocalArgs`。现有 Codex、Claude Code 在本地工作区
使用宿主机的 `npx -y ...`。

## 核心设计决策

### 统一 Agent 行为合同

Hermes 不能作为一套独立的产品行为进入 Memoh。用户打开一个 Bot 后，无论底层是
Memoh native agent、Codex/Claude ACP agent，还是 Hermes ACP agent，外层行为都应尽量
一致。差异只应该来自 Agent 自身能力，而不是 Memoh 的集成边界不一致。

必须统一的行为：

- **Workspace 语义统一**：同一个 Bot/session 下的 project path、cwd、local/container
  路由、toolkit PATH 规则保持一致。Hermes 的 `HERMES_HOME` 只管 Hermes profile，不改变
  Memoh 对工作区的定义。
- **工具审批统一**：文件写入、终端命令、代码执行、浏览器/外部副作用等危险操作必须走
  Memoh 的 approval / permission 机制。Hermes 自己的内部工具不能绕过 Memoh workspace
  capability。
- **环境隔离统一**：managed 模式不能继承宿主或 server 里的 provider key。ACP process
  和由它发起的 terminal tool 都使用同一套 sanitized env。
- **配置生命周期统一**：Bot metadata 保存、managed config 落盘、runtime
  invalidate/restart、错误反馈要和现有 managed ACP 体验一致，不能出现“保存成功但首次
  session 才失败”的隐藏状态。
- **密钥处理统一**：API response scrub、metadata merge、backup/export、import 后重填
  key 的行为和其他 managed secrets 一致。
- **UI 暴露统一**：不可用的 backend/setup mode 不展示或禁用；错误原因要在 Bot 设置页和
  onboarding 中可见。
- **测试口径统一**：每个 ACP Agent 都应该有 profile metadata 测试、process/env 测试、
  offline runtime smoke、provider smoke、permission smoke 和 frontend metadata 测试。

允许存在的差异：

- Hermes 可以有自己的 `HERMES_HOME`、profile、memory、skills、sessions、cron 和 logs。
- Hermes 可以有自己的 provider/model 配置格式。
- Hermes 第一版可以只承诺 ACP coding path，不承诺官方 Docker/Nix 中包含的 dashboard、
  messaging、browser、Playwright、TTS 等完整能力。
- Phase 1 的 self 模式可以使用宿主 Hermes profile，但必须在 UI 中明确标成共享宿主状态
  的高级模式；它不承诺 Bot 级隔离，不应被描述成 fully managed Agent。

因此，Hermes 的接入原则是：**外层行为按 Memoh Agent Runtime Contract 统一，内部
profile/config 按 Hermes 原生模型隔离。**

### 分阶段暴露能力

不要一次性把 `local`、`container`、`self`、`api_key` 都暴露出去。现有前端会
按 `/acp/profiles` 返回内容渲染选项，如果 Profile 过早声明支持容器或 managed，
用户就会看到还没有验证的路径。

建议：

- Phase 1 只注册 `SupportedBackends: ["local"]` 和 `SetupModes: ["self"]`。
- Phase 2 完成 managed config、安全、导出策略、保存失败语义和 smoke gate 的隐藏实现，
  不开放 UI。
- Phase 3 在 Phase 2 全部通过后，再开放 `api_key`。
- Phase 4 完成 toolkit 离线打包后，再开放 `container`。
- 前端必须按当前 bot/workspace backend 过滤或禁用不支持的 profile/setup mode。只靠
  后端 Profile 声明不够，因为现有 onboarding 和设置页会直接渲染 `/acp/profiles`。
- 如果希望提前合入完整代码，必须用后端 feature gate 加前端过滤，保证未完成路径
  不出现在 UI，也不会写入 bot metadata。

### `HERMES_HOME` 是托管边界

Managed 模式必须设置 Bot 级 `HERMES_HOME`。Self 模式不能假装被 Memoh 托管：
如果不设置 `HERMES_HOME`，Hermes 会使用运行环境默认 profile，通常是
`~/.hermes`，这意味着多个 Bot 可能共享密钥、memory、skills、sessions、logs
和 cron。这个模式应在 UI 上标为“使用宿主 Hermes profile”的高级选项。

更稳的长期方向是给 self 模式增加可选 `hermes_home` 字段：

- 默认：用户显式选择宿主默认 profile。
- 可选：用户指定某个已配置的 Hermes profile 目录。
- 不建议：Memoh 在 self 模式偷偷生成 profile，因为这会改变 self 模式语义。

### `HOME` 不等于 `HERMES_HOME`

Managed Hermes 不应使用当前 `prepareProcessEnv` 为非 Codex 容器 Agent 准备的
临时 `HOME=/tmp/memoh-acp/<uuid>`。Hermes 的 memory、skills、sessions 需要保留，
不能落到进程结束后删除的临时目录。

实现上要对 Hermes 做特殊处理：

- 容器 managed Hermes 注入 `HERMES_HOME`。
- 容器 managed Hermes 不进入“非 Codex managed Agent 使用临时 HOME”的分支。
- `HOME` 只在明确需要稳定系统 home 时设置，不能把它当作 Hermes profile 目录。

### 单一 Hermes home 解析源

配置写入路径和进程运行时 `HERMES_HOME` 必须来自同一个 helper。否则
`SessionPool` 用 `workspaceInfo.DefaultWorkDir` 写配置，`Runner` 又用
`resolveWorkspacePaths` 计算运行路径，两边很容易漂移。

最终实现要避免 `SessionPool` 和 `Runner` 各自解析路径。方案定为新增一个
`ResolvedSessionContext`，由同一处解析出 workspace root、project path、cwd、backend
和 Hermes home。`SessionPool` 用这个 context 写 config；`Runner.StartRequest` 接收
同一个 context 并直接使用，不能再自行重算。如果没有传入 context，Runner 只能走旧路径
兼容非 Hermes agent。

建议 API 形态：

```go
type SessionContextInput struct {
    AgentID       string
    SetupMode     SetupMode
    BotID         string
    Backend       string
    WorkspaceRoot string
    ProjectPath   string
    LocalDataRoot string
}

type ResolvedSessionContext struct {
    AgentID       string
    SetupMode     SetupMode
    Backend       string
    WorkspaceRoot string
    ProjectPath   string
    CWD           string
    HermesHome    string
}

func ResolveSessionContext(input SessionContextInput) (ResolvedSessionContext, error)
```

这个 context 至少被两处共用：

- `SessionPool` 写 `config.yaml` / `.env`。
- `acpclient.Runner` 生成进程 `Env` 和 `cwd`。

本地 managed 的 `HERMES_HOME` 最终定为 Memoh app data 下的 per-bot 目录，所以解析
输入还需要拿到 local data root。当前 `WorkspaceInfo` 没有这个字段，Phase 2 要么扩展
workspace info，要么从 server runtime config 注入；不能让 config writer 和 Runner
分别猜路径。

### 工具审批是 managed 模式 blocker

Hermes 有自己的 file、terminal/process、browser、memory、skills、
`execute_code`、`delegate_task` 等工具面。只托管模型 key 不等于托管执行权限。

在开放 managed 模式前必须完成：

- 实测 Hermes ACP 的文件修改、终端命令、代码执行、浏览器动作是否会触发
  `session/request_permission`。
- smoke 要具体到协议事件：分别触发文件写入、shell 命令、`execute_code`，断言收到
  `session/request_permission`，并覆盖 deny 后不执行、allow 后才执行。
- 增加 Hermes `ToolQuirks` / permission mapping matrix。至少要把 Hermes 的 file
  write/edit、shell/process、`execute_code` 映射到 Memoh 统一的 `write`、`edit`、
  `exec` approval 和 stream event。映射不到的权限必须 fail closed，不能静默放行。
- Permission smoke 使用固定 fixture：一个只写临时文件的 prompt、一个只执行可观测
  no-op shell 命令的 prompt、一个 `execute_code` prompt；每个 fixture 都断言 canonical
  tool 名称、审批标题、deny/allow 后的状态变化。
- 还要验证 Hermes 没有通过内部 shell 或内部 process runner 绕过 Memoh 暴露的
  terminal/file capability。
- 如果 Hermes 有 session mode 或配置项可以固定权限策略，Memoh managed 模式要
  写入安全默认值，类似 Claude Code 托管 settings 的处理方式。
- 如果某些 Hermes 工具不会经过 ACP permission callback，要么禁用，要么把
  managed/container 支持延后。
- 增加 live smoke：危险终端命令必须 surfaced 到 Memoh 审批，而不是直接执行。

### 密钥和导出要单独设计

`Sensitive` 字段只解决 API response scrub，不解决备份和 workspace export。
Managed Hermes 会让 API Key 至少出现在两个位置：

- Bot metadata 的 `managed.api_key`。
- `HERMES_HOME/.env`。

现有 bot backup 会写 `bot/profile.json`，workspace export 还可能递归打包 `/data`。
所以开放 `api_key` 前必须补上接口级导出策略。第一版 contract 定为 **默认不导出 Hermes
managed secrets**：

- Bot backup/export 检测 ACP managed secrets，标记为敏感导出。
- `bot/profile.json` 中的 ACP sensitive managed fields 默认 scrub，不写明文 API key。
- import 后要求用户重新输入 Hermes API key。
- `WorkspaceData.ExportData` 增加 secret exclude policy；默认排除
  `.memoh-hermes/`、`.hermes/`、`.codex/` 下的 `.env` / `auth.json`
  和嵌套 `.env` / `auth.json`。
- 底层 `tarGzDir` / zip writer 增加 filter，不能只在上层文档里约定。
- 本地 managed 的 app-data `HERMES_HOME` 不走 workspace archive。import 后要重建一个空的
  per-bot Hermes home，标记 key 缺失，等用户下一次保存 managed config 时重新写入
  `config.yaml` / `.env`。
- backup manifest 要记录 Hermes secret 已被 scrub，UI/import 结果里显示“需要重新填写
  Hermes API key”。
- 第一版不做“完整 secret export”。如果未来需要完整导出，必须先有 passphrase 加密或
  明确危险确认。
- 长期建议把 managed secret 放到统一 secret store，metadata 只保存引用。

## 产品形态

### Phase 1：`local + self`

第一版语义很简单：Memoh 启动本地 `hermes-acp`，Hermes 使用用户已经配置好的
profile。Memoh 不写 Hermes 配置文件，也不承诺 Hermes memory、skills、sessions、
logs、cron、auth 状态按 Bot 隔离。

适合用户：

- 已经安装 Hermes。
- 已经运行过 `hermes setup`、`hermes model` 或手动维护 `HERMES_HOME`。
- 明白宿主 `~/.hermes` 可能被多个 Bot 共享。

Profile 建议：

```go
func hermesProfile() Profile {
    return Profile{
        ID:                AgentHermesID,
        DisplayName:       AgentHermesName,
        Description:       "Hermes Agent ACP adapter",
        LocalCommand:      "hermes-acp",
        SupportedBackends: []string{"local"},
        SetupModes:        []string{setupModeSelf},
    }
}
```

前端显示文案要说明：self 模式使用当前运行环境中的 Hermes profile，不由 Memoh
隔离或管理。这个 warning 不能只放在详情页；列表页直接启用 Hermes self 时也必须
出现不可跳过的确认，避免用户绕过风险提示。

### Phase 3：managed `api_key`

Managed 模式由 Memoh 为 Bot 创建最小 Hermes profile。它不暴露 Hermes 全量配置，
只提供足够启动 ACP 编程任务的模型和 Provider 配置。

第一版 managed 字段建议：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `provider` | text 或 select | 是 | 第一版只允许明确支持的 provider。不要做隐藏默认值。 |
| `model` | text | 是 | Hermes 模型名。 |
| `base_url` | url | custom 必填 | OpenAI-compatible endpoint。 |
| `api_key` | password | 是 | 写入 `.env`，不作为进程参数传递。 |
| `max_tokens` | text / number | 否 | 内部可覆盖；第一版 UI 可不暴露，未设置时不写入 Hermes 配置。 |

第一批 managed provider 只建议承诺：

| Provider | `.env` Key | 配置行为 |
| --- | --- | --- |
| `openrouter` | `OPENROUTER_API_KEY` | `model.provider: openrouter` |
| `openai-api` / `openai` | `OPENAI_API_KEY` | `model.provider: openai-api`，`openai` 仅作为兼容输入别名 |
| `gemini` / `google` / `google-ai-studio` | `GOOGLE_API_KEY` | `model.provider: gemini`，使用 Hermes native Gemini adapter |
| `custom` | `MEMOH_HERMES_API_KEY` | `model.provider: custom:memoh-managed`，`providers.memoh-managed.key_env` 指向该 key，要求 `base_url` |

`anthropic` 等 Provider 可以作为后续阶段。原因是 Hermes Python extras、provider SDK、
env key 兼容和离线打包都要分别验证；不能只在 UI 里列出来。

Managed 模式开放后的 Profile 才增加：

```go
ManagedFields: []ManagedField{
    {ID: "provider", Label: "Provider", Type: "text", Required: true, Placeholder: "openrouter"},
    {ID: "model", Label: "Model", Type: "text", Required: true, Placeholder: "anthropic/claude-sonnet-4"},
    {ID: "base_url", Label: "Base URL", Type: "url", Placeholder: "https://api.example.com/v1"},
    {ID: "api_key", Label: "API key", Type: "password", Required: true, Sensitive: true},
},
SetupModes: []string{setupModeSelf, setupModeAPIKey},
```

如果前端要把 `provider` 做成下拉，`ManagedField` schema 需要支持 `options`。
如果 `custom` 时要求 `base_url`，schema 还需要支持条件校验，或者前端做
Hermes-specific validation。

### Phase 4：container

容器模式只有在 toolkit 可以离线运行 Hermes 后才开放：

```go
Command:           "hermes-acp",
SupportedBackends: []string{"local", "container"},
```

不要让生产容器工作区静默依赖运行时网络安装。运行时 `uvx` fallback 只能作为显式
调试开关，例如 `MEMOH_HERMES_ALLOW_NETWORK_INSTALL=1`。

## `HERMES_HOME` 策略

Managed 模式推荐路径：

- 容器工作区：`/data/.memoh-hermes`
- 本地工作区：Memoh app data 下的 per-bot 目录，例如
  `<local-data-root>/acp/hermes/<bot-id>`

本地 managed 不应默认写 `<workspaceRoot>/.memoh-hermes`，因为这会把 `.env`、
logs、sessions、skills 和 memory 放进用户项目目录，容易被 `git add`、项目搜索、
压缩包或 workspace export 带走。

workspace fallback 只允许作为显式 debug / migration 开关，不作为产品默认路径。如果受
当前 local bridge 限制必须临时使用 workspace root，至少要做这些保护，并且不能开放给
普通 managed UI：

- 自动把 `.memoh-hermes/` 加入 `.git/info/exclude`。
- 文件浏览 UI 默认隐藏或标记该目录。
- workspace export 默认排除 `.memoh-hermes/`、`.hermes/`、`.codex/` 下的
  `.env` / `auth.json` 等 secret 文件。
- 文档和 UI 明确提示这是 Bot 状态目录，不属于用户项目源码。

Self 模式默认不写 `HERMES_HOME`，但必须显示风险提示。后续可以增加
`hermes_home` 可选字段，让用户显式选择隔离目录。

## Managed 配置渲染

建议文件：

```text
internal/acpclient/hermes_config.go
internal/acpclient/hermes_config_test.go
```

输入结构：

```go
type HermesManagedConfig struct {
    Managed map[string]string
}
```

派生字段：

- `Provider`：归一化 `managed["provider"]`，第一版不做隐藏默认值。
- `Model`：`managed["model"]`。
- `BaseURL`：`managed["base_url"]`。
- `APIKey`：`managed["api_key"]`。
- `EnvKey`：根据 Provider 推导的 secret key。
- `MaxTokens`：`managed["max_tokens"]`，为空时不写入。

校验规则：

- `api_key` 模式必须有 `api_key`。
- `api_key` 模式必须有 `provider`。
- `api_key` 模式必须有 `model`。
- `provider == "openai"` 作为兼容别名接受，写入时归一化为 `openai-api`。
- `provider == "google"` / `"google-gemini"` / `"google-ai-studio"` 作为
  `gemini` 兼容别名接受，写入时归一化为 `gemini`。
- `provider == "custom"` 时必须有 `base_url`。
- `max_tokens` 如果传入，必须是正整数。
- 第一版拒绝未知 Provider。要支持任意 Provider，必须先增加显式
  `api_key_env` 或 native config 模式。
- `.env` 输出必须做 dotenv quoting，不能用简单字符串拼接。
- YAML 输出使用 Hermes 官方文档确认的主模型 schema：`model.provider`、
  `model.default`，显式传入 `max_tokens` 时才写 `model.max_tokens`；`custom`
  额外写 `providers.memoh-managed`，用 `key_env` 绑定 `.env` 中的
  `MEMOH_HERMES_API_KEY`。真实 provider/fake provider smoke 仍应用来验证 session
  能启动和发起最小请求。

标准 Provider 的 `config.yaml` 示例：

```yaml
model:
  provider: openrouter
  default: anthropic/claude-sonnet-4
```

Google AI Studio 的 `config.yaml` 示例：

```yaml
model:
  provider: gemini
  default: gemini-3.5-flash
```

Custom endpoint 的 `config.yaml` 示例使用 Hermes named custom provider 形态：

```yaml
model:
  provider: custom:memoh-managed
  default: my-model
providers:
  memoh-managed:
    name: Memoh Managed
    base_url: https://example.com/v1
    key_env: MEMOH_HERMES_API_KEY
    default_model: my-model
    api_mode: chat_completions
```

`.env` 示例：

```text
OPENROUTER_API_KEY=...
GOOGLE_API_KEY=...
MEMOH_HERMES_API_KEY=...
```

写文件策略：

- 容器工作区通过 workspace bridge 写 `/data/.memoh-hermes`。
- 本地 app data 路径由 server 直接写，或扩展 local bridge 支持安全写入；不把 managed
  secret 默认写进用户项目目录。
- 输出内容要确定性，方便测试。
- 密钥写入 `.env`，不要进入 `StartRequest.Env`。

验证策略：

- 增加 canary：用生成的 `HERMES_HOME` 运行 `hermes-acp --check` 或等价命令。
- 如果 Hermes 没有稳定 check 命令，至少做 ACP `initialize` + `session/new`
  smoke。
- `custom` provider 的 `base_url` 必填；OpenAI-compatible 中转明确写
  `api_mode: chat_completions`。如果后续支持 Anthropic Messages 或 Codex
  Responses 形态，再用 targeted smoke 追加可配置字段。
- 官方 Gemini provider 需要做 live smoke，验证 `GOOGLE_API_KEY` 写入 `.env`
  后 Hermes 能通过 native Gemini adapter 完成 ACP `initialize`、`session/new`
  和最小 prompt。
- 不为了第三方中转的非标准行为修改默认请求参数；`max_tokens` 只作为显式覆盖项。

## Runtime 环境变量

扩展 `internal/acpclient.processOptions` 相关逻辑，为 Hermes managed 模式注入
`HERMES_HOME`。

规则：

- 非 Hermes Agent 返回现有逻辑。
- Hermes self 模式不写 managed config。本地 self 默认不注入 `HERMES_HOME`，直接使用宿主
  Hermes profile；容器 self 会设置 `HERMES_HOME=/data/.hermes`，给容器内已有 profile 一个
  稳定持久路径。
- 容器 managed 模式返回 `/data/.memoh-hermes`。
- 本地 managed 模式使用 `ResolvedSessionContext.HermesHome`，路径是 Memoh app data 下的
  per-bot 目录；workspace fallback 只有显式 debug/migration 开关才能启用。
- 容器 managed Hermes 不进入临时 `HOME=/tmp/memoh-acp/<uuid>` 分支。

Managed Hermes 必须补充 bridge-level 环境清理能力，这是开放 managed 的硬性前置项。
现在 bridge 执行命令时会继承 `os.Environ()` 再 append Memoh 传入的 env，单纯不传
API Key 不能保证干净。需要在 workspace bridge exec/pipe 协议中增加 `CleanEnv` 或
`UnsetEnv` 语义，并覆盖：

- ACP process 启动。
- `resolveCommand` / wrapper 探测路径。
- Hermes 后续通过 ACP terminal capability 发起的 shell/process tool。

协议迁移要写清楚，不能只改 `processOptions`：

- 在 `internal/workspace/bridgepb/bridge.proto` 的 `ExecInput` 增加向后兼容字段，例如
  `clean_env` 和 `unset_env`。
- 重新生成 bridge pb，并更新 bridge server exec 和 pipe 两条路径。
- bridge client 从裸 `[]string` env 迁移到 `ExecOptions`，保留旧调用默认行为。
- 现有 Codex/Claude callsite 默认不设置 `CleanEnv`，行为不变。
- Hermes managed ACP process、Hermes wrapper probe 和 terminal tool 明确设置
  `CleanEnv`/`UnsetEnv`。
- 增加 bridge 单测证明继承自 `os.Environ()` 的 key 会被移除。

Managed Hermes 启动前应清理：

- `HERMES_HOME`
- `HERMES_*`
- `MEMOH_HERMES_API_KEY`
- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `ANTHROPIC_API_KEY`
- `GOOGLE_API_KEY`
- `GEMINI_API_KEY`
- 其他明确支持 Provider 的 key

Terminal tool 也要使用同一份 sanitized base env；否则 ACP 进程本身干净，但 Hermes
调用终端工具时仍可能继承 Memoh server、用户 shell 或容器镜像中的 Provider Key。
此外，ACP agent 通过 terminal request 传入的 `p.Env` 不能重新覆盖这些 key。规则定为：
`HERMES_HOME`、`HERMES_*` 和已支持 Provider key 在 `p.Env` 中一律拒绝或过滤，并记录
debug log；不允许 agent 显式把这些变量改回去。没有这个能力时，不开放 Hermes managed。

## SessionPool 启动流程

在 `reconcileManagedCodexConfig` 附近增加 Hermes reconciliation，但不要复制路径
计算逻辑：

```go
func (p *SessionPool) reconcileManagedHermesConfig(
    ctx context.Context,
    botID string,
    profile acpprofile.Profile,
    setup acpprofile.AgentSetup,
    mode acpclient.SetupMode,
) error
```

规则：

- 不是 `acpprofile.AgentHermesID` 直接返回 nil。
- self 模式直接返回 nil，除非后续实现 `hermes_home` 初始化。
- managed 模式先调用导出的 `acpclient.ResolveSessionContext`，得到同一组 canonical
  paths 和 `HermesHome`。
- 在启动 ACP 进程前写入 Hermes managed `config.yaml` 和 `.env`。
- 写入后的路径必须和 Runner 运行时注入的 `HERMES_HOME` 完全一致。
- `Runner.StartRequest` 带上同一个 `ResolvedSessionContext`；Runner 使用它生成 cwd 和 env，
  不再为 Hermes managed 单独解析一次。

调用位置建议在 Codex 配置 reconciliation 之后、`managedProcessEnv` 之前。

还要处理配置变更后的 runtime 生命周期：

- Bot ACP metadata 修改后，如果 Hermes runtime 已经启动，需要 invalidate 或 restart。
- 建议在 `acpagent.SessionPool` 增加 `CloseBotAgentRuntimes(botID, agentID)` 或按
  config fingerprint 自动替换 runtime。
- `runtimeHandle` 增加 config fingerprint，reuse 条件从 `agentID + projectPath` 扩展为
  `agentID + projectPath + configFingerprint`。
- handler/service 需要注入一个 runtime closer，例如
  `CloseBotAgentRuntimes(botID, agentID)`，在 create/update/import/OAuth 回调写入新
  metadata 或 secret 后关闭旧 runtime。
- `UsersHandler.prepareACPWorkspaceConfig` 需要从 Codex 专用逻辑改为遍历 enabled ACP
  agents，按 agent 调用对应 preparer。
- managed ACP 保存路径改为可失败的保存事务：保存前先用候选 metadata 做 profile/backend
  校验和 managed field 校验；create 时先分配 botID 并准备 workspace/config，失败则不创建
  Bot；update 时先基于候选 metadata 写入/验证 config，失败则保留旧 metadata 并返回错误；
  成功后再关闭旧 runtime。不能继续使用“先保存 metadata，config 写失败只 log”的行为。
- 如果短期只在 runtime start 写配置，UI 必须明确提示“保存后需要重启 ACP runtime”，
  但这不是 managed 模式的推荐最终行为。

`validateManagedFields` 可以复用 Profile 中 `Required` 字段的通用校验，但需要额外
校验 `provider == custom && base_url == ""`。如果条件校验放在后端，前端也应有
相同提示，避免提交后才失败。

## ACP Profile 定义

增加常量：

```go
AgentHermesID   = "hermes"
AgentHermesName = "Hermes"
```

Phase 1 Profile：

```go
func hermesProfile() Profile {
    return Profile{
        ID:                AgentHermesID,
        DisplayName:       AgentHermesName,
        Description:       "Hermes Agent ACP adapter",
        LocalCommand:      "hermes-acp",
        SupportedBackends: []string{"local"},
        SetupModes:        []string{setupModeSelf},
    }
}
```

Phase 3/4 再增加 managed fields、container command 和更多 backend/mode。

如果后续前端支持带 options 的 `ManagedField`，可以把 `provider` 从文本字段升级成
下拉。当前 schema 没有 options、默认值、条件校验和本地化 key，是否扩展 schema
是 API/SDK 的一个明确决策点。

## Workspace Toolkit

容器路径应该和现有 ACP Agent 一样：用户不需要在每个 workspace image 里手动安装
Hermes。虽然 Hermes 是 Python 项目，不像 Codex/Claude Code 那样直接分发 npm/二进制
CLI，但仍然可以做成同样的 Memoh 使用体验：toolkit 内置一个 `hermes-acp` 命令，
workspace 里直接运行，不要求用户安装 Hermes。

当前结论：第一版 container/toolkit 支持采用 **pinned uv offline cache + wrapper**。
也就是在 toolkit 构建期把 pinned Hermes package 预热到 toolkit 内部的 uv cache/tool
目录，运行时 wrapper 把这些 seed 复制到可写 runtime cache，再执行
`uv tool run --offline`。如果后续验证发现 uv cache relocation 不稳定，再升级为
sealed Python runtime + venv。

### 官方可复用分发形态

调研结果不是“完全没有 binary”，而是没有看到适合直接下载进 toolkit 的
`hermes-acp-linux-amd64` 这类单文件 release asset。官方当前有三类分发路径：

- 标准 installer：`curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash`。
  这是 bootstrap installer，会安装 uv/Python/Node、clone repo、创建 venv、安装依赖
  并生成 `hermes` 命令，不是单文件二进制下载。
- Nix package：官方 Nix 文档说 `nix profile install github:NousResearch/hermes-agent`
  后会得到 `hermes`、`hermes-agent`、`hermes-acp`，且 Python 依赖由 uv2nix 固化，
  不需要 runtime pip/venv/npm。源码里的 Nix package 实际用 `makeWrapper` 包装
  sealed venv 里的 `hermes-acp` entrypoint。
- Docker image：官方 `nousresearch/hermes-agent` 镜像内有 `/opt/hermes` 不可变安装树
  和 `.venv`，并把 `/opt/hermes/.venv/bin` 放到 PATH。官方 Dockerfile 在构建期
  `uv sync` 和 `uv pip install -e "."`，运行时不应该再 lazy install 到 `/opt/hermes`。

这些事实说明：Hermes 官方自己也不是靠运行时 `uvx` 执行生产部署，而是把 Python
环境固化后再提供 wrapper。Memoh 应沿用这个方向。

### 方案对比

| 方案 | 可行性 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- | --- |
| runtime `uvx` | 可行但不稳 | 实现最少，贴近 ACP registry | 运行时可能下载 Python/包，依赖网络和可写 cache | 只做显式调试 fallback |
| wheelhouse + runtime install | 可行 | 可缓存 wheel | 运行时仍在安装，toolkit 只读时复杂，失败面大 | 不作为主线 |
| sealed Python runtime + venv + wrapper | 高 | 和官方 Docker/Nix 思路一致，运行时无 resolver/无网络 | 需要目标 glibc 环境构建和 live smoke | 后续升级备选 |
| 复制官方 Docker `/opt/hermes` | 中 | 官方已验证，依赖完整 | Debian trixie/bookworm、体积、路径和可裁剪性要验证 | 参考或临时验证 |
| Nix closure | 中 | 依赖闭包最确定 | `/nix/store` 绝对路径、体积和集成成本高 | 长期备选 |
| PEX/shiv/zipapp | 未验证 | 可能接近单文件 | Hermes 有 data files、插件、native deps、动态 lazy deps，偏离官方路径 | 不建议第一版 |

### 备选 sealed Python runtime 方案

本次实现先走 pinned uv offline cache。下面的 sealed runtime 方案保留为后续升级路径，
用于解决 uv cache relocation、glibc 兼容或只读 toolkit smoke 中发现的问题。
本节提到的 `hermes-toolkit-builder`、sealed venv 和 `MEMOH_HERMES_REQUIRE_TOOLKIT`
不是本次 PR 的验收门槛。

主线方案是“构建期缓存，运行时执行固定 Python runtime + venv”：

1. 新增 Debian bookworm/glibc Hermes packaging stage，在最终 canonical 路径
   `/opt/memoh/toolkit/hermes/...` 构建 artifact。当前 `toolkit-assembly` 是
   Alpine/musl，不能在这个 stage 里创建 Hermes venv。
2. Dockerfile 拓扑应明确为：
   - `toolkit-assembly` 继续负责 Node/npm ACP、display、uv 等现有 toolkit 内容。
   - `hermes-toolkit-builder` 使用 Debian bookworm/glibc，构建
     `/opt/memoh/toolkit/hermes/python` 和 `/opt/memoh/toolkit/hermes/venv`。
   - 最终 assembly 把 `hermes-toolkit-builder` 的 `/opt/memoh/toolkit/hermes`
     复制到 `/assembly/toolkit/hermes`。
   - `toolkit-acp-bridge-live` 和生产 workspace 都从 `/assembly/toolkit` 复制最终
     artifact。
3. pin 三个版本：
   - `HERMES_AGENT_VERSION`
   - `UV_VERSION`
   - `PYTHON_VERSION`，例如 Python 3.11 或 3.12，必须满足 Hermes
     `>=3.11,<3.14`
   同时记录 SHA256/digest：uv 下载包、Hermes wheel、Python runtime 来源都要可复现。
4. 使用 uv 把 Python 安装到最终 canonical 路径对应的目录。不要依赖构建机的
   `~/.local/share/uv/python`，否则 venv 复制到 runtime 后 `bin/python` 会指向不存在的
   构建机路径。构建 stage 里也使用 `/opt/memoh/toolkit/hermes/...` 这个最终路径创建
   artifact，避免 console script 和 venv shebang 记录错误前缀。
5. 使用固定 cache 目录加速构建，但 cache 只作为 build artifact 输入，不作为 runtime
   依赖：

```bash
UV_CACHE_DIR=/build/hermes-uv-cache \
  uv python install "$PYTHON_VERSION" \
    --install-dir /opt/memoh/toolkit/hermes/python

PYTHON_BIN="$(find /opt/memoh/toolkit/hermes/python -type f -path '*/bin/python3*' | head -n 1)"

uv venv /opt/memoh/toolkit/hermes/venv \
  --python "$PYTHON_BIN" \
  --relocatable \
  --link-mode copy

UV_CACHE_DIR=/build/hermes-uv-cache \
  uv pip install \
    --python /opt/memoh/toolkit/hermes/venv/bin/python \
    "hermes-agent[acp,mcp]==$HERMES_AGENT_VERSION"
```

6. build 完成后删除不需要的 build cache，把以下内容作为 toolkit artifact：
   - `$TOOLKIT/hermes/python`
   - `$TOOLKIT/hermes/venv`
   - `$TOOLKIT/hermes/version`
   - `$TOOLKIT/bin/hermes-acp`
7. toolkit wrapper 只执行固定入口，不调用 `uv`：

```bash
exec "$TOOLKIT/hermes/venv/bin/python" -m acp_adapter.entry "$@"
```

wrapper 还要固定关闭 lazy/runtime install 路径：

```bash
export HERMES_DISABLE_LAZY_INSTALLS=1
export UV_OFFLINE=1
export PIP_NO_INDEX=1
```

同时支持一个测试强制开关：

```bash
MEMOH_HERMES_REQUIRE_TOOLKIT=1
```

设置后，如果 sealed runtime 不存在，wrapper 直接失败，不允许 fallback 到用户 PATH。
这个开关用于 live smoke 和生产容器 managed 模式，防止 PATH 上碰巧有 `hermes-acp`
掩盖 toolkit artifact 缺失。设置后要禁止所有 fallback 分支：用户 PATH fallback、
glibc/musl guard fallback、debug `uvx` fallback 都不能执行。

8. 验证分为两个 gate，不能混在一起：
   - Offline runtime smoke：强制无网络，只验证 runtime 能启动和 ACP 握手。
   - Online provider smoke：有网络和真实 Provider Key，验证最小 prompt。
9. Offline runtime smoke：
   - 运行 `hermes-acp --version`。
   - 运行 ACP `initialize`。
   - 运行 ACP `session/new`。
   - 使用 `docker run --network none`。
   - 不复用依赖 host TCP 连接的 `toolkit-acp-bridge-live` harness；`--network none`
     下 host 无法连进容器。需要新增 container-internal stdio ACP harness，或用 override
     entrypoint 在容器内直接驱动 `hermes-acp` stdio。
   - 设置 `MEMOH_HERMES_REQUIRE_TOOLKIT=1`，且不设置
     `MEMOH_HERMES_ALLOW_NETWORK_INSTALL`。
   - 清空或指向临时空目录的 `UV_CACHE_DIR`，并断言未写入。
   - toolkit 以只读方式挂载或在测试前后 checksum 对比，断言没有写入 toolkit 安装树。
   - 断言实际 `sys.executable` 在 `$TOOLKIT/hermes/venv/bin/python` 下。
10. Online provider smoke：
   - 用 managed `HERMES_HOME` 和测试 key 跑最小 prompt。
   - 也可以用本地 fake OpenAI-compatible server 做无外网 prompt smoke，但这应作为
     单独测试，不和 offline runtime smoke 混淆。

本次实现采用折中方案：toolkit assembly 阶段用 pinned `uv tool run --from
"hermes-agent[acp,mcp]==<version>" hermes-acp --help` 预热 `uv-cache`、`uv-tools` 和
Python 安装目录；运行时 wrapper 默认只使用 `uv tool run --offline`。这样不要求
Hermes 提供单文件 binary，也不让生产容器在正常路径上联网安装。

这个方案和现有 npm ACP 的差异只是 artifact 形态不同：

- npm ACP：`$TOOLKIT/acp/lib/node_modules/...` + `$TOOLKIT/bin/<agent>`。
- Hermes ACP：`$TOOLKIT/uv` + `$TOOLKIT/uv-cache` + `$TOOLKIT/uv-tools` +
  `$TOOLKIT/python` + `$TOOLKIT/bin/hermes-acp`；运行时复制到
  `${MEMOH_HERMES_RUNTIME_DIR:-${XDG_CACHE_HOME:-/tmp}/memoh-hermes-acp/<version>}` 后启动。

对 `acpprofile.Profile` 和 `acpclient.resolveCommand` 来说，两者都只是
`/opt/memoh/toolkit/bin` 里的一个命令。

长期如果 `uv tool run --offline` 的缓存可迁移性不足，再升级到 sealed Python runtime
+ venv：在目标 glibc runtime 上构建 Hermes venv，复制到 toolkit，并让 wrapper 直接
执行 venv 里的 entrypoint。这个升级不改变 Profile、runtime、permission 和 managed
config 设计。

需要 pin：

- `HERMES_AGENT_VERSION`。
- `UV_VERSION`，不能依赖安装时的 latest。
- `PYTHON_VERSION`。
- 版本检查，确保 pin 变化时会重新构建 artifact。
- 把 Hermes 版本写进 wrapper 或 `$TOOLKIT/hermes/version`，因为 build-time shell
  变量不会自动存在于 runtime。

新增 wrapper：

```text
docker/toolkit/bin/hermes-acp
```

推荐 wrapper 行为：

1. 只使用 toolkit 内置 `uv`、`uv-cache`、`uv-tools` 和 Python 安装目录作为 seed。
2. 运行前复制 seed 到可写 runtime 目录。
3. 默认运行 `uv tool run --offline --from "hermes-agent[acp,mcp]==<pinned>" hermes-acp`。
4. 默认不 fallback 到 PATH 上的 `hermes-acp`，避免容器 managed 模式被镜像里的偶然安装掩盖。
5. 默认不运行 `uvx` 联网安装或 runtime install。
6. 只有设置 `MEMOH_HERMES_ALLOW_NETWORK_INSTALL=1` 时，才允许 pinned package 运行时联网安装。
7. 全部不可用时输出清晰错误。
8. 和现有 wrapper 一样设置 toolkit CA bundle。

需要验证的打包问题：

- 当前 toolkit assembly 在 Alpine 阶段运行。Hermes sealed Python runtime + venv
  必须挪到 glibc packaging stage，或用目标 runtime image 构建。
- macOS 开发机不能直接执行 Linux Python runtime。`install-workspace-toolkit` 会先生成
  通用 toolkit，再用 Debian 容器补 Hermes glibc/manylinux cache；生产 Dockerfile 也有
  `toolkit-assembly-glibc` stage 负责同一件事。没有 Docker 的本地环境只能等待 CI 或
  glibc host 生成 Hermes cache。
- `hermes-agent[acp,mcp]` 可能不包含所有 Provider extras。离线 managed 第一批只承诺
  `openrouter`、`openai-api`、`custom`，并接受 `openai` 作为 `openai-api`
  的兼容别名；其他 Provider 要单独验证。
- `hermes-agent[acp,mcp]` 会安装 Hermes core、ACP adapter 和 MCP SDK 依赖；这已经
  包含 OpenAI-compatible provider 路径，但 native Anthropic、Bedrock、Azure 等
  extras 不在第一批承诺里。
- 第一版 runtime deps 范围定义为“ACP adapter + OpenAI-compatible coding path”。
  `git`、`ssh`、`rg` 依赖 workspace image 现有工具或 Memoh 已有 toolkit；Hermes browser
  tools、Playwright、Node、ffmpeg 不纳入第一批承诺。
- relocation 验收要检查 `uv tool run --offline` 不依赖构建目录或网络，并且 runtime
  writes 只发生在 wrapper 指定的可写 runtime cache 目录。
- `linux/amd64` 和 `linux/arm64` 都要构建并跑 offline runtime smoke。

验收标准：在 toolkit assembly 后，用 container-internal harness 和 `--network none`
仍能运行基础 ACP：

```bash
hermes-acp --version
```

完整验收必须同时包含：

- ACP `initialize` 成功。
- ACP `session/new` 成功。
- 禁用运行时联网安装时仍然可运行。
- toolkit 安装树前后 checksum 不变。
- `linux/amd64` 和 `linux/arm64` 都通过 Docker buildx 构建和 offline runtime smoke。

`hermes-acp --check` 如果不是稳定公开命令，就不要把它作为唯一验收标准。
Hermes 自带 browser tools 可能还依赖 Node/Chromium，不纳入第一批离线承诺。

### 当前阶段需要确认的 toolkit 决策

当前阶段要确认的不是“有没有 Hermes 二进制”，而是 **pinned uv offline cache
是否能作为 Memoh toolkit 的稳定内置 artifact**。确认标准：

- toolkit assembly 阶段必须能把 pinned Hermes package 缓存到 `$TOOLKIT/uv-cache` /
  `$TOOLKIT/uv-tools` / `$TOOLKIT/python`。
- macOS 本地 `install.sh` 不执行 Linux uv；container Hermes toolkit artifact 需要在 Linux
  Docker/CI 构建环境中生成。
- `docker/toolkit/bin/hermes-acp` 默认只走 offline cache，内置缺失时失败，不找用户 PATH。
- 断网 container-internal harness 下 ACP `initialize` 和 `session/new` 成功；最小 prompt
  放到本地 fake OpenAI-compatible smoke、nightly/manual real provider smoke。
- 如果 uv cache relocation 或 glibc 兼容失败，再评估 sealed venv、Nix closure 或官方
  Docker 安装树提取。

## 前端影响

现有 ACP 配置 UI 基本 schema-driven，但 Hermes 需要补几个能力点，不能只加图标。

必须处理：

- 当前终态中 Hermes Profile 和现有 Codex/Claude Code 一样支持 `local` / `container`，
  因此前端不需要 Hermes-only backend 过滤。后续如果新增 local-only 或 container-only
  ACP Profile，backend 过滤应提升成通用 ACP UI 能力，避免 UI 隐藏集合和 metadata 保存
  集合不一致。
- 第一版 managed UI 不扩 OpenAPI schema，采用 Hermes-specific 前端校验：
  - `provider` 仍使用现有文本字段，前端校验只允许
    `openrouter`、`openai`、`openai-api`、`custom`。
  - `custom` 时 `base_url` 必填。
  - `openrouter/openai/openai-api` 时 `base_url` 可空。
  - 未支持 Provider 不展示。
- settings 和 onboarding 不能各自手写一套 Hermes 表单逻辑。当前实现把 metadata
  读取、序列化和 managed 条件校验放在 `apps/web/src/utils/acp/metadata.ts`，
  两个入口应复用这套 helper；如果后续要做 provider select，再新增共享 field-view helper。
- 后续如果要把 provider options/default/required_when 做成通用能力，再扩
  `ManagedField` schema 并重新生成 SDK。
- Managed field 的 `Label`、`Placeholder`、`Help` 目前来自后端英文字符串，前端会
  直接渲染。中文/日文 UI 要么扩展 schema 增加 `label_key/help_key/placeholder_key`，
  要么前端按 `agent_id + field_id` 映射本地化文案。第一版采用
  `agent_id=hermes + field_id` 的本地映射，覆盖 `en/zh/ja`。
- profile display name、description 和 self warning 也要走本地映射。否则设置列表和
  onboarding 仍会混入后端英文文案。
- Onboarding 和 Bot ACP 设置页都要有相同校验。
- self 模式 UI 要明确提示使用宿主 Hermes profile，可能共享 `~/.hermes` 中的密钥、
  memory、skills、sessions、logs 和 cron。这个 warning 要覆盖 Bot 设置页和 onboarding。
- Bot ACP 列表页的 enable switch 不能绕过 self warning。Hermes self 首次启用必须进入
  detail/确认弹窗，或在列表行显示不可跳过的 warning 后才能持久化。
- 后续如果要支持 self 模式的 `hermes_home` 字段，需要先支持 self-mode fields；当前
  前端只在 `api_key` 模式展示和保存 managed fields。

建议测试场景：

- 如果后续出现 backend 受限的 ACP Profile，Onboarding 和 Bot 设置页要按 backend 做通用
  过滤/禁用；当前 Hermes 终态同时支持 local/container，不需要 Hermes-only 特判。
- Hermes `self` 保存后 metadata 中 `managed` 为空。
- Hermes `api_key` 写入 `provider/model/base_url/api_key`。
- `custom` 缺少 `base_url` 时前端阻止提交。
- settings 和 onboarding 使用同一个 ACP metadata validator。
- 从 onboarding 选择 Hermes 后，`?acp=hermes` 能进入对应 session 路径。
- API response 中 password 字段显示 scrubbed 值，保存时能保留旧值。

第一版不要加 Hermes 专属 OAuth 按钮。

## API 和 SDK

新增 Profile 会改变 `/acp/profiles` 返回内容。如果只做 Phase 1，不需要新增 REST
endpoint，也不一定需要扩展 OpenAPI schema。

如果要支持以下能力，则需要明确走 OpenAPI/SDK 变更：

- `ManagedField.options`
- `ManagedField.default`
- 条件校验，例如 `required_when`
- 本地化 key，例如 `label_key`、`help_key`、`placeholder_key`
- backend/mode feature gate 元信息

第一版 Hermes managed 不依赖这些 schema 扩展；它使用前端 Hermes-specific 映射和后端
校验。这样可以避免为了一个 Provider select 先扩大 `/acp/profiles` 合同。

这些变更完成后需要运行：

```bash
mise run swagger-generate
mise run sdk-generate
```

## Metadata 形态

Self 模式示例：

```json
{
  "acp": {
    "agents": {
      "hermes": {
        "enabled": true,
        "setup_mode": "self",
        "managed": {}
      }
    }
  }
}
```

Managed 模式示例：

```json
{
  "acp": {
    "agents": {
      "hermes": {
        "enabled": true,
        "setup_mode": "api_key",
        "managed": {
          "provider": "openrouter",
          "model": "anthropic/claude-sonnet-4",
          "base_url": "",
          "api_key": "sk-or-..."
        }
      }
    }
  }
}
```

现有 metadata scrub/merge 逻辑应该会因为 `api_key` 字段标记为 `Sensitive` 而
自动遮蔽 API response，并在用户未重新输入密钥时保留旧值。但这不覆盖 backup 和
workspace export，必须另做导出策略。

## 测试计划

后端单元测试：

- Phase 1 Profile 列表包含 Hermes，但只声明 `local+self`。
- `/acp/profiles` safe metadata 继续拒绝泄漏 `hermes-acp`、`uvx`、
  `HERMES_HOME`、API Key env 名等实现细节。
- local self smoke 能用 PATH 上的 `hermes-acp` 完成 ACP `initialize`、`session/new` 和
  最小 prompt。
- PATH 上没有 `hermes-acp` 时错误可读，包含 Hermes 安装/配置提示。
- Hermes managed config renderer 输出确定性的 `config.yaml` 和 `.env`。
- `.env` 对特殊字符做正确 quoting。
- `provider == custom` 且缺少 `base_url` 时失败。
- 未知 Provider 在第一版失败。
- Hermes managed startup 注入 `HERMES_HOME`，但不把 API Key 放进
  `StartRequest.Env`。
- 容器 managed startup 不设置临时 `HOME=/tmp/memoh-acp/<uuid>`。
- self 模式不要求 managed fields，也不写 managed config。
- `ResolveSessionContext` 被 config writer 和 Runner 共用；Runner 收到 context 后不再重算
  Hermes managed cwd/home。
- metadata 修改后 Hermes runtime 会被 invalidate 或 restart。
- create/update/onboarding 中先做 Hermes managed 纯配置校验；update 只有 managed 配置字段
  确实变化时才写 workspace config，写入失败时请求返回错误且旧 metadata 不被覆盖。create
  在 bot 已创建后遇到 workspace 写入失败时记录并提示错误，避免请求重试造成重复 bot。
- local managed `HERMES_HOME` 使用 app data per-bot 目录，workspace fallback 只有显式
  debug 开关才能启用。

Process / bridge 测试：

- bridge 支持 `CleanEnv` / `UnsetEnv`，并能真正移除继承自 `os.Environ()` 的变量。
- bridge proto、client `ExecOptions`、server exec/pipe 路径都覆盖 clean/unset 语义；未设置
  clean/unset 的旧 callsite 行为不变。
- managed Hermes ACP process 清理继承的 `HERMES_HOME`、`HERMES_*` 和 Provider Key env。
- Hermes terminal tool 使用 sanitized base env，不重新泄漏 Provider Key。
- Hermes terminal request 中的 `p.Env` 不能覆盖 `HERMES_HOME`、`HERMES_*` 和 Provider Key。
- 现有 Codex/Claude 行为不变。
- 容器模式能从 toolkit path 解析到 `hermes-acp`。
- 未设置 `MEMOH_HERMES_ALLOW_NETWORK_INSTALL=1` 时 wrapper 不联网安装。

Backup / export 测试：

- 含 Hermes managed secret 的 bot export 被标记为敏感。
- `bot/profile.json` 默认 scrub ACP sensitive managed fields，不明文泄漏 API key。
- `WorkspaceData.ExportData` 的 exclude/options 被调用，底层 tar/zip filter 不写 secret path。
- workspace export 默认排除 `.memoh-hermes/`、`.hermes/`、`.codex/` 下的
  `.env` / `auth.json`。
- local managed app-data `HERMES_HOME` 不进入 workspace archive。
- import 后重建空 Hermes home，Hermes managed API key 为空，需要用户重新输入。

Toolkit 测试：

- toolkit assembly 在 Linux 构建 pinned Hermes uv offline cache；macOS 本地 `.toolkit-test`
  不被当作 Hermes Linux artifact 验收。
- toolkit 产物包含 `$TOOLKIT/uv`、`$TOOLKIT/uv-cache`、`$TOOLKIT/uv-tools`、
  `$TOOLKIT/python` 和 `$TOOLKIT/bin/hermes-acp`。
- wrapper 在缺包时输出清晰错误。
- wrapper 正常路径复制 seed 到 writable runtime dir 后执行 `uv tool run --offline`，不运行 network install。
- 未设置 `MEMOH_HERMES_ALLOW_NETWORK_INSTALL=1` 时不会 fallback 到 PATH 或 network install。
- container-internal harness 用 `docker run --network none` 跑 offline runtime smoke：
  `hermes-acp --version`、ACP `initialize`、ACP `session/new` 成功。
- offline smoke 前后 toolkit checksum 不变，uv/cache 临时目录不产生运行时安装内容。
- Docker buildx 的 `linux/amd64` 和 `linux/arm64` 都通过 toolkit build + offline runtime smoke。
- relocation 检查 `uv tool run --offline` 不依赖构建目录或网络。
- optional provider extras 不存在时，UI 不展示对应 Provider。

Provider smoke：

- Phase 3 前置是 local managed smoke：写入 app-data `HERMES_HOME`，使用本地 fake
  OpenAI-compatible server，启动 Hermes ACP，发送短 prompt，断言回复包含 marker。
- 真实 provider smoke 用开关控制，例如 `MEMOH_LIVE_HERMES_ACP=1`、
  `MEMOH_LIVE_HERMES_PROVIDER=gemini`、`MEMOH_LIVE_HERMES_MODEL`、
  `MEMOH_LIVE_HERMES_API_KEY`；只有 `custom` provider 需要
  `MEMOH_LIVE_HERMES_BASE_URL`。该测试放到 manual/nightly。
- Phase 4 前置是 container toolkit provider smoke：启动 `toolkit-acp-bridge-live`，
  写入容器内 Hermes managed config，通过 `acpclient.Runner` 发送短 prompt，断言回复
  包含 marker。

Permission smoke：

- 触发文件写入、shell 命令、`execute_code` 三类危险操作。
- 每类都断言出现 `session/request_permission`。
- 断言 Hermes permission event 被 `ToolQuirks` / mapping matrix 映射成 Memoh canonical
  `write/edit/exec` approval 和 stream event。
- deny 后断言操作没有执行。
- allow 后断言操作才执行。
- 断言没有绕过 Memoh terminal/file capability 的内部 shell 路径。

前端测试：

- ACP metadata 表单能处理 Hermes self。
- managed 打开后能处理 `provider/model/base_url/api_key`。
- `provider` 文本字段只接受 `openrouter/openai/openai-api/custom`，其中 `openai`
  作为后端兼容别名。
- `custom` 缺少 `base_url` 时前端阻止提交。
- settings 和 onboarding 使用同一个 ACP metadata validator。
- self 模式跳过 managed 字段校验。
- self 模式展示共享宿主 Hermes profile 的风险提示。
- 如果后续出现 backend 受限的 ACP profile，backend 过滤不会把未支持的 profile
  暴露给用户。
- Hermes profile 名称、描述、字段文案和 warning 后续可本地化。

## Rollout 计划

下面的 Phase 是完整实现的交付顺序，不是能力取舍。Phase 1 先让用户能跑通 local self，
Phase 2/3/4 继续补齐 managed、权限、安全、导出和 toolkit 容器支持，直到达到和
Codex/Claude Code 同级的一等 ACP Agent。

Phase 1：Profile 和 local self。

- 增加 Hermes Profile。
- 增加前端显示名/图标 fallback。
- UI 提示 self 模式使用宿主 Hermes profile，Memoh 不隔离 `~/.hermes`。
- 本地工作区可以运行用户已安装的 `hermes-acp`。
- 完成 local self 端到端 smoke：创建 local workspace bot、启用 Hermes self、启动 ACP
  session、完成 `initialize`、`session/new`、发送最小 prompt 并收到 Hermes 回复。
- PATH 上没有 `hermes-acp` 时，Bot 设置页或 session 启动错误必须明确提示如何安装/配置
  Hermes，不能表现为 generic process failed。
- 如果 Hermes self 的危险工具 permission 不能稳定 surfaced 到 Memoh approval，Phase 1
  必须在 UI 中标注 external profile 风险，并且不能把它描述为 managed/sandboxed。
- 不改 toolkit。
- 不开放 managed/container。

Phase 2：managed config 隐藏实现和 smoke gate。

- 增加 `ResolveSessionContext`，保证 config writer 和 Runner 使用同一组 canonical paths
  和 `HERMES_HOME`。
- 本地 managed home 定为 Memoh app data per-bot 目录。
- 增加 managed config renderer。
- 增加 bridge `CleanEnv` / `UnsetEnv` proto、client `ExecOptions`、server exec/pipe 能力，
  并覆盖 ACP process 和 terminal tool。
- 增加 terminal request env denylist，禁止 Hermes 和 Provider key 被 `p.Env` 覆盖。
- 增加 backup/export secret 策略：默认 scrub metadata secret，workspace export/tar filter
  排除 Hermes secret 文件，app-data home import 后重建并要求重新输入 API key。
- 增加 `CloseBotAgentRuntimes(botID, agentID)` 和 config fingerprint runtime 替换机制。
- 泛化 `UsersHandler.prepareACPWorkspaceConfig`，按 enabled ACP agents 调对应 reconciler。
- 改造 create/update/onboarding 保存语义：先做纯配置校验；update 的 managed config 写入
  失败时请求失败且旧 metadata 不被覆盖；create 在 bot 已创建后的 workspace 写入失败不能
  回滚成孤儿 bot，需要返回可见提示并允许设置页重写。
- 完成 Permission smoke，证明 Hermes 工具会经过 ACP permission callback。
- 完成 local managed fake-provider smoke，证明保存 config 后可以启动并完成最小 prompt。
- 增加 runtime invalidation/restart。
- 这个阶段不向 UI 开放 `api_key`。

Phase 3：打开 managed `api_key` feature gate。

- 开放 `openrouter`、`openai-api`、`custom`，并接受 `openai` 作为兼容别名。
- 前端使用 Hermes-specific provider 校验和条件校验；provider select 等通用 schema
  能力留到后续扩展。
- 后端开启 Profile `setupModeAPIKey`。
- 只在 Phase 2 所有 smoke 和安全测试通过后开放。

Phase 4：Toolkit 容器支持。

- 在 Docker build 参数 / helper 中 pin Hermes 版本。
- toolkit install 阶段预热 `$TOOLKIT/uv-cache`、`$TOOLKIT/uv-tools` 和 `$TOOLKIT/python`。
- 增加 `hermes-acp` wrapper，默认只执行 pinned offline `uv tool run`。
- macOS dev host 路径不直接当作 Linux artifact 验收，Linux artifact 仍需 Docker/CI smoke。
- 验证 uv offline cache relocation、`linux/amd64`/`linux/arm64` 断网启动。
- 增加 container-internal offline runtime smoke、本地 fake provider smoke 和
  manual/nightly real provider smoke。
- 开放 `SupportedBackends: ["local", "container"]`。

Phase 5：高级配置。

- 增加可选 raw/native `config.yaml` 导入或编辑。
- 为 `ManagedField` schema 增加 provider options、默认值、条件校验和 i18n key。
- 考虑 self 模式的 `hermes_home` override。
- 验证并开放更多 Provider，例如 Anthropic、Gemini。
- 只有当 Hermes OAuth / Portal 有清晰的非交互式流程并适合 Memoh Web UI 时，再考虑
  接入。

## 待确认问题

- 第一版应该 pin 哪个 Hermes 版本？调研时上游 registry 使用 `0.17.0`，实现前要再
  查一次最新稳定版本。
- Hermes 是否有稳定的安全 session mode 或配置项可以强制所有危险操作走 ACP
  permission callback？
- Hermes `custom` provider 当前按 named provider + `key_env` + `api_mode:
  chat_completions` 落地；后续是否开放用户自选 `api_mode` 仍待产品决策。
- 是否允许高级用户提供任意 `api_key_env`？这能支持更多 Provider，但会增加校验和
  文档负担。
- uv 的 pin 版本如何做跨平台预热和离线迁移？是否需要专门的 glibc build stage？

## 主要风险

- Python 打包可能没有现有 npm ACP 包那么直接。承诺容器支持前，必须验证 toolkit
  relocation/offline 行为。
- self 模式默认使用宿主 Hermes profile，多个 Bot 可能共享 Hermes 密钥、记忆、
  skills、sessions 和日志。
- Hermes 有自己的工具和审批系统。managed 模式必须先证明危险操作会 surfaced 到
  Memoh 审批。
- API Key 可能通过 bot backup 或 workspace export 泄漏。managed 模式必须先补
  导出策略。
- Managed UI 如果做得太大，会持续追随 Hermes 配置变化而漂移。第一版应保持小表单，
  后续再提供高级 native config 出口。
