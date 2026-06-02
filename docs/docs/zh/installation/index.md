# 安装选择

Memoh 有两种分发方式：面向个人/本地使用的 Desktop 桌面版，以及面向长期在线和多人使用的 Server Deploy。先选对形态，再看具体安装步骤。

| 场景 | 选择 | 原因 |
|------|------|------|
| 个人电脑、本地记忆、快速试用、单用户工作流 | [Desktop 桌面版](/zh/installation/desktop) | App 会自己启动本地服务、embedded Qdrant、本地存储和 bundled CLI。 |
| 共享服务器、远程访问、生产长期在线、对接 Telegram/Discord/飞书/微信/邮件等渠道 | [Server Deploy](/zh/installation/docker) | Docker Compose 栈会持续运行后端、网页端、数据库、记忆服务和 workspace runtime。 |

## Desktop 桌面版

Desktop 适合把 Memoh 当成本地应用使用。它会管理 `127.0.0.1:18731` 上的本地 `memoh-server`，准备本地存储，启动 embedded Qdrant，并自动连接界面。

如果机器人需要在你的电脑离线时继续服务外部渠道，请改用 Server Deploy。

## Server Deploy

Server Deploy 适合多人、远程、长期在线或多租户场景。机器人需要持续接入 Telegram、Discord、飞书、微信、公众号、邮件等外部渠道时，也应该用这一形态。

一般从 [Server Deploy](/zh/installation/docker) 的 Docker Compose 部署开始。

## 相关页面

- [Workspace backend](/zh/installation/workspace-backends) 解释 Docker、containerd、Apple 和本地 workspace 的差异。
- [SQLite 部署](/zh/installation/sqlite) 适合更轻量的单节点 server 部署。
