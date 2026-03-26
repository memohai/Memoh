# DeerFlow vs Memoh 对比分析：Harness 框架演进方向

> 记录时间: 2026-03-26
> 分析来源: DeerFlow (bytedance/deer-flow) 与 Memoh 架构对比

---

## 一、核心差异

| 维度 | DeerFlow (Harness) | Memoh (当前) |
|------|-------------------|--------------|
| **核心抽象** | Thread (任务会话) | Bot (常驻实体) |
| **Agent 生命周期** | 随任务创建/销毁 | 长期运行 |
| **架构分层** | Harness(可复用框架) + App(应用代码) | 单体应用 |
| **任务执行** | Lead Agent + 动态 Sub-agents | 单一 Bot 处理 |
| **使用方式** | HTTP API / Embedded Library / CLI | HTTP API / Web UI |
| **沙箱** | 任务级隔离 | Bot 级隔离 |

---

## 二、DeerFlow 的 Harness 本质特征

### 1. 分层架构

```
deer-flow/
├── packages/
│   └── harness/          # 可发布的框架包 (deerflow-harness)
│       └── deerflow/     # 核心框架代码
├── app/                  # 应用层代码 (Gateway, Channels)
├── skills/               # Skill 目录
└── frontend/             # Web UI
```

**关键原则**: Harness 层 (`deerflow.*`) 不依赖应用层 (`app.*`)，反向可以。

### 2. Thread/Task 为中心

- 用户与 Thread 交互，而非直接与 Agent 交互
- 每个 Thread 有独立的沙箱、记忆、工作目录
- Agent 配置通过 `RunnableConfig` 动态传入

### 3. 完整的中间件链

DeerFlow 使用 12 个中间件处理请求：

1. ThreadDataMiddleware - 创建线程目录
2. UploadsMiddleware - 处理文件上传
3. SandboxMiddleware - 获取沙箱实例
4. DanglingToolCallMiddleware - 处理中断的工具调用
5. GuardrailMiddleware - 权限控制
6. SummarizationMiddleware - 上下文压缩
7. TodoListMiddleware - 任务追踪（计划模式）
8. TitleMiddleware - 自动生成标题
9. MemoryMiddleware - 记忆更新队列
10. ViewImageMiddleware - 图像处理
11. SubagentLimitMiddleware - 子代理并发限制
12. ClarificationMiddleware - 澄清请求拦截

### 4. Sub-agent 编排系统

```python
# 动态创建子代理执行任务
task_tool = {
    "description": "委派任务给子代理",
    "subagent_type": "general-purpose" | "bash" | "custom",
    "prompt": "...",
    "max_turns": 10
}
```

特点：
- 双线程池调度（scheduler + execution）
- 最大并发限制（MAX_CONCURRENT_SUBAGENTS = 3）
- 15 分钟超时
- SSE 事件实时反馈

### 5. 沙箱 Provider 模式

```python
# 抽象接口
class SandboxProvider:
    def acquire(self, thread_id) -> Sandbox
    def get(self, thread_id) -> Sandbox
    def release(self, thread_id)

# 多种实现
- LocalSandboxProvider    # 本地执行
- AioSandboxProvider      # Docker 隔离
- K8sSandboxProvider      # Kubernetes 编排
```

虚拟路径系统：
- `/mnt/user-data/workspace/` - 工作目录
- `/mnt/user-data/uploads/` - 上传文件
- `/mnt/user-data/outputs/` - 输出目录
- `/mnt/skills/` - Skill 目录

### 6. Skill 渐进加载

Skills 按需加载到上下文，不是一次性加载所有：

```yaml
# SKILL.md 格式
---
name: research
description: 深度研究能力
allowed_tools: [web_search, web_fetch, bash]
---

# Skill 内容...
```

### 7. Embedded Client

```python
from deerflow import DeerFlowClient

client = DeerFlowClient()
response = client.chat("分析这个文件", thread_id="task-1")

# 流式输出
for event in client.stream("hello"):
    if event.type == "messages-tuple":
        print(event.data["content"])
```

---

## 三、Memoh 向 Harness 演进的关键改造

### 修正：保持 Bot 核心，内部增强 Harness 能力

**产品定位差异决定架构选择**：
- DeerFlow = 任务执行框架（Thread 为核心）
- Memoh = 个人助手（Bot 为核心，长期关系）

```
DeerFlow: User → Thread → 完成任务 → 结束
                    ↓
              动态创建 Agent

Memoh:    User ↔ Bot (长期运行、终身记忆)
                    ↓
              遇到复杂任务时
                    ↓
              内部启动 Task/Sub-agents
                    ↓
              完成后结果返回 Bot
```

**Bot 保持核心地位**：
- 长期运行、持久记忆、用户关系、跨平台身份

