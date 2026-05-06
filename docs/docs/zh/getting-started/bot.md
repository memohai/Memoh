# 机器人

机器人是 Memoh 里**独立**的智能体：自带容器、长期记忆、可配性格，并能通过各 **渠道** 对话、用工具做事。

## 创建

1. 侧栏进入 **Bots**。
2. 点 **Create Bot**。
3. 基本信息：
   - **Display Name**：对外的名字
   - **Avatar**：头像 URL
   - **Timezone**：可空；不填则继承用户或系统时区
   - **ACL Preset**：如 `allow_all` 只给自己用、`private_only` 等快捷策略
4. 创建。

---

## 详情页

点卡片进 **详情**，各 tab 管不同事：

| Tab | 内容 |
|-----|------|
| **Overview** | 容器、库、渠道、记忆等健康检查 |
| **General** | 主模型/标题/生图、记忆/搜索/浏览器/TTS、时区、语言、推理、危险区 |
| **Container** | 起停、快照、导入导出 |
| **Memory** | 浏览、搜、建、改、压记忆 |
| **Platforms** | 各消息渠道（Telegram、Discord、飞书等） |
| **Access** | ACL 与默认通过/拒绝 |
| **Email** | 邮服绑定、发件箱 |
| **Terminal** | 进容器 shell |
| **Files** | 容器内文件管理 |
| **MCP** | 连接（Stdio/Remote/OAuth） |
| **Heartbeat** | 心跳间隔、模型、执行日志 |
| **Compaction** | 会话压缩设置与记录 |
| **Schedule** | cron 与日志 |
| **Skills** | 技能 Markdown |

---

## 核心先配什么

1. 打开机器人 **General**，先管模型与各类绑定。
2. **Heartbeat** 管周期自主跑。
3. **Compaction** 管会话写不长时的压缩。
4. **Access** 在 ACL 预设之后细调。

若这些资源还没有，先建好：

- [供应商与模型](/zh/getting-started/provider-and-model.md)
- [内置记忆提供方](/zh/memory-providers/builtin.md)（如用）
- [搜索提供方](/zh/getting-started/search-provider.md)
- [TTS 提供方](/zh/tts-providers/index)

---

## General 字段

| 字段 | 说明 |
|------|------|
| **Chat Model** | 主对话模型 |
| **Title Model** | 可选，生成会话标题 |
| **Image Generation Model** | 可选，需带 `image-output` 的聊天模型 |
| **Memory Provider** | 长期记忆后端；内置类型还可自带记忆/向量模型 |
| **Search Provider** | 联网搜索用哪家 |
| **TTS Model** | 来自 TTS 流，不是普通 chat 供应商里选 |
| **Timezone** | 不填则用户时区再落到系统 |
| **Language** | 机器人主用语 |
| **Reasoning Enabled** | 当前 chat 模型有 `reasoning` 时可用 |
| **Reasoning Effort** | `low` / `medium` / `high` |

注意：

- **生图模型** 故意与主聊天模型分开，好单独换「更擅长出图」的。
- **TTS** 在 [TTS 提供方](/zh/tts-providers/index.md) 里用 `speech` 模型，例如 Edge。
- `context_window` 会影响状态栏展示和 [会话压缩](/zh/getting-started/compaction.md) 的体感。

---

## Heartbeat 字段

| 字段 | 说明 |
|------|------|
| **Heartbeat Enabled** | 开不开周期自主 |
| **Interval** | 多少分钟一次 |
| **Heartbeat Model** | 可与主 chat 不同 |

同 tab 可看各次执行日志。

---

## Compaction 相关（此处指「会话」）

这里说的是 **当前会话** 的上下文压短，不是改记忆条目的那种。

| 字段 | 说明 |
|------|------|
| **Compaction Enabled** | 是否自动在会话里压摘要 |
| **Compaction Threshold** | 触发的估算 token 阈值 |
| **Compaction Ratio** | 压多狠 |
| **Compaction Model** | 可选，专门做摘要的模型 |

细节见 [会话上下文压缩](/zh/getting-started/compaction.md)。

---

## 访问与 ACL

创建时先给一个 **ACL 预设**，之后在 **Access** 里微调。**预设** 给一版默认策略，**Default Effect** 管「没命中规则时」放行还是挡。

[会话](/zh/getting-started/sessions.md) 与 Discuss 的默认行为在那一页。若你用 API/自动化，配置里还可能有 `discuss_probe_model_id` 等进阶项，日常创建不必先动。

---

## 终端

**Terminal** tab 开交互 shell，可多 tab；**容器在跑**时才能用。

---

## 删除

**General** 最下 **Danger Zone** → **Delete Bot**，会删掉该机器人相关数据（含容器文件与记忆等），**不可恢复**。
