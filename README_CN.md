<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" width="100" height="100">
  <h1>Memoh</h1>
  <p>多成员、结构化长记忆、容器化的 AI Agent 系统。</p>
  <p>📌 <a href="https://docs.memoh.ai/blogs/2026-02-16.html">Introduction to Memoh - The Case for an Always-On, Containerized Home Agent</a></p>
  <div align="center">
    <img src="https://img.shields.io/github/package-json/v/memohai/Memoh" alt="Version" />
    <img src="https://img.shields.io/github/license/memohai/Memoh" alt="License" />
    <img src="https://img.shields.io/github/stars/memohai/Memoh?style=social" alt="Stars" />
    <img src="https://img.shields.io/github/forks/memohai/Memoh?style=social" alt="Forks" />
    <img src="https://img.shields.io/github/last-commit/memohai/Memoh" alt="Last Commit" />
    <img src="https://img.shields.io/github/issues/memohai/Memoh" alt="Issues" />
    <a href="https://deepwiki.com/memohai/Memoh">
      <img src="https://deepwiki.com/badge.svg" alt="DeepWiki" />
    </a>
    <img src="https://github.com/memohai/Memoh/actions/workflows/docker.yml/badge.svg" alt="Docker" />
  </div>
  <div align="center">
    [<a href="https://t.me/memohai">Telegram 群组</a>]
    [<a href="https://docs.memoh.ai">文档</a>]
    [<a href="mailto:business@memoh.net">合作</a>]
  </div>
  <hr>
</div>

Memoh 是一个常驻运行的容器化 AI Agent 系统。你可以创建多个 AI 机器人，每个机器人运行在独立的容器中，拥有持久化记忆，并通过 Telegram、Discord、飞书(Lark)、Email 或内置的 Web/CLI 与之交互。机器人可以执行命令、编辑文件、浏览网页、通过 MCP 调用外部工具，并记住一切 —— 就像给每个 Bot 一台自己的电脑和大脑。

## 快速开始

