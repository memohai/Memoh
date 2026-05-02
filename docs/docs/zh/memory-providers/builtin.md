# 内置记忆

自带默认记忆后端，接 Memoh 的抽取/检索流程，支持：

- 从对话里抽记忆
- 聊天时语义检索
- 手建、手改
- 记忆压缩、整库重建

三种 **memory mode**，对基础设施和效果要求不同。

---

## 模式

| 模式 | 索引 | 要啥 | 适合 |
|------|------|------|------|
| **Off** | 仅文件 | 无额外服务 | 最轻，不做向量 |
| **Sparse** | 神经稀疏向量 | sparse 服务 + 数据库稀疏索引（`--profile sparse`） | 不想交 embedding API 钱、又要比纯词匹配强 |
| **Dense** | 稠密向量 | embedding 模型 + 数据库向量索引 | 要稠密语义检索时 |

### Sparse 在干什么

用 OpenSearch 项目放出来的 [`opensearch-neural-sparse-encoding-multilingual-v1`](https://huggingface.co/opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1) 把文字变成**稀疏向量**（一批 token 下标 + 权重）。不另买 embedding API，在 `sparse` 容器里本地跑。多语言，一般比只关键词强不少。

---

## 建一个

1. **Memory Providers**。
2. **Add Memory Provider**。
3. **Name**、**Provider Type** 选 `builtin`。
4. **Create**。

---

## 配置

| 字段 | 说明 |
|------|------|
| **Memory Mode** | `off`（默认）/ `sparse` / `dense` |
| **Embedding Model** | 仅 `dense` 要，指向你的 embedding 模型 |
| **Index** | 当前模式使用的数据库索引 |

**Edit**、**Delete** 如常。

---

## 依赖

### Off

只要文件侧索引，无向量服务。

### Sparse

要 **sparse 服务**：

```bash
docker compose --profile sparse up -d
```

`config.toml` 里至少要有类似：

```toml
[sparse]
base_url = "http://sparse:8085"
```

### Dense

要 **embedding 模型**（在提供方里配）。向量会存到当前数据库后端：PostgreSQL 使用 pgvector，SQLite 使用 sqlite-vec。

（稠密模式细节、embedding 在 UI 里选哪条，以你当前版本为准。）

---

## 绑到机器人

1. **Bots** → 机器人
2. **General** → **Memory Provider**
3. 保存

若未选，运行层面不会用这条提供方。

---

## 配好之后

在 **Memory** tab 可手建、从对话抽、搜、改、压、重建等。日常操作见 [长期记忆](/zh/getting-started/memory.md)。
