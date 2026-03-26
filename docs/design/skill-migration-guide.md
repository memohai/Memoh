# Memoh Skill System V2 迁移指南

## 概述

Memoh Skill System 已升级至 V2，与 DeerFlow 的 skill 机制对齐。新版本提供了更丰富的元数据、状态管理、权限控制和打包分发能力。

**实现状态**: ✅ 核心功能已完成（2026-03-26）

## 新特性

### 1. 扩展的元数据字段 ✅ 已实现

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| name | string | 是 | Skill 标识符 (hyphen-case 格式) |
| description | string | 是 | 简短描述 (最大 1024 字符) |
| version | string | 否 | 语义化版本 (如 1.0.0) |
| author | string | 否 | 作者或组织 |
| license | string | 否 | 许可证标识 (如 MIT) |
| allowed-tools | []string | 否 | 允许使用的工具白名单 |
| compatibility | string | 否 | 版本兼容性声明 |
| category | string | 否 | 分类标签 |

### 2. 工具权限控制 ✅ 已实现（API 层）

通过 `allowed-tools` 字段限制 skill 可用的工具：

```yaml
---
name: safe-search
description: A skill that can only search the web
allowed-tools:
  - web_search
  - web_fetch
---
```

**注意**: 当前在 API 层验证，Agent 执行层的工具权限检查待实现。

### 3. 状态管理 ✅ 已实现

技能状态通过 `extensions_config.json` 管理：

```json
{
  "version": "1.0.0",
  "skills": {
    "deep-research": {
      "enabled": true,
      "auto_load": false,
      "category": "public"
    }
  }
}
```

### 4. 分类存储

技能分为两类存储：
- `public/` - 内置/官方技能
- `custom/` - 用户自定义技能

### 5. 打包与分发

支持 `.skill` 格式的打包和安装：

```bash
# 导出技能
GET /api/v1/bots/{bot_id}/skills/v2/{skill_name}/export

# 安装技能
POST /api/v1/bots/{bot_id}/skills/v2/install
Content-Type: multipart/form-data
file=@my-skill.skill
```

## API 变更

### 新端点

| 方法 | 端点 | 描述 |
|------|------|------|
| GET | `/api/v1/bots/{bot_id}/skills/v2` | 列出所有技能（带扩展元数据） |
| GET | `/api/v1/bots/{bot_id}/skills/v2/{skill_name}` | 获取单个技能 |
| PUT | `/api/v1/bots/{bot_id}/skills/v2/{skill_name}` | 创建/更新技能 |
| PATCH | `/api/v1/bots/{bot_id}/skills/v2/{skill_name}/state` | 更新技能状态 |
| DELETE | `/api/v1/bots/{bot_id}/skills/v2/{skill_name}` | 删除技能 |
| POST | `/api/v1/bots/{bot_id}/skills/v2/validate` | 验证技能格式 |
| POST | `/api/v1/bots/{bot_id}/skills/v2/install` | 安装 .skill 包 |
| GET | `/api/v1/bots/{bot_id}/skills/v2/{skill_name}/export` | 导出技能为 .skill 包 |

### CLI 命令

Memoh 提供 `memoh skill` CLI 命令来管理技能，与 DeerFlow CLI 对齐。

#### 环境变量

```bash
export MEMOH_SERVER=http://localhost:8080    # 服务器地址
export MEMOH_BOT_ID=<bot-id>                  # Bot ID
export MEMOH_TOKEN=<jwt-token>                # JWT Token
export MEMOH_USERNAME=admin                   # 用户名
export MEMOH_PASSWORD=<password>              # 密码
```

#### 命令列表

```bash
# 列出所有技能
memoh skill list

# 获取技能详情
memoh skill get <skill-name>

# 安装技能
memoh skill install <path-to-skill.skill>

# 卸载技能
memoh skill uninstall <skill-name>
# 或
memoh skill rm <skill-name>

# 启用/禁用技能
memoh skill enable <skill-name>
memoh skill disable <skill-name>

# 导出技能
memoh skill export <skill-name> [output-path]
```

