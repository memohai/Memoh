# Memoh Skill System v2 - DeerFlow 对齐设计

## 目标

将 Memoh 的 skill 系统与 DeerFlow 对齐，提供更丰富的元数据、权限控制、状态管理和分发机制。

## 当前状态 vs 目标状态

| 特性 | 当前 (v1) | 目标 (v2/DeerFlow) |
|------|-----------|-------------------|
| 元数据字段 | name, description, metadata | + version, author, license, allowed-tools, compatibility |
| 权限控制 | 无 | allowed-tools 白名单 |
| 状态管理 | 无 | extensions_config.json |
| 分类存储 | 单一目录 | public/custom 分类 |
| 打包格式 | 原始 Markdown | .skill ZIP 包 |
| 安装机制 | 手动上传 | API 安装 + 分发 |

## 数据模型

### Skill 元数据 (SKILL.md frontmatter)

```yaml
---
name: deep-research                    # 必填: hyphen-case 格式
description: 深度研究技能描述          # 必填: 1024字符以内
version: 1.0.0                         # 可选: 语义化版本
author: memoh-team                     # 可选: 作者/组织
license: MIT                           # 可选: 开源协议
allowed-tools:                         # 可选: 工具白名单
  - web_search
  - web_fetch
  - read_file
compatibility: memoh>=2.0              # 可选: 兼容性声明
category: research                     # 可选: 分类标签
---
```

### SkillItem (内部结构)

```go
type SkillItem struct {
    // 核心字段
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Content     string         `json:"content"`      // Markdown 内容(不含 frontmatter)
    Raw         string         `json:"raw"`          // 完整原始内容

    // 扩展元数据
    Version      string        `json:"version,omitempty"`
    Author       string        `json:"author,omitempty"`
    License      string        `json:"license,omitempty"`
    AllowedTools []string      `json:"allowed_tools,omitempty"`
    Compatibility string       `json:"compatibility,omitempty"`
    Category     string        `json:"category,omitempty"`

    // 运行时状态
    Enabled      bool          `json:"enabled"`
    InstalledAt  time.Time     `json:"installed_at"`
    UpdatedAt    time.Time     `json:"updated_at"`

    // 文件位置
    SkillDir     string        `json:"-"`            // 宿主路径
    ContainerPath string       `json:"container_path"` // 容器内路径
}
```

### ExtensionsConfig (状态管理)

```go
type ExtensionsConfig struct {
    Version    string                 `json:"version"`     // 配置版本
    UpdatedAt  time.Time              `json:"updated_at"`

    Skills     map[string]SkillState  `json:"skills"`      // skill 状态
    MCPServers map[string]MCPConfig   `json:"mcp_servers"` // MCP 服务器配置
}

type SkillState struct {
    Enabled     bool      `json:"enabled"`
    AutoLoad    bool      `json:"auto_load"`    // 是否自动加载到 prompt
    InstalledAt time.Time `json:"installed_at"`
    Source      string    `json:"source"`       // 安装来源
}
```

## 存储结构

### 宿主目录

```
/data/memoh/
├── .skills/                    # Skill 存储根目录
│   ├── public/                 # 内置/官方 skills
│   │   ├── deep-research/
│   │   │   └── SKILL.md
│   │   ├── code-review/
│   │   │   └── SKILL.md
│   │   └── templates/          # 可选: 模板资源
│   │       └── task_plan.md
│   ├── custom/                 # 用户自定义 skills
│   │   ├── my-company-style/
│   │   │   ├── SKILL.md
│   │   │   └── examples/
│   │   └── private-tool/
│   │       └── SKILL.md
│   └── extensions_config.json  # 状态管理文件
```

### 容器内路径

```
/mnt/memoh/
├── .skills/                    # 挂载自宿主 /data/memoh/.skills
│   ├── public/
│   ├── custom/
│   └── extensions_config.json
```

## API 设计

### Skill 管理 API

```go
// 列表 Skills (支持过滤)
GET /api/v1/bots/{bot_id}/skills
Query: ?enabled_only=true&category=research

// 获取单个 Skill
GET /api/v1/bots/{bot_id}/skills/{skill_name}

// 创建/更新 Skill
PUT /api/v1/bots/{bot_id}/skills/{skill_name}
Body: {
    "content": "---\nname: xxx\n...\n---\n# ...",
    "enabled": true
}

// 删除 Skill
DELETE /api/v1/bots/{bot_id}/skills/{skill_name}

// 更新 Skill 状态
PATCH /api/v1/bots/{bot_id}/skills/{skill_name}/state
Body: {
    "enabled": true,
    "auto_load": false
}

// 安装 Skill (从 .skill 包)
POST /api/v1/bots/{bot_id}/skills/install
Content-Type: multipart/form-data
Body: file=@my-skill.skill

// 导出 Skill 为 .skill 包
GET /api/v1/bots/{bot_id}/skills/{skill_name}/export
Response: application/zip (.skill 文件)

// 验证 Skill 格式
POST /api/v1/bots/{bot_id}/skills/validate
Body: {
    "content": "---\nname: xxx\n..."
}
```