**Harness 能力增强 Bot**：
- 复杂任务时内部启用编排
- 需要时启动 Sub-agents
- 工具执行沙箱化
- Skill 按需加载

### 改造 1: 分层架构（Harness 能力内化到 Bot）

```
memoh/
├── internal/
│   ├── harness/              # 核心编排能力（非独立包，先内化）
│   │   ├── agent/            # Agent 执行引擎
│   │   ├── subagent/         # 子代理编排（新增）
│   │   ├── tools/            # 工具系统
│   │   ├── sandbox/          # 沙箱抽象
│   │   └── skills/           # Skill 渐进加载
│   ├── core/                 # 领域模型
│   │   └── bot.go            # Bot 实体（保持核心）
│   ├── server/
│   ├── handlers/
│   └── channels/
└── web/
```

**关键原则**:
- Harness 能力先内化为 Bot 的内部能力
- Bot 按需启用复杂编排，而非全部任务都走 Thread 模式
- 未来可考虑抽取独立包，但非优先

### 改造 3: Sub-agent 编排机制

```go
type SubAgent interface {
    Name() string
    Execute(ctx context.Context, task Task) (Result, error)
}

type Harness struct {
    LeadAgent    Agent
    SubAgents    map[string]SubAgent
    Sandbox      SandboxProvider
    Memory       MemoryProvider
}
```

### 改造 4: Sandbox 抽象化

当前: 每个 Bot 一个 containerd 容器
目标: 支持多种沙箱后端

```go
type SandboxProvider interface {
    Acquire(threadID string) (Sandbox, error)
    Get(threadID string) (Sandbox, error)
    Release(threadID string) error
}

// 实现
- LocalSandboxProvider      // 本地执行
- ContainerdProvider        // containerd 隔离
- DockerProvider            // Docker 隔离
- K8sProvider               // Kubernetes 编排
```

### 改造 5: 中间件链

参考 DeerFlow，设计 Memoh 的中间件：

```
请求 → AuthMiddleware → RateLimitMiddleware →
      SandboxMiddleware → MemoryMiddleware →
      ToolInterceptor → AgentExecution →
      ResponseMiddleware
```

### 改造 6: Skill 系统重构

当前: Bot 级别配置，启动时加载
目标: 按需加载、标准格式、动态启用

```yaml
# SKILL.md
---
name: web-research
description: Web 研究能力
version: 1.0.0
author: memohai
allowed_tools: [web_search, web_fetch, browser]
---

技能详细描述...
```

### 改造 7: Embedded Client

```go
package main

import "github.com/memohai/memoh/packages/harness"

func main() {
    client := harness.NewClient()

    // 同步调用
    resp, err := client.Chat(ctx, "分析这个文件",
        harness.WithThreadID("task-1"),
        harness.WithSkill("research"),
    )

    // 流式调用
    stream, err := client.Stream(ctx, "执行任务")
    for msg := range stream {
        fmt.Println(msg.Content)
    }
}
```

---

## 四、演进路径建议

### 阶段 1: 抽象层提取
- [ ] 识别核心编排逻辑（Agent 循环、工具调用）
- [ ] 提取为独立的 `memoh-harness` 包
- [ ] 确保无外部依赖（数据库、Web 框架）
- [ ] 编写单元测试保证边界

### 阶段 2: Bot 内部任务编排
- [ ] Bot 保留核心地位
- [ ] 引入内部 Task/Thread 概念（仅用于复杂任务）
- [ ] Bot 配置 + 运行时任务上下文的结合
- [ ] 简单对话保持现有流程，复杂任务启用编排

### 阶段 3: Sub-agent 系统
- [ ] 设计 Sub-agent 注册机制
- [ ] 实现任务分解与委派
- [ ] 结果聚合逻辑
- [ ] 并发控制与超时处理

### 阶段 4: 沙箱标准化
- [ ] 定义 `SandboxProvider` 接口
- [ ] 实现多种沙箱后端
- [ ] 虚拟路径系统
- [ ] 安全隔离验证

### 阶段 5: 高级功能
- [ ] 中间件链
- [ ] 计划模式（TodoList）
- [ ] 上下文压缩
- [ ] 记忆系统优化

---

## 五、关键决策点

| 决策项 | DeerFlow 选择 | Memoh 可选方案 | 考虑因素 |
|--------|--------------|---------------|---------|
| 编排引擎 | LangGraph | 自研 / 引入工作流引擎 / 保持当前 | 复杂度、可控性、生态 |
| 多语言 | Python 为主 | 纯 Go / Go+Python 运行时 | 工具生态、性能 |
| 内存存储 | 本地 JSON | PostgreSQL / SQLite / 可选 | 规模、部署复杂度 |
| 平台集成 | Channels 作为 App 层 | 类似设计 | 可扩展性 |
| Skill 格式 | Markdown + YAML | 保持当前 / 标准统一 | 生态兼容性 |

---

## 六、结论

