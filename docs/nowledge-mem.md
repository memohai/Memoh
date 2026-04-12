# Nowledge Mem 集成设计文档

## 背景

Memoh 内置的 builtin memory provider 效果不佳。Nowledge Mem 是一个本地优先的 AI 记忆管理器，具备知识图谱、6 路混合搜索、后台智能（实体抽取、EVOLVES 关系检测、Crystal 合成、衰减评分）等能力，作为替代方案集成到 Memoh 的 memory provider 体系中。

## 调研：Nowledge Mem 能力概览

### 产品定位

- 本地优先的个人知识库，数据默认存储在用户设备
- 免费，支持 macOS / Windows / Linux
- 为 AI 工具（Claude Code、Cursor 等）提供统一的持久化记忆层

### 核心能力

| 能力 | 说明 |
|---|---|
| 记忆管理 | CRUD + 标签组织 + 重要性评分 |
| 语义搜索 | 6 路混合：semantic + full-text + entity + community + label + graph traversal |
| 知识图谱 | 自动实体抽取（Person/Concept/...）、关系可视化、遍历深度控制 |
| 蒸馏 (Distill) | 从对话线程中提取关键记忆 |
| Crystal 合成 | 三条以上独立记忆汇聚同一主题时自动合成统一摘要 |
| Decay + Confidence | 双分评分系统：新鲜度（指数衰减 + 频率提升）+ 可信度（只增不减） |
| Working Memory | 每日自动生成 daily briefing |
| 后台智能 | 定时任务：crystallization、community detection、KG extraction、decay refresh |

### 接入方式

- **REST API**：`http://127.0.0.1:14242`，无认证
- **MCP Server**：`http://127.0.0.1:14242/mcp`（streamableHttp）
- **CLI**：`nmem` 命令行工具

## 方案选择

### 排除的方案

- **方案 B（双向同步）**：Memoh 和 Nowledge Mem 各自有完整的记忆 pipeline，双向同步没有意义
- **方案 C（MCP 接入）**：不够原生，Agent 不会主动调用 MCP 工具来记忆

### 最终方案：方案 A — 作为 Memory Provider

将 Nowledge Mem 作为 Memoh 的 memory provider 实现，通过 `adapters.Provider` 接口接入：

- `OnBeforeChat`：搜索相关记忆注入上下文
- `OnAfterChat`：将对话写入 Nowledge Mem，由其 LLM 做实体抽取和知识图谱构建
- 搜索、存储、CRUD 全部代理到 Nowledge Mem REST API

### 为什么不用方案 D（线程归档）

方案 D 只是事后归档，对 agent 在线对话质量无直接帮助。可作为 A 的补充，不做替代。

## 设计决策

### 多用户/群聊适配

Nowledge Mem 是单用户个人知识库，没有 `user_id` 或 `group_id` 概念。但在多用户维度上，现有的 Mem0 和 OpenViking provider 同样没做隔离（都只按 `bot_id` scope）。

关键改进：**在存储文本中嵌入结构化的发言人标识**，让 Nowledge Mem 的实体抽取和全文搜索能区分不同人。

### 存储文本格式

每条记忆对应一轮对话（用户消息 + bot 回复），头部标注来源上下文：

```
(Telegram 群组「开发讨论」)
[张三] 我最近在用 Rust 重写后端
[小助手] 很好的选择，Rust 的性能和安全性都很出色
```

- **头部标注**：`({Platform} {会话类型}「{群组名}」)`
  - Platform：从 `CurrentChannel` 取值（telegram / feishu / discord 等），首字母大写
  - 会话类型映射：`group` → `群组`，`private` → `私聊`，`thread` → `话题`
  - 群组名：有则加 `「...」`，无则省略（私聊一般没有群组名）
  - 示例：`(Telegram 私聊)` 或 `(Feishu 群组「开发讨论」)`
- **用户消息**：`[{display-name}] {消息内容}`
  - display-name 从 YAML front-matter header 中解析（Memoh 的 user_header.go 在每条用户消息中嵌入了 `display-name` 字段）
  - 回退链：YAML header → AfterChatRequest.DisplayName → `"用户"`
- **Bot 消息**：`[{bot-display-name}] {消息内容}`
  - 使用 bot 的 display name（从 bots 表的 `display_name` 字段取）
  - Nowledge Mem 的实体抽取会将 bot 名识别为 Person 实体

### 为什么不用 `(@username)` 双标识

AfterChatRequest 中只有 `DisplayName`（人类可读名）和内部 UUID（UserID、ChannelIdentityID），没有平台级 username。YAML header 中也只有 `display-name` 和 `channel-identity-id`（内部 ID）。因此只使用 display-name。

### 查询侧

直接使用用户消息原文查询 `POST /memories/search`。Nowledge Mem 的 6 路搜索会自动处理：
- 语义搜索命中话题相关记忆
- 全文搜索命中包含人名的记忆
- 实体搜索命中 Person 实体关联的记忆

不需要在查询时拼接发言人信息。

### Spaces 隔离

利用 Nowledge Mem 的 Spaces 功能实现 per-bot 记忆隔离：

- 每个 bot 自动映射到一个 Space，名称为 `memoh:{botID}`（botID 是稳定的 UUID）
- 首次使用时自动 ensure（`GET /spaces` 查找 → 未找到则 `POST /spaces` 创建）
- `sync.Map` 缓存 `botID → spaceID` 映射，避免重复 API 调用
- 所有 `POST /memories`、`POST /memories/search` 调用带 `space_id` 参数
- 不同 bot 的记忆完全隔离，搜索不互相干扰
- Entity graph 保持全局（Nowledge Mem 设计如此），跨 bot 的实体关联仍可用

### 上下文注入格式

与现有 provider 一致的 `<memory-context>` XML 格式：

```xml
<memory-context>
Relevant memory context (use when helpful):
- [2025-01-15] [张三] 喜欢用 Rust，推荐过《The Rust Programming Language》
- [2025-01-10] [李四] 对 Rust 感兴趣但还没开始学习
</memory-context>
```

## 实现

### 新建文件

- `internal/memory/adapters/nowledgemem/client.go` — HTTP 客户端
- `internal/memory/adapters/nowledgemem/nowledgemem.go` — Provider 实现

### 修改文件

- `internal/memory/adapters/types.go` — 添加 `ProviderNowledgeMem` 常量
- `internal/memory/adapters/service.go` — 验证 + 元数据
- `cmd/memoh/serve.go` — 注册工厂
- `apps/web/src/pages/memory/components/add-memory-provider.vue` — UI 下拉选项
- `packages/sdk/src/types.gen.ts` — TypeScript 类型
- `apps/web/src/i18n/locales/en.json` / `zh.json` — 国际化

### 配置

创建 provider 时只需 `base_url`（可选，默认 `http://127.0.0.1:14242`）：

```json
{
  "name": "nmem",
  "provider": "nowledgemem",
  "config": {}
}
```

## 局限性

1. **本地依赖**：Nowledge Mem 必须与 Memoh 在同一台机器上运行
2. **无 GetAll**：Nowledge Mem API 不提供列出所有记忆的端点（带分页的 GET /memories 不含语义排序），GetAll 返回 unsupported error
3. **无 Compact**：记忆整理由 Nowledge Mem 后台智能自动处理（decay refresh、crystallization），不暴露给 Memoh
4. **display-name 可变**：用户改名后，旧记忆中的名字不会更新。但 Nowledge Mem 的 Entity aliases 机制可能缓解此问题
