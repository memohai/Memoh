<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" height="80">
  <h1>Memoh</h1>
  <p>可自托管、常在线的容器化 AI 智能体编排</p>
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
    <a href="https://t.me/memohai">
      <img src="https://img.shields.io/badge/Telegram-Group-26A5E4?logo=telegram&logoColor=white" alt="Telegram" />
    </a>
    <a href="https://docs.memoh.ai">
      <img src="https://img.shields.io/badge/Docs-memoh.ai-3eaf7c?logo=readthedocs&logoColor=white" alt="Documentation" />
    </a>
  </div>
</div>

**Memoh（/ˈmemoʊ/）** 是一套常在线的容器化 AI 智能体编排。你可以建多个机器人，各跑在独立容器里、带持久记忆，在 Telegram、Discord 等渠道里跟它们聊。机器人能跑命令、改文件、逛网页、通过 MCP 接外部工具，并记住聊过的内容——就像给每个机器人各配了一台电脑和一份持续的记忆。

## 快速开始

一键安装（**需先装 [Docker](https://www.docker.com/get-started/)**）：

```bash
curl -fsSL https://memoh.sh | sh
```

*全部默认、静默安装：`curl -fsSL ... | sh -s -- -y`*

或手动：

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
# 编辑 config.toml
docker compose up -d
```

Kubernetes 部署（**默认使用 Kubernetes workspace**）：

```bash
kubectl apply -k deploy/kubernetes
```

> **安装指定版本：**
> ```bash
> curl -fsSL https://memoh.sh | MEMOH_VERSION=v0.6.0 sh
> ```
>
> **镜像拉取慢时可用国内镜像：**
> ```bash
> curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
> ```
>
> 不要对整个安装脚本用 `sudo`。需要时脚本内部会自行调用 `sudo docker`。在 macOS 上，或用户已在 `docker` 组时，连 Docker 也不必 sudo。

启动后打开 <http://localhost:8082>。默认账号：`admin` / `admin123`

自定义与生产环境见 [DEPLOYMENT.md](DEPLOYMENT.md)。

文档入口：

- [关于 Memoh](https://docs.memoh.ai/about)
- [提供方与模型](https://docs.memoh.ai/getting-started/provider-and-model)
- [机器人设置](https://docs.memoh.ai/getting-started/bot)
- [会话与讨论模式](https://docs.memoh.ai/getting-started/sessions)
- [渠道](https://docs.memoh.ai/getting-started/channels)
- [技能](https://docs.memoh.ai/getting-started/skills)
- [应用超市](https://docs.memoh.ai/getting-started/supermarket)
- [斜杠命令](https://docs.memoh.ai/getting-started/slash-commands)

## 为什么选 Memoh？

设计取向是**常连不断**：AI 一直在线，数据留在你手里。

- **轻、快**：适合家里或小工作室当基础设施，在边缘设备上也能跑得动。
- **默认容器化**：每个机器人有独立容器，自带文件系统、网络与工具环境。
- **算力与数据拆着用**：能力可以走云上大模型，记忆和索引以本地为主，隐私更好拆。
- **多用户设计**：用户之间、机器人和用户之间，分享和隐私有明确边界。
- **全图形化配置**：机器人、渠道、MCP、技能、各项设置都在网页里配，不必写代码。

## 功能概览

### 核心

- 🤖 **多机多人**：多个机器人，可私聊、可群聊、可互相对话。群聊里能区分不同用户、各自记上下文，并支持跨平台身份绑定。
- 📦 **容器化**：每个机器人在自己的 containerd 容器里，独立盘与网——和独占一台小机器差不多。支持快照、数据导入导出与版本管理。
- 🗂️ **持久化文件**：每个机器人有可写的 home 目录，重启、升级、迁移不丢。机器人可自由读写、整理文件；你也可在网页文件管理器里浏览、上传、下载、编辑。
- 🧠 **记忆工程**：由 LLM 做事实抽取，混合检索（稠密 + 稀疏 + BM25），可按提供方接长期记忆，有记忆整理与会话级上下文整理。可插后端：内置（关/稀疏/稠密）、[Mem0](https://mem0.ai)、OpenViking。
- 💬 **渠道多**：Telegram、Discord、飞书、QQ、Matrix、Misskey、钉钉、企业微信、微信、公众号、邮件（Mailgun / SMTP / Gmail OAuth），以及自带 Web 界面。

### 智能体能力

- 🔧 **MCP（Model Context Protocol）**：支持 HTTP / SSE / Stdio / OAuth。可接外部工具服务；每个机器人自己管自己的 MCP 连接。
- 🌐 **浏览器自动化**：Playwright 驱动无头 Chromium/Firefox，可导航、点击、填表、截屏、读可访问性树、管多标签。
- 🎭 **技能、应用超市与子智能体**：用模块化技能描述行为，从应用超市装整理好的技能与 MCP 模板，重活可交给有独立上下文的子智能体。
- 💭 **会话与讨论模式**：聊天、讨论、定时、心跳、子智能体等会话，可用斜杠命令，并查看会话状态。
- ⏰ **自动化**：基于 Cron 的定时任务，以及周期心跳，让机器人能自主活动。

### 管理

- 🖥️ **Web 界面**：现代表盘（Vue 3 + Tailwind）——流式聊天、工具调用展示、文件管理、全套可视化配置。深色/浅色、多语言。
- 🔐 **访问控制**：基于优先级的 ACL，有预设、允许/拒绝、可按渠道身份、渠道类型或会话作用域配置。
- 🧪 **多模型**：OpenAI 兼容、Anthropic、Google、OpenAI Codex、GitHub Copilot、Edge TTS 等。可按机器人选模型、提供方 OAuth、自动拉模型列表。
- 🎙️ **语音与转写**：机器人可经 10+ 家 TTS（Edge、OpenAI、ElevenLabs、Deepgram、Azure、Google、MiniMax、火山、阿里、OpenRouter 等）发声；从 Telegram、Discord 等收到语音会可用 STT（OpenAI / OpenRouter）自动转写，也可用内置工具按需转任意音频。
- 🚀 **一键部署**：Docker Compose，含自动迁移、containerd 与 CNI 网络。

## 记忆系统

开箱带一套**可完全自托管的记忆引擎**，不依赖外部 API、不必绑 SaaS。每个机器人会跨会话、跨天、跨平台记住你告诉它的事；群聊里会按用户分开记，不会把你和别人混在一起。

### 内置记忆（默认）

三种模式，在网页上按机器人切换：

| 模式 | 后端 | 适合什么场景 |
|------|------|-------------|
| **关** | 仅文件，不做向量检索 | 小范围试用、排错、或想尽量少动件 |
| **稀疏** | 本机小模型出神经稀疏向量 + BM25 | 无 API 费用、全在本地跑，短事实类回忆效果不错 |
| **稠密** | 向量模型 + 数据库向量索引 | 按语义找记忆，不只靠关键词 |

实现上大致包括：

- **由 LLM 做事实抽取**：每轮对话会解析、去重，存成结构化记忆，不是堆原始整段话。
- **混合检索**：稠密、稀疏与 BM25 一起参与再排序，于是「某 API key 是什么」（偏字面）和「上周说的那个项目」（偏语义）都能用得上。
- **记忆整理**：用 LLM 周期合并冗余或过时的条目，索引体量可控，召回更稳。
- **可检查、可改**：浏览、搜索、手建/手改记忆、整库重建，还能在页面上看向量流形可视化（Top-K 与 CDF 等）。

### 其他提供方

若想接现成的记忆服务，Memoh 也支持把 [**Mem0**](https://mem0.ai)（SaaS）和 **OpenViking**（自管或 SaaS）换进去，绑定和聊天体验一样，只换存储后端。

完整说明见[文档](https://docs.memoh.ai/memory-providers/)。

## 图集

<table>
  <tr>
    <td><img src="./assets/gallery/01.png" alt="图集 1" width="100%"></td>
    <td><img src="./assets/gallery/02.png" alt="图集 2" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">聊天</strong></td>
    <td><strong text-align="center">容器</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/03.png" alt="图集 3" width="100%"></td>
    <td><img src="./assets/gallery/04.png" alt="图集 4" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">提供方</strong></td>
    <td><strong text-align="center">文件管理</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/05.png" alt="图集 5" width="100%"></td>
    <td><img src="./assets/gallery/06.png" alt="图集 6" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">定时任务</strong></td>
    <td><strong text-align="center">Token 用量</strong></td>
  </tr>
</table>


## 为本项目拆出的子项目

- [**Twilight AI**](https://github.com/memohai/twilight-ai) — 给 Go 用的轻量、惯用 AI SDK，风格参考 [Vercel AI SDK](https://sdk.vercel.ai/)。与提供方解耦（OpenAI、Anthropic、Google），流式、工具调用、MCP、嵌入一等公民。

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
