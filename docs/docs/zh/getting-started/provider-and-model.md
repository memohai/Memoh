# 配置 Provider 和模型

在创建 Bot 之前，你需要至少配置一个 LLM Provider，并添加聊天模型和嵌入模型。本指南将引导你通过 Web UI 完成配置。

## 前提条件

- Memoh 已运行（参见 [Docker 安装](/zh/installation/docker)）
- 已登录 Web UI：http://localhost:8082

## 第 1 步：打开模型页面

点击左侧边栏的 **Models** 打开 Provider 和模型配置页面。

![Models 页面 - 侧边栏](/getting-started/provider-model-01-sidebar.png)

页面分为两个面板：
- **左侧**：Provider 列表和搜索
- **右侧**：选中 Provider 的详情和模型（未选择时为空状态）

## 第 2 步：添加 Provider

点击左侧面板底部的 **Add Provider** 按钮（带加号图标）。

![添加 Provider 按钮](/getting-started/provider-model-02-add-provider.png)

在对话框中填写：

| 字段 | 说明 |
|------|------|
| **Name** | Provider 的显示名称（如 `my-openai`、`ollama-local`） |
| **API Key** | 你的 API 密钥。对于 Ollama 等本地服务，可以使用占位符如 `ollama` |
| **Base URL** | API 基础 URL（如 `https://api.openai.com/v1`、Ollama 使用 `http://localhost:11434/v1`） |

![添加 Provider 对话框](/getting-started/provider-model-03-provider-dialog.png)

**示例 — OpenAI：**
- Name：`openai`
- API Key：`sk-...`
- Base URL：`https://api.openai.com/v1`

**示例 — Ollama（本地）：**
- Name：`ollama`
- API Key：`ollama`
- Base URL：`http://localhost:11434/v1`

点击 **Add** 保存。新 Provider 将出现在左侧边栏。

## 第 3 步：添加模型

从左侧面板选择一个 Provider。右侧面板将显示 Provider 表单和模型列表。

![Provider 已选择 - 模型列表](/getting-started/provider-model-04-provider-selected.png)

点击 **Add Model** 打开模型创建对话框。

填写：

| 字段 | 说明 |
|------|------|
| **Client Type** | API 协议：`openai-responses`、`openai-completions`、`anthropic-messages` 或 `google-generative-ai` |
| **Type** | `chat` 或 `embedding` |
| **Model** | 模型 ID（如 `gpt-4`、`llama3.2`、`text-embedding-3-small`） |
| **Display Name** | 可选的显示名称 |
| **Dimensions** | 嵌入模型必填（如 OpenAI 嵌入模型使用 `1536`） |
| **Multimodal** | 仅限聊天模型——模型支持图片时启用 |

**你至少需要：**
- 一个 **chat** 模型（用于对话）
- 一个 **embedding** 模型（用于记忆）

可以将它们添加在同一个或不同的 Provider 下。例如：
- Chat：`gpt-4`，客户端类型 `openai-responses`（OpenAI）或 `llama3.2`，客户端类型 `openai-completions`（Ollama）
- Embedding：`text-embedding-3-small`，客户端类型 `openai-completions`（OpenAI）或 `nomic-embed-text`，客户端类型 `openai-completions`（Ollama）

## 第 4 步：编辑或删除

- **Provider**：选择 Provider 后，可以在右侧面板编辑 Name、API Key 和 Base URL，或删除 Provider。
- **模型**：使用列表中每个模型卡片上的编辑或删除操作。

## 下一步

配置好至少一个聊天模型和一个嵌入模型后，你可以创建 Bot（通过侧边栏的 **Bots**），并在 Bot 设置中分配这些模型。
