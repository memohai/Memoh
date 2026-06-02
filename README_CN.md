<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" height="80">
  <h1>Memoh</h1>
  <p>开源的多智能体平台<br>
  每个 Agent 都拥有独立的电脑、桌面、网络与长期记忆</p>
  <div align="center">
    <img src="https://img.shields.io/github/package-json/v/memohai/Memoh" alt="Version" />
    <img src="https://img.shields.io/github/stars/memohai/Memoh?style=social" alt="Stars" />
    <img src="https://img.shields.io/github/forks/memohai/Memoh?style=social" alt="Forks" />
    <a href="https://deepwiki.com/memohai/Memoh">
      <img src="https://deepwiki.com/badge.svg" alt="DeepWiki" />
    </a>
    <a href="https://t.me/memohai">
      <img src="https://img.shields.io/badge/Telegram-Group-26A5E4?logo=telegram&logoColor=white" alt="Telegram" />
    </a>
  </div>
  <h3>
    <a href="https://memoh.ai/desktop">下载 Memoh Desktop</a> · <a href="#部署到服务器">部署到服务器</a> · <a href="https://memoh.ai">官网</a> · <a href="https://x.com/memoh_ai">X</a> · <a href="https://docs.memoh.ai">文档</a>
  </h3>
  <img src="./assets/hero.png" alt="Memoh" width="1000">
</div>

## Memoh 是什么？

Memoh 让你在一台机器上运行多个 AI Agent。每个 Agent 都拥有独立的容器环境、文件系统、桌面、浏览器、网络和长期记忆，像一台真正属于它的电脑。

你可以通过 Telegram、Discord、飞书、微信、Web UI 等渠道和它们对话；它们可以记住上下文、操作浏览器、调用 MCP 工具、执行定时任务。

给自己跑一个，给团队成员各分配一个，或在一台机器上同时运行一组专用 Agent。

## 快速开始

### Desktop

[下载 Memoh Desktop](https://memoh.ai/desktop)

### 部署到服务器

```bash
curl -fsSL https://memoh.sh | sh
```

<details>
<summary><strong>更多部署选项</strong></summary>

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
# 编辑 config.toml
docker compose up -d
```

> **镜像拉取慢时可用国内镜像：**
> ```bash
> curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
> ```
>
> 不要对整个安装脚本用 `sudo`。需要时脚本内部会自行调用 `sudo docker`。在 macOS 上，或用户已在 `docker` 组时，连 Docker 也不必 sudo。

自定义与生产环境见 [DEPLOYMENT.md](DEPLOYMENT.md)。

</details>

## 为什么选 Memoh？

- **每个 Agent 一台电脑**：独立容器，自带文件系统、网络、桌面和浏览器
- **多用户、多机器人**：给自己跑一个，给家人各部署一个，在一台机器上同时跑一群
- **轻量**：边缘设备也能跑，算力走云端，数据留本地

## 功能概览

### 核心

- **多机多人**：多个机器人，可私聊、可群聊、可互相对话，支持跨平台身份绑定
- **容器化 Workspace**：每个机器人运行在独立容器里，拥有自己的文件系统、网络、工具和桌面环境
- **内置记忆**：跨会话、跨平台的长期记忆，开箱即用，也支持接入 [Mem0](https://mem0.ai)、OpenViking
- **十余种渠道**：Telegram、Discord、飞书、微信、QQ、邮件等

### 智能体能力

- **MCP**：接入外部工具服务，每个机器人独立管理连接
- **Browser Use**：在容器内驱动浏览器
- **Computer Use**：操作容器桌面，处理需要 GUI 的工作流
- **技能与应用超市**：模块化技能，从超市安装模板，重活交给子智能体
- **自动化**：定时任务与周期心跳

## 记忆系统

开箱带一套可完全自托管的记忆引擎，每个机器人会跨会话、跨天、跨平台记住你告诉它的事

也支持接入 [**Mem0**](https://mem0.ai) 和 **OpenViking** 等外部记忆服务，完整说明见[文档](https://docs.memoh.ai/memory-providers/)

## 为本项目拆出的子项目

- [**Twilight AI**](https://github.com/memohai/twilight-ai) — 给 Go 用的轻量 AI SDK，风格参考 [Vercel AI SDK](https://sdk.vercel.ai/)

## 项目状态

![License](https://img.shields.io/github/license/memohai/Memoh) ![Last Commit](https://img.shields.io/github/last-commit/memohai/Memoh) ![Commit Activity](https://img.shields.io/github/commit-activity/m/memohai/Memoh) ![Issues](https://img.shields.io/github/issues/memohai/Memoh) ![Pull Requests](https://img.shields.io/github/issues-pr/memohai/Memoh)

## Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=memohai/Memoh&type=date&legend=top-left)](https://www.star-history.com/#memohai/Memoh&type=date&legend=top-left)

## 贡献者

<a href="https://github.com/memohai/Memoh/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=memohai/Memoh" />
</a>

## 社区

- 🌐 [**网站**](https://memoh.ai)
- 📚 [**文档**](https://docs.memoh.ai) — 安装、概念与指南
- 🤝 [**合作**](mailto:business@memoh.net) — business@memoh.net
- 💬 [**Telegram 群组**](https://t.me/memohai) — 交流与支持
- 🛒 [**应用超市**](https://github.com/memohai/supermarket) — 整理好的技能与 MCP 模板

---

**许可证**：AGPLv3

Made with ❤️ by MemohAI Team,

Copyright (C) 2026 MemohAI (memoh.ai). All rights reserved.
