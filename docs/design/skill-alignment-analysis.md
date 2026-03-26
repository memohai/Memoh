# Memoh Skill System 与 DeerFlow 对齐分析报告

## 执行摘要

当前 Memoh Skill System V2 实现了**基础架构层**的对齐，但在**技能生态层**和**评估框架层**还有显著差距。

| 层级 | 对齐度 | 说明 |
|------|--------|------|
| 基础架构层 | 85% | 元数据、打包、状态管理已对齐 |
| 技能生态层 | 10% | 缺少预置技能库 |
| 评估框架层 | 5% | 缺少完整的评估和优化框架 |

---

## 1. 已实现对齐 ✅

### 1.1 Skill 元数据格式

**DeerFlow 格式：**
```yaml
---
name: skill-name
description: When to use this skill...
---
```

**Memoh V2 格式（已对齐）：**
```yaml
---
name: skill-name
description: When to use this skill...
version: 1.0.0
author: author-name
license: MIT
allowed-tools: [tool1, tool2]
compatibility: memoh>=2.0
category: productivity
---
```

✅ **优势**：Memoh 实际上比 DeerFlow 更完整，支持更多元数据字段

### 1.2 打包格式 (.skill)

| 特性 | DeerFlow | Memoh V2 | 状态 |
|------|----------|----------|------|
| ZIP 压缩 | ✅ | ✅ | ✅ 对齐 |
| 安全解压 | ✅ (防遍历/炸弹) | ✅ (512MB限制) | ✅ 对齐 |
| 单根目录检测 | ✅ | ✅ | ✅ 对齐 |
| Frontmatter 验证 | ✅ | ✅ | ✅ 对齐 |

### 1.3 存储结构

**DeerFlow：**
```
.skills/
├── public/          # 内置技能
├── custom/          # 用户安装
└── extensions_config.json
```

**Memoh V2（已对齐）：**
```
/data/.skills/
├── public/          # 内置技能
├── custom/          # 用户安装
└── extensions_config.json
```

### 1.4 API 端点

| 端点 | DeerFlow | Memoh V2 | 状态 |
|------|----------|----------|------|
| List skills | ✅ | ✅ | ✅ 对齐 |
| Get skill | ✅ | ✅ | ✅ 对齐 |
| Update skill | ✅ | ✅ | ✅ 对齐 |
| Delete skill | ✅ | ✅ | ✅ 对齐 |
| Install .skill | ✅ | ✅ | ✅ 对齐 |
| Export skill | ✅ | ✅ | ✅ 对齐 |
| Validate skill | ✅ | ✅ | ✅ 对齐 |
| Update state | ✅ | ✅ | ✅ 对齐 |

---

## 2. 尚未对齐 ❌

### 2.1 Bundled Resources（缺失）

**DeerFlow 支持：**
```
skill-name/
├── SKILL.md              # 主文件 (必需)
├── scripts/              # 可执行脚本
│   └── generate.py
├── references/           # 参考文档
│   └── chart-types.md
├── templates/            # 模板文件
│   └── template.md
├── assets/               # 静态资源
│   └── icon.png
└── agents/               # 子代理定义
    └── grader.md
```

**Memoh 现状：**
- ❌ 只支持单文件 SKILL.md
- ❌ 不支持 bundled resources
- ❌ 技能无法包含脚本、模板、参考文档

**影响：**
- `chart-visualization` 技能需要 26 个参考文档描述图表类型
- `ppt-generation` 需要脚本 `generate.py` 来合成 PPTX
- `skill-creator` 需要多个子代理定义 (`agents/`)

### 2.2 预置技能库（严重缺失）

**DeerFlow 20+ Public Skills：**

| 技能名称 | 用途 | Memoh 状态 |
|----------|------|-----------|
| chart-visualization | 26+ 种图表生成 | ❌ 缺失 |
| ppt-generation | PPT 生成 | ❌ 缺失 |
| deep-research | 深度研究方法论 | ❌ 缺失 |
| image-generation | 图片生成 | ❌ 缺失 |
| video-generation | 视频生成 | ❌ 缺失 |
| podcast-generation | 播客生成 | ❌ 缺失 |
| frontend-design | 前端设计 | ❌ 缺失 |
| data-analysis | 数据分析 | ❌ 缺失 |
| github-deep-research | GitHub 研究 | ❌ 缺失 |
| skill-creator | 技能创建器 | ❌ 缺失 |
| consulting-analysis | 咨询分析 | ❌ 缺失 |
| surprise-me | 随机创意 | ❌ 缺失 |
| claude-to-deerflow | Claude Code 集成 | ❌ 缺失 |

**影响：**
- 新用户没有开箱即用的功能
- 需要手动创建所有技能
- 无法快速体验系统能力

### 2.3 Skill Creator 评估框架（严重缺失）

**DeerFlow 完整框架：**

```
skill-creator/
├── SKILL.md                    # 主技能文件
├── agents/                     # 子代理
│   ├── analyzer.md            # 结果分析器
│   ├── comparator.md          # 盲比较器
│   └── grader.md              # 评分器
├── references/                 # 参考文档
│   ├── schemas.md             # JSON Schema
│   ├── workflows.md           # 工作流文档
│   └── output-patterns.md     # 输出模式
├── eval-viewer/               # 评估查看器
│   ├── generate_review.py     # HTML 生成器
│   └── templates/             # 模板
└── scripts/                   # 工具脚本
    ├── aggregate_benchmark.py # 聚合基准
    ├── run_loop.py            # 优化循环
    └── package_skill.py       # 打包工具
```

**核心功能：**