### 核心转变（修正版）

> **不是从"Bot"转向"Thread"，而是让 Bot 拥有 Harness 的内部编排能力**

| 维度 | DeerFlow | Memoh |
|------|---------|-------|
| 核心交互单位 | Thread | Bot |
| 使用方式 | 发起任务 → 完成 → 结束 | 持续对话关系 |
| 记忆模式 | 任务上下文 | 终身记忆 |
| Sub-agent | 必须（复杂任务分解）| 可选（需要时启用）|
| Harness 角色 | 全部 | Bot 内部增强 |

### Memoh 的独特优势（保持并强化）

1. **个人助手定位** — 长期关系、终身记忆
2. **容器化** — 已使用 containerd，比 DeerFlow 更轻量
3. **多平台** — Telegram、Discord、Lark、Email
4. **记忆工程** — 多提供商架构
5. **浏览器** — 每个 Bot 独立 headless 浏览器

### 需要引入的 Harness 特性

1. **Bot 内部 Task 模式** — 复杂任务时启用编排
2. **Sub-agent 能力** — 需要时动态创建子代理
3. **Skill 按需加载** — 不是所有技能常驻上下文
4. **沙箱标准化** — Provider 模式支持多种后端
5. **中间件链** — 可扩展的请求处理流程

---

## 七、参考资源

- DeerFlow 文档: https://github.com/bytedance/deer-flow
- DeerFlow Backend CLAUDE.md: `/backend/CLAUDE.md`
- Memoh 文档: https://docs.memoh.ai

---

## 附录：中间件链优化分析

### 当前架构问题

**`internal/channel/inbound/channel.go:HandleInbound` 现状：**
- 约 2340 行的硬编码流程
- 各阶段紧耦合，难以扩展
- 新功能需要修改核心方法

**当前处理流程（硬编码）：**
```
1. Validate (行 234) → 2. Identity Resolution (行 259)
→ 3. Command Interception (行 282) → 4. Attachment Processing (行 284)
→ 5. Route Resolution (行 290) → 6. Session Management (行 311)
→ 7. ACL Checks (行 330) → 8. Token Generation (行 378)
→ 9. Stream Execution (行 520)
```

### 优化目标

1. **可扩展性** - 新功能通过添加中间件实现，不修改核心代码
2. **可组合性** - 中间件可条件启用（如某些渠道跳过某些检查）
3. **可测试性** - 每个中间件独立测试
4. **可观测性** - 中间件链执行过程可追踪

### 核心接口设计

```go
// Context 跨中间件共享状态
type Context struct {
    context.Context

    // 请求级
    Channel   channel.ChannelConfig
    Message   channel.InboundMessage
    Sender    channel.StreamReplySender

    // 解析结果
    Identity  *models.Identity
    Bot       *models.Bot
    Session   *models.Session
    Route     *RouteResult

    // 中间件可添加自定义数据
    Metadata  map[string]any
}

// Middleware 接口
type Middleware interface {
    Name() string
    Execute(ctx *Context, next NextFunc) error
}

type NextFunc func(ctx *Context) error
```

### 中间件映射表

| # | 中间件名称 | 对应当前代码 | 职责 | 可跳过条件 |
|---|-----------|-------------|------|-----------|
| 1 | **ValidateMiddleware** | 行 234 验证 | 消息格式校验 | - |
| 2 | **IdentityMiddleware** | 行 259 身份解析 | 用户身份识别 | - |
| 3 | **PolicyMiddleware** | 命令拦截逻辑 | 系统命令处理（/reset等）| - |
| 4 | **CommandMiddleware** | 行 282 命令拦截 | 特殊指令处理 | 非指令消息 |
| 5 | **AttachmentMiddleware** | 行 284 附件处理 | 文件上传处理 | 无附件时 |
| 6 | **RouteMiddleware** | 行 290 路由解析 | Bot/技能路由 | - |
| 7 | **SessionMiddleware** | 行 311 会话管理 | 会话创建/恢复 | - |
| 8 | **ACLMiddleware** | 行 330 ACL检查 | 权限验证 | 白名单渠道 |
| 9 | **TokenMiddleware** | 行 378 Token生成 | 生成LLM请求 | - |
| 10 | **NotifyMiddleware** | 缺失 | 处理前通知（typing等）| 非交互渠道 |
| 11 | **ExecuteMiddleware** | 行 520 流执行 | 调用Agent执行 | - |
| 12 | **StreamMiddleware** | 流处理 | 响应流管理 | 非流式响应 |

### 链式执行示例

