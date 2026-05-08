# Telegram

把 Memoh 机器人接到 Telegram。Memoh 对 TG 支持较全：流式、Markdown、附件等。

## 1. 在 Telegram 建 Bot

先要有一个 **API Token**。

1. 在 Telegram 里搜官方 **@BotFather**。
2. 发 `/newbot`。
3. 按提示填：
   - **Name**：展示名，如 `My Memoh Bot`。
   - **Username**：全局唯一、以 `bot` 结尾，如 `my_memoh_bot`。
4. BotFather 会给 **API Token**（形如 `123456789:ABC...`）。**不要泄露。**

> 官方说明：[Telegram Bot Tutorial](https://core.telegram.org/bots/tutorial)

## 2. 在 Memoh 里填

1. 网页里打开机器人 **详情** → **Platforms**。
2. **Add Channel** → 选 **Telegram**。
3. 把 **API Token** 填进凭据。
4. **Save and Enable**。

## 支持的能力

- **流式**：边生成边出字。
- **Markdown**：粗体、斜体、代码块、链接等。
- **附件**：收图/文件，机器人也可发文件。
- **回复链**：跟贴上下文能用于推理。
