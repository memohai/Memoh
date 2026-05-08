# QQ

走 QQ 开放平台的官方机器人，可和用户在 QQ 里互动。

## 1. 建 QQ 机器人

1. 打开 [QQ 机器人开放平台](https://q.qq.com/qqbot/openclaw/)，用 QQ 登录。
2. **创建机器人**（创建机器人），一般无需审批；每号最多约 5 个。
3. 记下 **AppID**、**AppSecret**。

> **注意**：AppSecret 往往只显示一次，存好。再看一次常会被迫重置。

## 2. 在 Memoh 里填

1. 机器人 **Platforms** → **Add Channel** → **QQ**。
2. 填 **AppID**、**AppSecret**。
3. 可选：
   - **Markdown Support**：开 Markdown（默认多开）。
   - **Enable Input Hint**：是否显示「正在输入」类提示（默认多开）。
4. **Save and Enable**。

## 支持的能力

- 文本、**Markdown**、**附件**。
- **正在输入**类提示（可关）。
- 场景：C2C 私聊、群、频道等（以平台与版本为准）。

## 参考

- [QQ 机器人开放平台](https://q.qq.com/)
- [QQ 机器人文档](https://bot.q.qq.com/wiki/)
