# Provider 和模型

在 Memoh 中，**Provider** 和**模型**是独立但相关联的概念：

- **Provider** 是 LLM 服务配置（API 端点和密钥）
- **模型**是该 Provider 下的具体聊天或嵌入模型，包括决定使用哪种 API 协议的**客户端类型**

## 客户端类型

每个模型有一个 `client_type`，决定 Memoh 如何与 LLM 服务通信：

| 客户端类型 | 说明 |
|-----------|------|
| `openai-responses` | OpenAI Responses API |
| `openai-completions` | OpenAI Chat Completions API（也适用于 Ollama、Mistral 等兼容服务） |
| `anthropic-messages` | Anthropic Messages API |
| `google-generative-ai` | Google Generative AI API |

## 典型配置

至少需要为生产就绪的 Bot 配置：

- 一个 **chat** 模型，用于对话生成
- 一个 **embedding** 模型，用于记忆索引和检索

## 模型分配到 Bot

Bot 在设置中引用模型 ID：

- `chat_model_id`
- `memory_model_id`
- `embedding_model_id`

这使得每个 Bot 可以按质量、延迟或成本进行个性化定制。

## Web UI 路径

- `Models > Add Provider > 选择 Provider > Add Model`
- `Bots > 选择一个 Bot > Settings > 选择 chat/memory/embedding 模型`