```go
// 构建中间件链
chain := NewChain(
    &ValidateMiddleware{},
    &IdentityMiddleware{Provider: idProvider},
    &PolicyMiddleware{Handlers: commandHandlers},
    &AttachmentMiddleware{Storage: s3Storage},
    &RouteMiddleware{Resolver: routeResolver},
    &SessionMiddleware{Store: sessionStore},
    &ACLMiddleware{Checker: aclChecker},
    &TokenMiddleware{Generator: tokenGen},
    &NotifyMiddleware{Notifier: notifier},
    &ExecuteMiddleware{Agent: agent},
    &StreamMiddleware{},
)

// 执行
ctx := NewContext(baseCtx, cfg, msg, sender)
if err := chain.Execute(ctx); err != nil {
    return err
}
```

### 条件中间件

```go
// 某些中间件只在特定条件下执行
type ConditionalMiddleware struct {
    Condition func(*Context) bool
    Middleware
}

// 示例：附件处理只在有附件时执行
&ConditionalMiddleware{
    Condition: func(ctx *Context) bool {
        return len(ctx.Message.Attachments) > 0
    },
    Middleware: &AttachmentMiddleware{},
}
```

### 与现有代码的兼容性

**阶段 1：并行运行**
- 保留现有 `HandleInbound` 方法
- 新增 `HandleInboundV2` 使用中间件链
- 通过配置开关切换

**阶段 2：逐步迁移**
- 将现有逻辑提取为独立中间件
- 每个中间件单独测试验证
- 渐进式替换

**阶段 3：完全切换**
- 移除旧实现
- 中间件链成为标准

### 收益对比

| 维度 | 当前硬编码 | 中间件链 |
|-----|-----------|---------|
| 新增功能 | 修改核心方法（高风险） | 添加中间件（低风险） |
| 功能开关 | 新增 if 分支 | 条件中间件或链配置 |
| 单测覆盖 | 需要完整集成测试 | 中间件独立单元测试 |
| 故障定位 | 在 2340 行中排查 | 中间件级日志追踪 |
| 渠道定制 | 复制修改代码 | 配置不同中间件组合 |

### 推荐实现步骤

1. **定义接口** - `Context`, `Middleware`, `Chain`
2. **提取第一个中间件** - 从 `Validate` 开始，风险最低
3. **验证模式** - 确保与现有依赖（数据库、缓存等）兼容
4. **逐步迁移** - 每次提取 1-2 个中间件，充分测试
5. **性能基准** - 对比中间件链与硬编码的性能差异

---

## 附录二：Subagent 机制优化分析

### 当前实现概览

**Memoh 当前实现** (`internal/agent/tools/subagent.go`):
- `spawn` 工具接收任务数组，并行执行
- 使用 `sync.WaitGroup` 等待所有任务完成
- 每个 subagent 创建独立 session，持久化消息历史
- 统一的系统提示词 (`system_subagent.md`)
- **问题**：无并发限制、无超时控制、同步阻塞执行

```go
// 当前实现核心逻辑
for i, task := range tasks {
    go func(idx int, query string) {
        defer wg.Done()
        results[idx] = p.runSubagentTask(ctx, session, sdkModel, modelID, systemPrompt, query)
    }(i, task)
}
wg.Wait() // 阻塞等待所有完成
```

### DeerFlow 参考实现

**DeerFlow** (`packages/harness/deerflow/subagents/`):
- **双线程池调度**：scheduler pool (3) + execution pool (3)
- **并发限制**：`MAX_CONCURRENT_SUBAGENTS = 3`
- **超时控制**：默认 15 分钟，可配置
- **异步执行**：后台运行，SSE 实时反馈
- **Subagent 类型系统**：general-purpose, bash 等
- **生命周期状态**：PENDING → RUNNING → COMPLETED/FAILED/TIMED_OUT

### 关键差异对比

| 特性 | Memoh (当前) | DeerFlow | 优先级 |
|-----|-------------|----------|-------|
| **并发控制** | 无限制（可能同时启动 N 个） | 最大 3 个并发 | 高 |
| **执行模式** | 同步阻塞 | 异步 + 实时反馈 | 高 |
| **超时控制** | 无 | 可配置（默认 15min） | 高 |
| **Subagent 类型** | 单一类型 | 多种类型（research, bash...） | 中 |
| **工具过滤** | 继承父 agent 全部工具 | 允许/禁止列表 | 中 |
| **状态追踪** | 简单成功/失败 | 完整生命周期状态 | 中 |
| **分布式追踪** | 无 | Trace ID 关联日志 | 低 |

### 优化方案设计

#### 1. 并发控制与执行模型

```go
// 双工作池设计
type SubagentExecutor struct {
    schedulerPool  *workerpool.Pool  // 调度协程池 (3 workers)
    executionPool  *workerpool.Pool  // 执行协程池 (3 workers)
    maxConcurrent  int               // 最大并发数 (默认 3)
}

// 带权重的任务队列
type Task struct {
    ID       string
    Prompt   string
    Priority int  // 优先级，用于队列排序
    Timeout  time.Duration
}
```

