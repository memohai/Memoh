# Bot

**Bot** 是 Memoh 中的主要运行时实体。

每个 Bot 拥有独立的：

- 配置
- 容器生命周期
- 记忆作用域
- 接入平台绑定
- 模型分配

## 关键设置

- **max-load-time**（`max_context_load_time`）：加载最近多少分钟的对话上下文到提示词中
- **language**：交互的首选语言（默认为 `auto`）
- **chat model / memory model / embedding model**：此 Bot 使用的模型 ID

## 为什么重要

Bot 抽象使得 Memoh 能够按 Agent 隔离行为和资源，同时在一个 Web UI 中集中管理。

## Web UI 路径

- `Bots > 选择一个 Bot > Settings`
