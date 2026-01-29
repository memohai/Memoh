# 快速开始

欢迎使用 Memoh！本指南将帮助你快速上手。

## 什么是 Memoh？

Memoh 是一个专属于你的 AI 私人管家，你可以把它跑在你的 NAS，路由器等个人设备上，24 小时的为你提供服务。

## 环境要求

在开始之前，请确保你的系统满足以下要求：

- **PostgreSQL 16+** - 数据库
- **Bun 1.2+** - JavaScript 运行时
- **PNPM** - 包管理器
- **Qdrant** - 向量数据库
- **Redis** - 缓存和会话存储

## 安装步骤

### 1. 克隆项目

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
```

### 2. 安装依赖

```bash
pnpm install
```

### 3. 配置环境变量

复制环境变量示例文件：

```bash
cp .env.example .env
```

编辑 `.env` 文件，配置以下变量：

```env
# 数据库配置
DATABASE_URL=postgresql://user:password@localhost:5432/memoh

# 管理员账户
ROOT_USER=admin
ROOT_USER_PASSWORD=your_password

# JWT 密钥
JWT_SECRET=your_jwt_secret_key

# Qdrant 向量数据库
QDRANT_URL=http://localhost:6333

# Redis 缓存
REDIS_URL=redis://localhost:6379
```

### 4. 初始化数据库

```bash
pnpm run db:push
```

### 5. 启动 API 服务

```bash
pnpm run api:dev
```

API 服务将在 `http://localhost:8080` 启动。

## 下一步

- [使用 CLI 工具](/cli/) - 学习如何使用命令行工具
- [配置 Telegram Bot](/platforms/telegram) - 集成 Telegram 平台
- [配置指南](/guide/configuration) - 了解如何配置 Memoh