## CLI 命令

Memoh 提供 `memoh skill` CLI 命令来管理技能，与 DeerFlow CLI 对齐。

### 环境变量

```bash
export MEMOH_SERVER=http://localhost:8080    # 服务器地址
export MEMOH_BOT_ID=<bot-id>                  # Bot ID
export MEMOH_TOKEN=<jwt-token>                # JWT Token (或用户名/密码)
export MEMOH_USERNAME=admin                   # 用户名
export MEMOH_PASSWORD=<password>              # 密码
```

### 命令列表

```bash
# 列出所有技能
memoh skill list
memoh skill list --category public

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

### 命令行参数

| 参数 | 简写 | 描述 | 环境变量 |
|------|------|------|----------|
| --server | -s | 服务器 URL | MEMOH_SERVER |
| --bot | -b | Bot ID | MEMOH_BOT_ID |
| --token | -t | JWT Token | MEMOH_TOKEN |
| --username | -u | 用户名 | MEMOH_USERNAME |
| --password | -p | 密码 | MEMOH_PASSWORD |

### 使用示例

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

### Extensions Config API

```go
// 获取配置
GET /api/v1/bots/{bot_id}/extensions/config

// 更新配置
PUT /api/v1/bots/{bot_id}/extensions/config
Body: ExtensionsConfig

// 批量更新 Skill 状态
PUT /api/v1/bots/{bot_id}/extensions/config/skills
Body: {
    "skills": {
        "deep-research": { "enabled": true, "auto_load": true },
        "code-review": { "enabled": false }
    }
}
```

## 权限与安全

### 工具白名单机制

1. 当 Skill 被加载时，系统检查 `allowed-tools`
2. 如果存在白名单，只有列出的工具可被该 Skill 使用
3. 在 Agent 执行时，工具调用前进行权限检查

### Skill 验证规则

```go
// 名称验证: hyphen-case
validNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// 禁止以连字符开头/结尾，禁止连续连字符
// 最大长度 64 字符

// 描述验证
// - 不能包含 < 或 >
// - 最大长度 1024 字符

// 版本验证 (语义化版本)
validVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
```

## .skill 打包格式

### 文件结构 (ZIP)

```
my-skill.skill
├── SKILL.md              # 主文件 (必须)
├── metadata.json         # 扩展元数据 (可选)
├── templates/            # 模板文件 (可选)
│   └── template.md
├── scripts/              # 辅助脚本 (可选)
│   └── init.sh
└── resources/            # 资源文件 (可选)
    └── image.png
```

### 安全提取规则

1. 拒绝绝对路径
2. 拒绝包含 `..` 的路径
3. 跳过符号链接
4. 限制总解压大小 (默认 512MB)
5. 验证文件类型

## 实现步骤

### Phase 1: 核心模型扩展
- [ ] 扩展 SkillItem 结构体
- [ ] 更新 parseSkillFile 函数
- [ ] 添加验证函数
- [ ] 更新 API handler

### Phase 2: 状态管理
- [ ] 实现 ExtensionsConfig 结构
- [ ] 添加配置持久化
- [ ] 实现配置 API
- [ ] 迁移现有 skills 到 config

### Phase 3: 打包安装
- [ ] 实现 .skill 打包功能
- [ ] 实现安全解压
- [ ] 添加安装 API
- [ ] 添加导出 API

### Phase 4: 工具权限
- [ ] 在 Agent 中添加工具权限检查
- [ ] 根据 allowed-tools 过滤可用工具
- [ ] 错误提示优化

## 与 DeerFlow 的差异点

| 方面 | DeerFlow | Memoh |
|------|----------|-------|
| 运行环境 | LangGraph 编排 | Twilight AI SDK |
| Skill 使用 | use_skill 工具激活 | use_skill 工具激活 |
| Subagent | 原生支持 | 通过 SDK 实现 |
| 容器 | Docker Sandbox | bridge+gVisor |

## 迁移路径

1. 保持现有 API 兼容
2. 新字段为可选，旧 skills 仍可工作
3. 逐步引导用户补充元数据
4. 提供迁移工具脚本
