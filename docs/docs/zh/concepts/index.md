# 核心概念总览

本章节阐述 Memoh 背后的核心设计概念。

当你想要了解 Memoh 的设计理念、功能存在的原因以及各部分如何协同工作时，请阅读这些页面。

## 概念图

- **Bot**：核心运行时单元
- **Provider 和模型**：LLM 能力的接入方式
- **记忆**：长期知识的存储与检索
- **接入平台（Channel）**：外部平台如何连接 Bot
- **定时任务（Schedule）**：任务的自动触发机制
- **容器（Container）**：每个 Bot 的隔离执行环境
- **MCP**：外部工具和服务的集成协议
- **子代理（Subagents）**：专门的委派代理
- **技能（Skills）**：可复用的能力指令
- **会话与历史**：聊天上下文与可追溯性

## 推荐阅读顺序

1. [Bot](/zh/concepts/bot.md)
2. [Provider 和模型](/zh/concepts/provider-and-model.md)
3. [记忆](/zh/concepts/memory.md)
4. [接入平台](/zh/concepts/channel.md)
5. [容器](/zh/concepts/container.md)
6. [定时任务](/zh/concepts/schedule.md)
7. [MCP](/zh/concepts/mcp.md)
8. [子代理](/zh/concepts/subagents.md)
9. [技能](/zh/concepts/skills.md)
10. [会话与历史](/zh/concepts/conversation-and-history.md)