**关键改进**：
- 限制同时运行的 subagent 数量，避免 LLM API 限流
- 超出限制的任务进入队列等待
- 支持优先级调度（紧急任务优先）

#### 2. 异步执行与实时反馈

```go
// 异步执行接口
type AsyncSubagentManager interface {
    // 启动异步任务，立即返回 taskID
    ExecuteAsync(task Task) (taskID string, err error)

    // 查询任务状态
    GetStatus(taskID string) (SubagentStatus, error)

    // 订阅实时更新（SSE/WebSocket）
    Subscribe(taskID string, handler EventHandler) error

    // 取消任务
    Cancel(taskID string) error
}

// 事件类型
type SubagentEventType string
const (
    EventStarted    SubagentEventType = "started"
    EventRunning    SubagentEventType = "running"
    EventProgress   SubagentEventType = "progress"
    EventCompleted  SubagentEventType = "completed"
    EventFailed     SubagentEventType = "failed"
    EventCancelled  SubagentEventType = "cancelled"
    EventTimeout    SubagentEventType = "timeout"
)
```

**与现有 spawn 工具的兼容**：
```go
// spawn 工具保持同步接口，但内部使用异步执行
func (p *SpawnProvider) execSpawn(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
    // 为每个任务启动异步执行
    var taskIDs []string
    for _, task := range tasks {
        id, _ := p.asyncManager.ExecuteAsync(task)
        taskIDs = append(taskIDs, id)
    }

    // 等待所有完成（保持向后兼容）
    results := p.waitForCompletion(ctx, taskIDs)
    return map[string]any{"results": results}, nil
}
```

#### 3. Subagent 类型系统

```go
// Subagent 配置定义
type SubagentConfig struct {
    Name             string
    Description      string
    SystemPrompt     string
    AllowedTools     []string    // 允许的工具（nil = 全部）
    DisallowedTools  []string    // 禁止的工具（如 spawn 防止嵌套）
    MaxTurns         int         // 最大对话轮数
    TimeoutSeconds   int         // 超时时间
    Model            string      // "inherit" 或具体模型名
}

// 内置类型
var BuiltinSubagents = map[string]SubagentConfig{
    "general-purpose": {
        Name:            "general-purpose",
        Description:     "通用任务处理",
        SystemPrompt:    "...",
        DisallowedTools: []string{"spawn"},  // 禁止嵌套
        MaxTurns:        50,
        TimeoutSeconds:  900,
        Model:           "inherit",
    },
    "bash": {
        Name:            "bash",
        Description:     "命令行专家",
        SystemPrompt:    "...",
        AllowedTools:    []string{"bash", "read_file", "write_file"},
        MaxTurns:        20,
        TimeoutSeconds:  300,
        Model:           "inherit",
    },
    "research": {
        Name:            "research",
        Description:     "研究分析专家",
        SystemPrompt:    "...",
        AllowedTools:    []string{"web_search", "web_fetch", "browser"},
        MaxTurns:        30,
        TimeoutSeconds:  600,
        Model:           "inherit",
    },
}
```

#### 4. 完整状态生命周期

```go
type SubagentStatus string
const (
    StatusPending    SubagentStatus = "pending"     // 等待执行
    StatusRunning    SubagentStatus = "running"     // 执行中
    StatusPaused     SubagentStatus = "paused"      // 暂停（可恢复）
    StatusCompleted  SubagentStatus = "completed"   // 成功完成
    StatusFailed     SubagentStatus = "failed"      // 执行失败
    StatusCancelled  SubagentStatus = "cancelled"   // 被取消
    StatusTimedOut   SubagentStatus = "timed_out"   // 超时
)

type SubagentExecution struct {
    ID            string
    Status        SubagentStatus
    ParentID      string          // 父 session ID
    Config        SubagentConfig

    // 时间追踪
    CreatedAt     time.Time
    StartedAt     *time.Time
    CompletedAt   *time.Time

    // 执行结果
    Result        *SpawnResult
    Error         error

    // 实时数据（执行中更新）
    CurrentTurn   int
    LastMessage   string
    Progress      float64  // 0-100
}
```

#### 5. 与中间件链的集成

在 DeerFlow 中，`SubagentLimitMiddleware` 负责截断超出并发限制的 subagent 调用：

```go
// SubagentLimitMiddleware 伪代码
func (m *SubagentLimitMiddleware) AfterModel(ctx *Context) error {
    toolCalls := extractToolCalls(ctx.Response)

    spawnCalls := filterByName(toolCalls, "spawn")
    if len(spawnCalls) > MAX_CONCURRENT_SUBAGENTS {
        // 截断超额调用，返回错误消息
        truncated := spawnCalls[:MAX_CONCURRENT_SUBAGENTS]
        excess := spawnCalls[MAX_CONCURRENT_SUBAGENTS:]

        // 为超额的调用添加错误响应
        for _, call := range excess {
            ctx.AddToolResponse(call.ID, "Error: 并发 subagent 数量超过限制")
        }

        ctx.Response.ToolCalls = truncated
    }
    return nil
}
```

