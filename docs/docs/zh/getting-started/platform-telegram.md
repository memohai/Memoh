# 配置 Telegram 接入平台

本指南将引导你将 Bot 连接到 Telegram，让用户通过 Telegram 消息与你的 Bot 聊天。

## 前提条件

- Memoh 已运行（参见 [Docker 安装](/zh/installation/docker)）
- 已登录 Web UI：http://localhost:8082
- 已创建 Bot（参见[创建 Bot](/zh/getting-started/create-bot)）
- 一个 Telegram 账号

## 第 1 步：创建 Telegram Bot

打开 Telegram，搜索官方 Bot `@BotFather`。

发送 `/newbot` 命令给 BotFather，按提示操作：

1. 输入 Bot 的**名称**（显示名称，如 `My Memoh Bot`）
2. 输入 Bot 的**用户名**（必须以 `bot` 结尾，如 `my_memoh_bot`）

BotFather 将创建 Bot 并提供一个 **Bot Token**（如 `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`）。

**请妥善保存此 Token** ——下一步需要用到。

## 第 2 步：打开 Bot 平台页面

在 Memoh Web UI 中，点击左侧边栏的 **Bots** 打开 Bot 页面。

选择要连接 Telegram 的 Bot。

点击 **Platforms** 标签页，打开接入平台配置页面。

## 第 3 步：添加 Telegram 接入平台

点击 **Add Channel** 按钮。

在对话框中选择 **Telegram** 作为接入平台类型。

填写配置：

| 字段 | 说明 |
|------|------|
| **Bot Token** | 来自 BotFather 的 Token（如 `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`） |

点击 **Save** 添加接入平台。

![添加接入平台按钮](/getting-started/platform-telegram-01-platforms.png)



## 第 4 步：绑定你的 Telegram 账号

打开 Memoh Web UI 设置页面，找到 `Bind Code` 部分，选择 Telegram 平台和所需的 TTL（秒），生成绑定码。

![绑定码](/getting-started/platform-telegram-02-bindcode.png)


在 Telegram 中打开 Bot 对话，发送绑定码，成功后你将收到 `Binding successful! Your identity has been linked.` 消息。


点击 **Save** 完成绑定。

## 第 5 步：测试连接

在 Telegram 上给你的 Bot 发送消息：

- **公开 Bot**（`public`）：将 Bot 添加到群组，其他成员在发送消息时 @ 你的 Bot。
- **个人 Bot**（`person`）：直接发送私信（需要先完成绑定）

Bot 将根据配置的模型和系统提示词进行回复。

## 下一步

- 配置[记忆](/zh/concepts/memory)，为 Bot 启用长期记忆
- 设置[技能](/zh/concepts/skills)，扩展 Bot 的能力
- 添加[定时任务](/zh/concepts/schedule)，自动执行任务