#### 命令行参数

| 参数 | 简写 | 描述 | 环境变量 |
|------|------|------|----------|
| --server | -s | 服务器 URL | MEMOH_SERVER |
| --bot | -b | Bot ID | MEMOH_BOT_ID |
| --token | -t | JWT Token | MEMOH_TOKEN |
| --username | -u | 用户名 | MEMOH_USERNAME |
| --password | -p | 密码 | MEMOH_PASSWORD |

#### 使用示例

```bash
# 使用环境变量
export MEMOH_SERVER=http://localhost:8080
export MEMOH_BOT_ID=my-bot
export MEMOH_TOKEN=eyJhbGciOiJIUzI1NiIs...

memoh skill list
memoh skill install ./my-skill.skill
memoh skill enable my-skill

# 使用命令行参数
memoh skill list -s http://localhost:8080 -b my-bot -t <token>
memoh skill install -b my-bot ./my-skill.skill
```

### 向后兼容

原有的 V1 API 仍然可用：
- `GET /api/v1/bots/{bot_id}/container/skills`
- `POST /api/v1/bots/{bot_id}/container/skills`
- `DELETE /api/v1/bots/{bot_id}/container/skills`

## 迁移步骤

### 1. 更新现有技能文件

将现有技能的 `SKILL.deer-flow.md` 更新为新的格式：

**旧格式：**
```yaml
---
name: my-skill
description: My skill description
---
```

**新格式：**
```yaml
---
name: my-skill
description: My skill description
version: 1.0.0
author: my-team
license: MIT
allowed-tools:
  - web_search
  - read_file
---
```

### 2. 迁移存储结构

将现有技能从单一目录迁移到分类目录：

```bash
# 旧结构
/data/.skills/my-skill/SKILL.deer-flow.md

# 新结构
/data/.skills/custom/my-skill/SKILL.md
```

### 3. 初始化 extensions_config.json

系统会自动创建 `extensions_config.json` 并同步现有技能状态。

## 实现文件

新功能分布在以下文件中：

| 文件 | 描述 |
|------|------|
| `internal/agent/tools/skillv2.go` | 扩展的 Skill 类型和解析器 |
| `internal/agent/tools/extensions_config.go` | 状态管理配置 |
| `internal/agent/tools/skill_installer.go` | .skill 包安装器 |
| `internal/handlers/skills_v2.go` | V2 API Handler |
| `cmd/memoh/skill.go` | CLI skill 命令实现 |

## 与 DeerFlow 的对比

| 特性 | DeerFlow | Memoh V2 |
|------|----------|----------|
| 元数据 | 完整 | 完整 |
| 工具白名单 | 支持 | 支持 |
| 状态管理 | extensions_config.json | extensions_config.json |
| 分类存储 | public/custom | public/custom |
| 打包格式 | .skill ZIP | .skill ZIP |
| 运行时 | LangGraph | Twilight AI |
| 容器 | Docker | bridge+gVisor |

## 后续计划

### 已完成 ✅
- [x] 扩展元数据字段
- [x] 工具白名单（API 层）
- [x] 状态管理
- [x] 分类存储
- [x] .skill 打包格式
- [x] V2 API Handler

### 待实现 📋
1. **Phase 2**: 在 Agent 层实现工具权限检查（执行时验证）
2. **Phase 3**: 添加路由注册到 Echo 服务器
3. **Phase 4**: 编写单元测试
4. **Phase 5**: 迁移现有技能到 V2 格式
5. **Phase 6**: skill 市场/仓库集成
6. **Phase 7**: skill 依赖管理

## 参考

- DeerFlow: https://github.com/bytedance/deer-flow
- 设计文档: `./skill-system-v2.md`
- 示例技能: `./example-skill.md`