### 实现优先级建议

**阶段 1（高优先级）**：
1. 添加并发限制（简单 WaitGroup → 有界信号量）
2. 添加超时控制（context.WithTimeout）
3. 基本状态追踪（pending/running/completed）

**阶段 2（中优先级）**：
1. Subagent 类型系统（general-purpose, bash）
2. 工具过滤（禁止 spawn 嵌套）
3. 异步执行接口

**阶段 3（低优先级）**：
1. SSE 实时反馈
2. 分布式追踪（Trace ID）
3. 任务队列与优先级调度

### 风险与缓解

| 风险 | 影响 | 缓解措施 |
|-----|-----|---------|
| 并发限制导致任务堆积 | 用户体验下降 | 队列长度限制 + 优雅降级 |
| 异步执行增加复杂度 | 调试困难 | 完善日志 + 状态查询接口 |
| 超时强制终止 | 数据不一致 | 支持优雅关闭 + 状态保存 |
| 嵌套调用限制 | 某些场景不可用 | 配置化白名单机制 |

---

## 附录三：Harness 分层架构设计

### DeerFlow 分层模式回顾

```
deep-flow/backend/
├── packages/harness/deerflow/    # Harness 层（可发布框架）
│   ├── agents/                   #   Agent 编排引擎
│   ├── subagents/                #   Sub-agent 系统
│   ├── sandbox/                  #   沙箱抽象
│   ├── tools/                    #   工具系统
│   ├── memory/                   #   记忆系统
│   ├── skills/                   #   Skill 加载
│   └── config/                   #   配置系统
│
└── app/                          # App 层（应用代码）
    ├── gateway/                  #   REST API
    └── channels/                 #   IM 渠道集成
```

**核心原则**：Harness 层 (`deerflow.*`) **绝不依赖** App 层 (`app.*`)，反向可以。

### Memoh 当前架构问题

```
memoh/
├── internal/                     # 所有代码混在一起
│   ├── agent/                    #   Agent 核心
│   ├── channel/                  #   渠道适配（紧耦合）
│   ├── handlers/                 #   HTTP 处理器
│   ├── workspace/                #   容器管理
│   └── ...                       #   其他模块
```

**问题**：
1. 没有清晰的框架层 vs 应用层界限
2. 渠道逻辑与核心编排紧耦合
3. 难以独立发布 "memoh-harness" 包供他人使用
4. 单元测试需要拉起完整依赖

### 目标分层架构

```
memoh/
├── harness/                      # NEW: Harness 框架层
│   ├── agent/                    #   Agent 执行引擎（Twilight SDK 包装）
│   │   ├── engine.go             #     核心执行循环
│   │   ├── stream.go             #     流式处理
│   │   └── config.go             #     Agent 配置
│   │
│   ├── orchestration/            #   编排系统（新增）
│   │   ├── middleware/           #     中间件链框架
│   │   ├── subagent/             #     Sub-agent 管理
│   │   ├── task/                 #     Task/Thread 抽象
│   │   └── sandbox/              #     沙箱 Provider 接口
│   │
│   ├── tools/                    #   工具系统
│   │   ├── registry.go           #     工具注册中心
│   │   ├── provider.go           #     ToolProvider 接口
│   │   └── builtins/             #     内置工具实现
│   │
│   ├── memory/                   #   记忆抽象层
│   │   ├── provider.go           #     MemoryProvider 接口
│   │   └── types.go              #     记忆类型定义
│   │
│   ├── skills/                   #   Skill 系统
│   │   ├── loader.go             #     Skill 加载器
│   │   ├── registry.go           #     Skill 注册表
│   │   └── types.go              #     Skill 类型定义
│   │
│   └── config/                   #   Harness 配置（不含 app 配置）
│       ├── model.go              #     模型配置
│       └── tool.go               #     工具配置
│
├── app/                          # App 应用层
│   ├── server/                   #   HTTP 服务器
│   │   ├── handlers/             #     请求处理器（原 internal/handlers）
│   │   ├── middleware/           #     HTTP 中间件（auth, cors等）
│   │   └── router.go             #     路由定义
│   │
│   ├── channels/                 #   渠道适配（原 internal/channel）
│   │   ├── telegram/             #     Telegram 适配
│   │   ├── discord/              #     Discord 适配
│   │   ├── feishu/               #     飞书适配
│   │   └── processor.go          #     渠道消息处理器
│   │
│   ├── storage/                  #   数据持久化实现
│   │   ├── postgres/             #     PostgreSQL 实现
│   │   ├── qdrant/               #     Qdrant 向量库实现
│   │   └── memory/               #     记忆 Provider 实现
│   │
│   ├── workspace/                #   容器运行时实现
│   │   ├── containerd/           #     containerd 实现
│   │   └── apple/                #     Apple Virtualization 实现
│   │
│   └── fx/                       #   依赖注入组装
│       └── modules.go            #     FX provider 定义
│
└── cmd/                          # 入口点
    ├── server/                   #   服务器启动
    └── worker/                   #   后台 worker
```

