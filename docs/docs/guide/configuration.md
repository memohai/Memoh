# 配置指南

## 环境变量

### 必需配置

- `DATABASE_URL` - PostgreSQL 连接字符串
- `ROOT_USER` - 超级管理员用户名
- `ROOT_USER_PASSWORD` - 超级管理员密码
- `JWT_SECRET` - JWT 签名密钥

### 可选配置

- `QDRANT_URL` - Qdrant 连接字符串（默认：http://localhost:6333）
- `REDIS_URL` - Redis 连接字符串（默认：redis://localhost:6379）
- `API_PORT` - API 服务端口（默认：8080）

## 模型配置

### 创建模型

```bash
pnpm cli model create \
  --name "GPT-4" \
  --model-id "gpt-4" \
  --base-url "https://api.openai.com/v1" \
  --api-key "your-api-key" \
  --client-type "openai" \
  --type "chat"
```

### 设置默认模型

```bash
pnpm cli config set \
  --chat-model <uuid> \
  --summary-model <uuid> \
  --embedding-model <uuid>
```

## 用户配置

### 设置最大上下文时间

```bash
pnpm cli config set --max-context-time <minutes>
```

默认值为 900 分钟（15 小时）。

