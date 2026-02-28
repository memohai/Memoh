# 创建 Bot


## 前提条件

- 完成 Provider 和模型配置。


## 第 1 步：打开 Bot 页面

点击左侧边栏的 **Bots** 打开 Bot 配置页面。


![Bots 页面 - 侧边栏](/getting-started/bots-01-sidebar.png)


## 第 2 步：创建 Bot

点击 **New Bot** 按钮（带加号图标）

![创建 Bot 按钮](/getting-started/bots-02-create-bot.png)

在对话框中填写：

| 字段 | 说明 |
|------|------|
| **Name** | Bot 的显示名称（如 `my-bot`、`telegram-public-bot`） |
| **Avatar URL** | 头像 URL（如 `https://gravatar.com/avatar/***`） |
| **Type** | Bot 类型：`person`（个人 Bot，绑定用户使用）、`public`（公开 Bot，用于接入平台群聊，如 Telegram 群组、QQ 群、Discord 频道） |

## 第 3 步：Bot 配置

在 Bots 页面点击一个 **Bot** 卡片

![Bot 配置](/getting-started/bots-03-config.png)

打开 **settings** 部分

![设置](/getting-started/bots-04-setting.png)

选择可用的 `Chat Model`、`Memory Model`、`Embedding Model` 并保存，完成基础配置。


## 第 4 步：测试 Bot

点击左侧边栏的 **Chat** 打开聊天页面，
然后输入提示词测试 Bot 配置。

![聊天测试](/getting-started/bots-05-chat.png)



## 下一步

- 添加[接入平台](/zh/concepts/channel)（如 [Telegram](/zh/getting-started/platform-telegram)、飞书、微信、Discord）
- 管理 Bot 的[记忆](/zh/concepts/memory)