### 依赖规则与边界

```
┌─────────────────────────────────────────────────────────────┐
│                        App 层                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   server    │  │  channels   │  │  storage/workspace  │  │
│  │  (HTTP API) │  │ (IM adapters)│  │   (infra impl)      │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                    │             │
│         └────────────────┼────────────────────┘             │
│                          ▼                                  │
│                   ┌─────────────┐                           │
│                   │     fx      │  (DI composition)         │
│                   └──────┬──────┘                           │
└──────────────────────────┼──────────────────────────────────┘
                           │ imports
┌──────────────────────────┼──────────────────────────────────┐
│                          ▼                                  │
│                    Harness 层                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │    agent    │  │orchestration│  │  tools/memory/skills│  │
│  │   (engine)  │  │(middleware) │  │   (abstractions)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                                             │
│  NO imports from app.*  ←  严格边界检查                     │
└─────────────────────────────────────────────────────────────┘
```

**依赖规则**：
1. **App → Harness**：允许，App 组装 Harness 组件
2. **Harness → App**：禁止，Harness 必须独立
3. **App → App**：允许，应用内部依赖
4. **Harness → Harness**：允许，框架内部依赖

### 接口边界定义

#### Harness 层暴露的接口

```go
// harness/agent/engine.go
package agent

type Engine interface {
    // 同步执行
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)

    // 流式执行
    Stream(ctx context.Context, req GenerateRequest) (<-chan StreamEvent, error)
}

// 纯配置结构，不含数据库依赖
type GenerateRequest struct {
    Model       ModelConfig
    System      string
    Messages    []Message
    Tools       []Tool
    SessionID   string
    MaxTurns    int
}
```

```go
// harness/orchestration/middleware/types.go
package middleware

type Context struct {
    context.Context

    // 请求数据（纯内存，无 DB 依赖）
    SessionID   string
    BotID       string
    UserID      string
    Message     Message

    // 中间件可修改的状态
    Metadata    map[string]any
}

type Middleware interface {
    Name() string
    Execute(ctx *Context, next NextFunc) error
}

type Chain interface {
    Use(middleware ...Middleware)
    Execute(ctx *Context) error
}
```

```go
// harness/sandbox/provider.go
package sandbox

// Provider 是沙箱的抽象，App 层提供具体实现
type Provider interface {
    Acquire(ctx context.Context, sessionID string) (Sandbox, error)
    Get(sessionID string) (Sandbox, error)
    Release(sessionID string) error
}

type Sandbox interface {
    Execute(ctx context.Context, cmd string) (ExecuteResult, error)
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, data []byte) error
}
```

#### App 层实现的接口

```go
// app/storage/postgres/memory.go
package postgres

// 实现 harness/memory.Provider 接口
type MemoryStore struct {
    queries *sqlc.Queries
    qdrant  *qdrant.Client
}

func (s *MemoryStore) Search(ctx context.Context, req memory.SearchRequest) ([]memory.Result, error) {
    // 使用 PostgreSQL + Qdrant 实现
}
```

```go
// app/workspace/containerd/provider.go
package containerd

// 实现 harness/sandbox.Provider 接口
type ContainerdProvider struct {
    client *containerd.Client
}

func (p *ContainerdProvider) Acquire(ctx context.Context, sessionID string) (sandbox.Sandbox, error) {
    // 创建/获取容器
}
```

### Go 惯例合规性分析与调整

原建议的 `harness/` + `app/` 顶级目录结构**不符合 Go 惯例**。Go 社区标准布局如下：

```
project/
├── cmd/              # 应用程序入口
├── internal/         # 私有应用代码（不允许被外部导入）
├── pkg/              # 可被外部使用的库代码（可选）
├── api/              # API 定义
├── web/              # 前端资源
└── docs/             # 文档
```

#### 调整后的合规设计

**方案 A：使用 `pkg/` + `internal/`（推荐）**