一键安装（**需先安装 [Docker](https://www.docker.com/get-started/)**）：

```bash
curl -fsSL https://memoh.sh | sudo sh
```

*静默安装（全部默认）：`curl -fsSL ... | sudo sh -s -- -y`*

或手动部署：

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
sudo docker compose up -d
```

> **安装指定版本：**
> ```bash
> curl -fsSL https://memoh.sh | sudo MEMOH_VERSION=v1.0.0 sh
> ```
>
> **使用中国大陆镜像加速：**
> ```bash
> curl -fsSL https://memoh.sh | sudo USE_CN_MIRROR=true sh
> ```
>
> macOS 或用户已在 `docker` 用户组中时，无需 `sudo`。

启动后访问 <http://localhost:8082>。默认登录：`admin` / `admin123`

自定义配置与生产部署请参阅 [DEPLOYMENT.md](DEPLOYMENT.md)。

## 为什么选择 Memoh？

OpenClaw 令人印象深刻，但在稳定性、安全性、配置复杂度和 token 成本上存在明显不足。如果你正在寻找稳定、安全的方案，不妨考虑 Memoh。

Memoh 是基于 Golang 的多 bot agent 服务，提供 bot、Channel、MCP、Skills 等的完整图形化配置。我们使用 Containerd 为每个 bot 提供容器级隔离，并大量借鉴 OpenClaw 的 Agent 设计。

Memoh Bot 能区分并记忆多人与多 bot 的请求，在任意群聊中无缝协作。你可以用 Memoh 组建 bot 团队，或为家人配置账号，用 bot 管理日常家务。

## 特性

- 🤖 **多 Bot 管理**：创建多个 bot；人与 bot、bot 与 bot 可私聊、群聊或协作。支持角色权限控制（owner / admin / member）与所有权转让。
- 👥 **多用户与身份识别**：Bot 可在群聊中区分不同用户，分别记忆每个人的上下文，并支持向特定用户单独发送消息。跨平台身份绑定将同一用户在 Telegram、Discord、飞书、Web 上的身份统一关联。
- 📦 **容器化**：每个 bot 运行在独立的 containerd 容器中，可在容器内自由执行命令、编辑文件、访问网络，宛如各自拥有一台电脑。支持容器快照保存与恢复。
- 🧠 **记忆工程**：多供应商记忆架构 —— 内置（off / sparse / dense 三种模式）、[Mem0](https://mem0.ai)、OpenViking。LLM 驱动的知识抽取，混合检索（稠密语义搜索 + BM25 关键词 + 神经稀疏向量），24 小时上下文加载，记忆压缩与重建，多语言自动检测。
- 💬 **多平台**：支持 Telegram、Discord、飞书(Lark)、Email 及内置 Web/CLI。跨平台统一消息格式，支持富文本、媒体附件、表情回应和流式输出。跨平台身份绑定。
- 📧 **邮件**：多适配器邮件服务（Mailgun、通用 SMTP），支持按 bot 绑定与发信审计日志。Bot 可将邮件作为渠道收发。
- 🔧 **MCP（模型上下文协议）**：完整 MCP 支持（HTTP / SSE / Stdio）。内置容器操作、记忆搜索、网络搜索、定时任务、消息发送等工具，可连接外部 MCP 服务器扩展。
- 🧩 **子代理**：为每个 bot 创建专用子代理，拥有独立上下文与技能，实现多代理协作。
- 🎭 **技能与身份**：通过 IDENTITY.md、SOUL.md 定义 bot 人格，模块化技能文件可在运行时启用/禁用。
- 🌐 **浏览器**：每个 Bot 可拥有独立的无头 Chromium 浏览器（基于 Playwright）。支持页面导航、点击、填写表单、截图（带编号标注的交互元素）、读取无障碍树、多标签页管理等，实现真正的网页自动化与 AI 驱动浏览。
- 🔍 **网络搜索**：内置 12 种搜索提供商 —— Brave、Bing、Google、Tavily、DuckDuckGo、SearXNG、Serper、搜狗、Jina、Exa、Bocha、Yandex，支持网页搜索与 URL 内容抓取。
- ⏰ **定时任务**：基于 Cron 的任务调度，支持最大调用次数限制。Bot 可自主在指定时间执行命令或工具。
- 💓 **心跳**：周期性自主任务，Bot 可按配置间隔执行例行操作（如签到、汇总、监控），并记录执行日志。
- 📥 **收件箱**：跨渠道收件箱，其他渠道的消息会排入收件箱并呈现在系统提示词中，确保 bot 不遗漏上下文。
- 📊 **Token 用量追踪**：按 bot 监控 token 消耗，支持用量统计与可视化。
- 🧪 **多模型**：兼容任何 OpenAI 兼容、Anthropic 或 Google Generative AI 提供商。每个 bot 可独立配置聊天、记忆和嵌入模型。
- 🖥️ **Web 管理界面**：基于 Vue 3 + Tailwind CSS 的现代面板，实时流式聊天、工具调用可视化、聊天内文件管理器、容器文件浏览器，所有配置可视化操作。深色/浅色主题，中英文支持。
- 🚀 **一键部署**：Docker Compose 编排，自动迁移、containerd 初始化与 CNI 网络配置。附带交互式安装脚本。

## 记忆系统

Memoh 的记忆系统围绕 **Memory Provider（记忆供应商）** 构建 —— 可插拔的后端，控制 bot 如何存储、检索和管理长期记忆。

| 供应商 | 说明 |
|--------|------|
| **Built-in（内置）** | 自托管，随 Memoh 附带。三种模式：**Off**（文件索引，无向量搜索）、**Sparse**（本地神经稀疏向量模型，无 API 费用）、**Dense**（基于嵌入模型的语义搜索，需 Qdrant）。 |
| **Mem0** | 通过 [Mem0](https://mem0.ai) API 的 SaaS 记忆服务。 |
| **OpenViking** | 自托管或 SaaS 记忆服务，使用自有 API。 |

每个 bot 绑定一个供应商。聊天过程中，bot 自动从每轮对话中抽取关键事实并存储为结构化记忆。收到新消息时，系统通过混合检索找到最相关的记忆并注入 bot 上下文 —— 实现跨对话的个性化长期记忆。

其他功能包括记忆压缩（合并冗余条目）、重建、手动创建/编辑，以及向量流形可视化（Top-K 分布与 CDF 曲线）。详见[文档](https://docs.memoh.ai/memory-providers/)。

## 图库

<table>
  <tr>
    <td><img src="./assets/gallery/01.png" alt="Gallery 1" width="100%"></td>
    <td><img src="./assets/gallery/02.png" alt="Gallery 2" width="100%"></td>
    <td><img src="./assets/gallery/03.png" alt="Gallery 3" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">与 Bot 聊天</strong></td>
    <td><strong text-align="center">容器与 Bot 管理</strong></td>
    <td><strong text-align="center">提供商与模型配置</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/04.png" alt="Gallery 4" width="100%"></td>
    <td><img src="./assets/gallery/05.png" alt="Gallery 5" width="100%"></td>
    <td><img src="./assets/gallery/06.png" alt="Gallery 6" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">容器文件管理器</strong></td>
    <td><strong text-align="center">定时任务</strong></td>
    <td><strong text-align="center">Token 用量追踪</strong></td>
  </tr>
</table>

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go, Echo, sqlc, Uber FX, pgx/v5, containerd v2 |
| Agent 网关 | Bun, Elysia |
| 浏览器网关 | Bun, Elysia, Playwright (Chromium) |
| 前端 | Vue 3, Vite, Pinia, Tailwind CSS, Reka UI |
| 存储 | PostgreSQL, Qdrant |
| 基础设施 | Docker, containerd, CNI |
| 工具链 | mise, pnpm, swaggo, sqlc |

## 架构

```
┌──────────────────┐  ┌─────────────────┐  ┌──────────────┐
│     Channels     │  │      Web UI     │  │   CLI        │
│ (TG/DC/FS/Email) │  │  (Vue 3 :8082)  │  │              │
└────────┬─────────┘  └────────┬────────┘  └──────┬───────┘
         │                     │                  │
         ▼                     ▼                  ▼
┌──────────────────────────────────────────────────────────┐
│                   Server (Go :8080)                       │
│  Auth · Bots · Channels · Memory · Containers · MCP      │
└──────────────────────┬───────────────────────────────────┘
                       │
           ┌───────────┼───────────┬───────────┐
           ▼           ▼           ▼           ▼
     ┌──────────┐ ┌─────────┐ ┌──────────────────┐ ┌───────────────────┐
     │ PostgreSQL│ │ Qdrant  │ │ Agent Gateway     │ │ Browser Gateway    │
     │          │ │ (向量库) │ │ (Bun/Elysia :8081)│ │ (Playwright :8083) │
     └──────────┘ └─────────┘ └────────┬──────────┘ └───────────────────┘
                                       │
                               ┌───────┼───────┐
                               ▼       ▼       ▼
                          ┌─────┐ ┌─────┐ ┌─────┐
                          │Bot A│ │Bot B│ │Bot C│  ← containerd
                          └─────┘ └─────┘ └─────┘
```

## 路线图

详见 [Roadmap](https://github.com/memohai/Memoh/issues/86)。

## 开发

开发环境配置请参阅 [CONTRIBUTING.md](CONTRIBUTING.md)。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=memohai/Memoh&type=date&legend=top-left)](https://www.star-history.com/#memohai/Memoh&type=date&legend=top-left)

## Contributors

<a href="https://github.com/memohai/Memoh/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=memohai/Memoh" />
</a>

**LICENSE**: AGPLv3

Copyright (C) 2026 Memoh. All rights reserved.
