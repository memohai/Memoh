# 了解 Memoh

## Memoh 是什么

Memoh 是面向多成员、结构化长期记忆和独立 workspace 的 AI 智能体平台。你可以创建多个机器人，为每个机器人提供自己的工作区和长期记忆，通过 Telegram、Discord、飞书、QQ、Matrix、Misskey、钉钉、企微、微信、微信公众号、邮件或自带网页界面使用。

每个机器人都有自己的运行时、工具、记忆、渠道和访问策略。按部署方式不同，workspace 可以是隔离容器，也可以是明确信任的本地 workspace。

## 两种分发方式

### Desktop 桌面版

Desktop 适合个人和本地使用。它会在 `127.0.0.1:18731` 启动本地 `memoh-server`，管理本地 SQLite 数据，启动用于记忆检索的 embedded Qdrant，打包 `memoh` CLI，并负责系统托盘里的唤起与退出流程。

想在自己的电脑上快速试用 Memoh，Desktop 是最短路径。

### Server Deploy

Server Deploy 适合长期在线和多人共享。只要 Memoh 需要服务多个用户、部署在服务器上持续运行，或在你的个人电脑离线时继续接入外部渠道，就应该用这一形态。

Server stack 用 Docker Compose 跑起来，包含后端、网页端、数据库、记忆服务和 workspace runtime。

## 和其它方案不一样在哪

### 多机器人、多用户

- 一个账号里可建多个机器人，分角色或分场景用。
- 私聊、群聊、委派流程里，人和机器人都可参与。
- 共享对话里能区分不同用户；跨渠道绑定身份后，同一个人可被稳定识别。

### 独立 Workspace

每个机器人可以有自己的 workspace，用来放文件、跑命令、托管 MCP 和执行长期任务。Server Deploy 通常通过 Docker、containerd 或 Apple 后端提供容器 workspace。Desktop/local 模式也可以启用 trusted local workspace，让机器人在明确受信任的本机路径里工作。

容器 workspace 还能提供完整图形桌面，通过 VNC/RFB 作为显示与输入基础，并运行有头 Chrome/Chromium。这样就能处理很多纯 headless 自动化不可靠的网站和登录流程。

### Browser Use 与 Computer Use

Memoh 把浏览器和 GUI 操作分成几层：

- **Headless browser 命令**：作为普通 workspace 命令运行。
- **Browser Use**：通过 CDP 操作 workspace 里的有头浏览器。
- **Computer Use**：通过截图、鼠标和键盘输入操作更完整的 workspace 桌面。

网页内操作优先用 Browser Use；原生弹窗、非浏览器应用或 CDP 够不到的 GUI 状态，再用 Computer Use。

### 长期记忆与会话负担

这是两件不同的事：

- **长期记忆**通过各记忆提供方存事实、跨会话检索。
- **会话上下文压缩**是在当前对话太长时，用摘要缩小活跃窗口。

注意：压缩会话上下文，改的是当前对话窗口；记忆压缩改的是存下来的记忆条目本身。

### 会话与 Discuss 模式

每个机器人有多路 **会话**，自带上下文。常见五类：

- **Chat**：普通面向人的对话。
- **Discuss**：偏观察；模型先组织判断，只有用发送类动作时才真正对频道说话。
- **Heartbeat**：按间隔自动跑的任务会话。
- **Schedule**：由 cron 触发的任务会话。
- **Subagent**：委派子智能体时产生的会话。

在渠道里可以用 `/new` 等切会话，网页端也有会话侧栏，可看上下文占用、缓存命中、用到的技能等。

### 渠道覆盖面

统一的渠道适配让一个机器人能同时在多个地方被叫到。当前可对接 Telegram、Discord、飞书、QQ、Matrix、Misskey、钉钉、企微、微信、公众号、邮件和 Web。

**个人微信扫码**与**公众号 Webhook** 是两套不同适配，别混用。

### 工具、技能、MCP、超市

内置能力包括：网页搜索与拉取、workspace 文件编辑和命令执行、Browser Use、Computer Use、记忆检索与管理、发消息/邮件、TTS、子智能体、可复用 **技能** 模块、外部 **MCP** 服务，以及从 **超市** 装技能和 MCP 模板。

### 供应商与模型

支持多种对接方式，例如 OpenAI Chat/Responses、Anthropic Messages、Google、Codex、GitHub Copilot、Edge 朗读等。模型按 **chat / embedding / speech** 分角色。文生图走兼容的 chat/图像能力，不单独做一层“图像供应商系统”。

### 运维与界面

网页端尽量把日常事做完：机器人各 tab、供应商与模型、会话里即时压缩与状态、workspace 文件/终端/显示、技能显隐、渠道里用斜杠命令。不必天天手改配置文件。

## 从哪开始

- [安装选择](/zh/installation/) — 先选 Desktop 还是 Server Deploy
- [供应商与模型](/zh/getting-started/provider-and-model) — 配好模型访问
- [机器人](/zh/getting-started/bot) — 创建并配置
- [Browser / Computer Use](/zh/getting-started/browser-computer-use) — 了解有头浏览器与桌面操作
- [渠道](/zh/getting-started/channels) — 选机器人出现的位置
- [技能](/zh/getting-started/skills)、[超市](/zh/getting-started/supermarket) — 扩展能力
