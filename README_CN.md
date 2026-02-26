<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" width="100" height="100">
  <h1>Memoh</h1>
  <p>多用户、结构化记忆、容器化的 AI Agent 系统。</p>
  <p>📌 <a href="https://docs.memoh.ai/blogs/2026-02-16.html">Introduction to Memoh - The Case for an Always-On, Containerized Home Agent</a></p>
  <div align="center">
    <img src="https://img.shields.io/github/package-json/v/memohai/Memoh" alt="Version" />
    <img src="https://img.shields.io/github/license/memohai/Memoh" alt="License" />
    <img src="https://img.shields.io/github/stars/memohai/Memoh?style=social" alt="Stars" />
    <img src="https://img.shields.io/github/forks/memohai/Memoh?style=social" alt="Forks" />
    <img src="https://img.shields.io/github/last-commit/memohai/Memoh" alt="Last Commit" />
    <img src="https://img.shields.io/github/issues/memohai/Memoh" alt="Issues" />
    <img src="https://github.com/memohai/Memoh/actions/workflows/docker.yml/badge.svg" alt="Docker" />
  </div>
  <div align="center">
    [<a href="https://t.me/memohai">Telegram 群组</a>]
    [<a href="https://docs.memoh.ai">文档</a>]
    [<a href="mailto:business@memoh.net">合作</a>]
  </div>
  <hr>
</div>

Memoh 是一个常驻运行的容器化 AI Agent 系统。你可以创建多个 AI 机器人，每个机器人运行在独立的容器中，拥有持久化记忆，并通过 Telegram、Discord、飞书(Lark) 或内置的 Web/CLI 与之交互。机器人可以执行命令、编辑文件、浏览网页、通过 MCP 调用外部工具，并记住一切 —— 就像给每个 Bot 一台自己的电脑和大脑。

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

> 若镜像拉取较慢，可使用中国大陆镜像源配置：
```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml up -d
```

> macOS 或用户已在 `docker` 用户组中时，无需 `sudo`。

启动后访问 <http://localhost:8082>。默认登录：`admin` / `admin123`

自定义配置与生产部署请参阅 [DEPLOYMENT.md](DEPLOYMENT.md)。

## 为什么选择 Memoh？

OpenClaw、Clawdbot、Moltbot 固然出色，但在稳定性、安全性、配置复杂度与 token 成本上仍有不足。若你正在寻找稳定、安全的 Bot SaaS 方案，不妨考虑我们的开源 Memoh。

Memoh 是基于 Golang 的多 bot agent 服务，提供 bot、Channel、MCP、Skills 等的完整图形化配置。我们使用 Containerd 为每个 bot 提供容器级隔离，并大量借鉴 OpenClaw 的 Agent 设计。

Memoh Bot 能区分并记忆多人与多 bot 的请求，在任意群聊中无缝协作。你可以用 Memoh 组建 bot 团队，或为家人配置账号，用 bot 管理日常家务。

## 特性

- 🤖 **多 Bot 管理**：创建多个 bot；人与 bot、bot 与 bot 可私聊、群聊或协作。支持角色权限控制（owner / admin / member）与所有权转让。
- 👥 **多用户与身份识别**：Bot 可在群聊中区分不同用户，分别记忆每个人的上下文，并支持向特定用户单独发送消息。跨平台身份绑定将同一用户在 Telegram、Discord、飞书、Web 上的身份统一关联。
- 📦 **容器化**：每个 bot 运行在独立的 containerd 容器中，可在容器内自由执行命令、编辑文件、访问网络，宛如各自拥有一台电脑。支持容器快照保存与恢复。
- 🧠 **记忆工程**：混合检索（稠密向量搜索 + BM25 关键词搜索），LLM 驱动的知识抽取。默认加载最近 24 小时上下文，支持记忆压缩与重建。
- 💬 **多平台**：支持 Telegram、Discord、飞书(Lark) 及内置 Web/CLI。跨平台统一消息格式，支持富文本、媒体附件、表情回应和流式输出。跨平台身份绑定。
- 🔧 **MCP（模型上下文协议）**：完整 MCP 支持（HTTP / SSE / Stdio）。内置容器操作、记忆搜索、网络搜索、定时任务、消息发送等工具，可连接外部 MCP 服务器扩展。
- 🧩 **子代理**：为每个 bot 创建专用子代理，拥有独立上下文与技能，实现多代理协作。
- 🎭 **技能与身份**：通过 IDENTITY.md、SOUL.md 定义 bot 人格，模块化技能文件可在运行时启用/禁用。
- 🔍 **网络搜索**：可配置搜索提供商（Brave Search 等），支持网页搜索与 URL 内容抓取。
- ⏰ **定时任务**：基于 Cron 的任务调度，支持最大调用次数限制。Bot 可自主在指定时间执行命令或工具。
- 📥 **收件箱**：跨渠道收件箱，其他渠道的消息会排入收件箱并呈现在系统提示词中，确保 bot 不遗漏上下文。
- 🧪 **多模型**：兼容任何 OpenAI 兼容、Anthropic 或 Google Generative AI 提供商。每个 bot 可独立配置聊天、记忆和嵌入模型。
- 🖥️ **Web 管理界面**：基于 Vue 3 + Tailwind CSS 的现代面板，实时流式聊天、工具调用可视化、容器文件浏览器，所有配置可视化操作。深色/浅色主题，中英文支持。
- 🚀 **一键部署**：Docker Compose 编排，自动迁移、containerd 初始化与网络配置，一条命令启动全栈。附带交互式安装脚本。

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go, Echo, sqlc, Uber FX, pgx/v5, containerd v2 |
| Agent 网关 | Bun, Elysia |
| 前端 | Vue 3, Vite, Pinia, Tailwind CSS, Reka UI |
| 存储 | PostgreSQL, Qdrant |
| 基础设施 | Docker, containerd, CNI |
| 工具链 | mise, pnpm, swaggo, sqlc |

## 架构

```
┌─────────────┐    ┌─────────────────┐    ┌──────────────┐
│   Channels   │    │    Web UI        │    │   CLI        │
│  (TG/DC/FS)  │    │  (Vue 3 :8082)  │    │              │
└──────┬───────┘    └────────┬────────┘    └──────┬───────┘
       │                     │                     │
       ▼                     ▼                     ▼
┌──────────────────────────────────────────────────────────┐
│                   Server (Go :8080)                       │
│  Auth · Bots · Channels · Memory · Containers · MCP      │
└──────────────────────┬───────────────────────────────────┘
                       │
           ┌───────────┼───────────┐
           ▼           ▼           ▼
     ┌──────────┐ ┌─────────┐ ┌──────────────────┐
     │ PostgreSQL│ │ Qdrant  │ │ Agent Gateway     │
     │          │ │ (向量库) │ │ (Bun/Elysia :8081)│
     └──────────┘ └─────────┘ └────────┬──────────┘
                                       │
                               ┌───────┼───────┐
                               ▼       ▼       ▼
                          ┌─────┐ ┌─────┐ ┌─────┐
                          │Bot A│ │Bot B│ │Bot C│  ← containerd
                          └─────┘ └─────┘ └─────┘
```

## 路线图

详见 [Roadmap Version 0.1.0](https://github.com/memohai/Memoh/issues/86)。

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