| 功能 | DeerFlow | Memoh | 优先级 |
|------|----------|-------|--------|
| 测试用例管理 | ✅ evals.json | ❌ | P0 |
| 定量断言 | ✅ assertions | ❌ | P0 |
| with-skill vs without-skill | ✅ 并行运行 | ❌ | P0 |
| HTML 评估查看器 | ✅ | ❌ | P1 |
| 基准测试报告 | ✅ benchmark.json | ❌ | P1 |
| 描述优化 | ✅ | ❌ | P1 |
| 盲比较 | ✅ | ❌ | P2 |

**影响：**
- 无法验证技能质量
- 无法迭代优化技能
- 无法检测技能退化

### 2.4 CLI 安装机制（部分缺失）

**DeerFlow 支持：**
```bash
# Python Client API
from deerflow.client import DeerFlowClient
client = DeerFlowClient()
client.install_skill("path/to/skill.skill")

# Direct function
from deerflow.skills.installer import install_skill_from_archive
install_skill_from_archive("path/to/skill.skill")
```

**Memoh 现状：**
```bash
# 仅支持 HTTP API
POST /api/v1/bots/{bot_id}/skills/v2/install
```

**缺失：**
- ❌ CLI 命令安装
- ❌ Python/Go SDK 安装
- ❌ 技能市场浏览

### 2.5 渐进式加载（部分缺失）

**DeerFlow 三级加载：**
1. **Level 1**: Metadata (name + description) - 始终在上下文 (~100 words)
2. **Level 2**: SKILL.md body - 触发时加载 (<500 lines)
3. **Level 3**: Bundled resources - 按需加载 (无限制)

**Memoh 现状：**
- ✅ Level 1: 支持（通过 API 获取元数据）
- ✅ Level 2: 支持（SKILL.md 内容）
- ❌ Level 3: 不支持（无法加载 bundled resources）

---

## 3. 用户需求的实现分析

### 3.1 需求1：复制 DeerFlow public skills 作为内置模板

**可行度：** ⭐⭐⭐⭐⭐ 高

**实现路径：**
1. 将 DeerFlow `skills/public/` 下的技能复制到 Memoh
2. 适配路径引用（如 `/mnt/skills/public/` → `/data/.skills/public/`）
3. 实现 bundled resources 支持

**工作量估算：**
- 简单技能（纯 SKILL.md）：1-2 小时/个
- 复杂技能（含 resources）：4-8 小时/个
- 全部 20+ 技能：约 2-4 周

**推荐优先级：**
1. P0: deep-research, chart-visualization, skill-creator
2. P1: ppt-generation, image-generation, frontend-design
3. P2: video-generation, podcast-generation

### 3.2 需求2：命令行安装机制

**可行度：** ⭐⭐⭐⭐ 中高

**需要实现：**
```bash
# 方案 A: 通过 CLI 调用 API
memoh skill install ./my-skill.skill
memoh skill install --url https://example.com/skill.skill
memoh skill list
memoh skill uninstall skill-name

# 方案 B: 直接在 CLI 实现安装逻辑
memoh skill install ./my-skill.skill --local
```

**工作量估算：** 3-5 天

### 3.3 需求3：内容生成相关技能

**可行度：** ⭐⭐⭐ 中等（依赖 bundled resources）

**依赖关系：**
```
ppt-generation
  └── depends on: image-generation
  └── depends on: bundled resources (scripts/)

image-generation
  └── depends on: bundled resources (scripts/generate.py)

chart-visualization
  └── depends on: bundled resources (references/)
  └── depends on: nodejs runtime
```

**工作量估算：**
- 实现 bundled resources 支持：1-2 周
- 移植 chart-visualization：3-5 天
- 移植 ppt-generation：5-7 天
- 移植 image-generation：5-7 天

---

## 4. 建议的实施方案

### Phase 1: 基础补齐（2 周）

1. **实现 Bundled Resources 支持**
   - 修改 `SkillInstaller` 支持多文件
   - 修改 `SkillV2` 包含资源列表
   - 实现资源加载 API

2. **添加 CLI 安装命令**
   - 实现 `memoh skill install/uninstall/list`

### Phase 2: 核心技能移植（2 周）

1. **移植 deep-research**（纯文档，最简单）
2. **移植 chart-visualization**（需 references/ 支持）
3. **移植 skill-creator**（需 agents/ 支持）

### Phase 3: 内容生成技能（3 周）

1. **评估 image-generation 可行性**
   - DeerFlow 使用特定 image-gen provider
   - 需要评估 Memoh 如何集成

2. **移植 ppt-generation**
   - 依赖 image-generation
   - 需要 Python 脚本执行环境

3. **评估 video/podcast 生成**
   - 需要额外研究实现方案

### Phase 4: 评估框架（3-4 周）

1. **实现基础评估系统**
   - evals.json 格式支持
   - 测试用例执行

2. **实现基准测试**
   - with-skill vs without-skill
   - benchmark.json 生成

3. **实现描述优化**
   - 触发率评估
   - 自动优化循环

---

## 5. 总结

### 当前对齐状态：40%

- ✅ **已对齐**：基础架构（元数据、打包、API）
- ❌ **缺失**：技能生态、评估框架、CLI

### 最大差距

1. **没有预置技能库** - 用户无法开箱即用
2. **没有评估框架** - 无法保证技能质量
3. **不支持 bundled resources** - 复杂技能无法实现

### 建议下一步

1. **立即**：实现 bundled resources 支持（解锁复杂技能）
2. **短期**：移植 deep-research + chart-visualization（提供即时价值）
3. **中期**：评估并移植内容生成技能
4. **长期**：实现完整评估框架

---

*分析时间：2026-03-26*
*DeerFlow 版本：main 分支*
*Memoh 版本：main 分支 + V2 实现*