```
memoh/
├── pkg/                          # 可复用的 Harness 框架代码
│   └── harness/                  #   对外暴露的框架包
│       ├── agent/                #     Agent 引擎接口
│       ├── orchestration/        #     编排框架
│       ├── tools/                #     工具接口与注册
│       ├── sandbox/              #     沙箱 Provider 接口
│       ├── memory/               #     记忆 Provider 接口
│       └── skills/               #     Skill 接口
│
├── internal/                     # 应用私有代码
│   ├── app/                      #   应用层组装（原 harness/app 边界）
│   │   ├── server/               #     HTTP 服务器
│   │   ├── channels/             #     渠道适配
│   │   ├── storage/              #     存储实现
│   │   └── fx/                   #     DI 组装
│   │
│   ├── domain/                   #   领域实现（依赖 pkg/harness 接口）
│   │   ├── agent/                #     Agent 实现（原 internal/agent）
│   │   ├── workspace/            #     容器实现
│   │   └── memory/               #     记忆实现
│   │
│   └── infra/                    #   基础设施
│       ├── postgres/             #     DB 实现
│       ├── qdrant/               #     向量库实现
│       └── containerd/           #     容器运行时
│
└── cmd/                          # 入口点（保持不变）
```

**依赖规则（符合 Go 编译约束）**：
```
internal/app/        → 可以导入 pkg/harness/
internal/domain/     → 可以导入 pkg/harness/
pkg/harness/         → 不能导入 internal/* （编译器强制）
```

**方案 B：单模块内化（更保守）**

如果短期内不计划发布独立 harness 包，保持单模块结构：

```
memoh/
├── internal/
│   ├── harness/                  #   框架层（包内分层）
│   │   ├── interfaces/           #     接口定义（不依赖任何实现）
│   │   │   ├── sandbox.go
│   │   │   ├── memory.go
│   │   │   └── tools.go
│   │   ├── engine/               #     编排引擎
│   │   └── middleware/           #     中间件框架
│   │
│   ├── app/                      #   应用层（实现 harness 接口）
│   │   ├── server/
│   │   ├── channels/
│   │   └── providers/            #     接口实现
│   │       ├── sandbox/
│   │       ├── memory/
│   │       └── storage/
│   │
│   └── domain/                   #   领域服务
│       ├── agent/
│       ├── bots/
│       └── conversation/
```

### 关键调整对比

| 原建议（非惯用） | 调整后（符合 Go 惯例） | 理由 |
|---------------|---------------------|------|
| `harness/` 顶级 | `pkg/harness/` | 标准库使用 `pkg/` 存放可导入包 |
| `app/` 顶级 | `internal/app/` | 应用代码应为私有（`internal`） |
| 代码移动 | 大部分保持不动 | 减少变更成本，渐进式重构 |

### 文件移动映射（方案 A）

| 当前路径 | 新路径 | 说明 |
|---------|-------|------|
| `internal/agent/agent.go` | `internal/domain/agent/engine.go` | 领域实现 |
| `internal/agent/tools/*.go` | `pkg/harness/tools/*.go` | 框架接口 |
| `internal/channel/inbound/` | `internal/app/channels/processor.go` | 应用层 |
| `internal/handlers/` | `internal/app/server/handlers/` | 应用层 |
| `internal/workspace/` | `internal/infra/containerd/` | 基础设施 |
| `internal/memory/` | `internal/domain/memory/` | 领域实现 |
| `internal/db/sqlc/` | `internal/infra/postgres/sqlc/` | 基础设施 |

### 推荐的渐进式迁移步骤

**阶段 1：接口抽离（1-2 周）**

不移动文件，先抽离接口：

```go
// pkg/harness/interfaces/sandbox.go
package interfaces

type SandboxProvider interface {
    Acquire(ctx context.Context, sessionID string) (Sandbox, error)
    Release(sessionID string) error
}
```

**阶段 2：依赖反转（2 周）**

将 `internal/agent/tools` 中的直接依赖改为接口依赖：

```go
// 修改前
func NewContainerProvider(log *slog.Logger, manager *workspace.Manager)

// 修改后
func NewContainerProvider(log *slog.Logger, provider interfaces.SandboxProvider)
```

**阶段 3：包重构（2 周）**

仅当需要发布独立 harness 包时，才将 `pkg/harness` 移到独立 go module：

```bash
# 可选：发布为独立模块
cd pkg/harness
go mod init github.com/memohai/harness
```

### 收益（调整后的设计）

| 维度 | 当前 | 调整后 |
|-----|-----|-------|
| Go 惯例 | 部分符合 | 完全合规 |
| 编译约束 | 无 | `internal/` 强制边界 |
| 变更成本 | 高（大量移动） | 低（渐进式） |
| 独立发布 | 需重构 | 可随时提取 `pkg/harness` |

---

## 八、讨论待办

- [x] **确认产品定位** — 个人助手（Bot 核心）vs 任务框架（Thread 核心）✅ Bot 核心
- [ ] **中间件链重构** — 将 HandleInbound 硬编码流程改造为可扩展中间件链
- [ ] **Subagent 机制优化** — 引入并发控制、异步执行、类型系统
- [ ] 如何在保持 Bot 长期运行的同时，内部支持 Task 编排模式
- [ ] Sub-agent 的触发条件（自动判断？用户指令？配置阈值？）
- [ ] 评估阶段 1 工作量
- [ ] 确定优先级和里程碑
