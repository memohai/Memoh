# 关于 Memoh

## Memoh 是什么？

Memoh 是一个多成员、结构化长记忆、容器化的 AI Agent 系统平台。你可以创建自己的 AI Bot，通过 Telegram、飞书等平台与它们聊天。每个 Bot 拥有独立的容器和记忆系统，可以在自己的容器内编辑文件、执行命令、访问网络——就像拥有一台自己的电脑。

## 核心功能

### 多 Bot 管理

创建多个 Bot。人与 Bot、Bot 与 Bot 之间可以私聊、群聊或协作。组建 Bot 团队，或为家庭成员设置账号，用 Bot 管理日常事务。

### 容器化隔离

每个 Bot 运行在自己的隔离容器中（基于 Containerd），拥有独立的文件系统和网络。Bot 可以在各自容器内自由读写文件、执行命令，互不干扰。

### 记忆工程

受 Mem0 启发、深度工程化的记忆层：

- 自动从每轮对话中提取关键事实，以结构化记忆存储
- 支持语义搜索（通过 Qdrant 向量数据库）和关键词检索（BM25）
- 默认加载最近 24 小时的对话上下文
- 自动记忆压缩，保持记忆库精简
- 多语言自动检测

### 多平台支持

统一的接入平台适配器架构，连接多个消息平台：

- **Telegram** — 完整支持，含流式输出、Markdown、附件和回复
- **飞书** — 完整支持
- **Web** — 内置 Web 聊天界面
- **CLI** — 命令行聊天

### Agent 能力

Bot 内置丰富的工具集：

- **Web 搜索** — 集成 Brave Search，获取实时信息
- **子代理** — 创建专门的子代理（Subagent），分配技能，委派复杂任务
- **技能** — 可配置的技能系统，扩展 Bot 能力
- **容器操作** — 在容器内读写文件、编辑代码、执行命令
- **记忆管理** — 搜索和管理记忆
- **消息发送** — 发送消息和回应

### 多 LLM Provider 支持

通过四种客户端类型灵活切换多种 LLM Provider：

- OpenAI Responses API、OpenAI Chat Completions API（包括兼容服务）
- Anthropic Messages API、Google Generative AI API

### MCP 协议支持

通过 HTTP 和 SSE 支持 Model Context Protocol (MCP)，连接外部工具服务。每个 Bot 可以拥有独立的 MCP 连接。

### 定时任务

使用 cron 表达式配置定时任务，在指定时间自动执行命令。支持最大执行次数限制和手动触发。

### 图形化配置

通过 Web 管理界面配置 Bot、接入平台、Provider、模型、MCP、技能等所有设置——无需编写代码即可搭建你的 AI Bot。

### CLI 工具

命令行工具，用于 Bot 管理、接入平台配置、模型管理、流式聊天等——专为偏好终端的开发者设计。

## 安装

运行 Memoh：

- **[Docker](/zh/installation/docker)** — 推荐方式。通过 Docker Compose 一键安装或手动部署。包含所有服务（PostgreSQL、Qdrant、Containerd、server、agent、web）——宿主机无需额外依赖。
- **[config.toml](/zh/installation/config-toml)** — 所有配置字段参考。
