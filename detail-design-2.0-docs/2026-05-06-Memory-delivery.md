---
date: 2026-05-06
modified_files:
  - apps/web/src/pages/bots/components/bot-memory.vue
  - apps/web/src/i18n/locales/en.json
  - apps/web/src/i18n/locales/zh.json
  - Design-new-setting-two-col.md
dependencies_changed:
  - @memohai/ui (Card, Tooltip, Popover, Empty, Separator, Skeleton)
  - lucide-vue-next (Zap, BrainCircuit)
architectural_impact: High
---

## 全局变更摘要 (Executive Summary)
今日针对 Bot 记忆管理模块进行了架构级的 UI/UX 深度重构。剥离了高阻断性的全局模态框，将高频但破坏心流的压缩操作降维至浮动 <module-id>Popover</module-id>；同时统一了左右双栏面板的绝对物理尺寸（锁定 240px 左导轨），使 Channels 与 Memory 面板实现视线无缝切换。彻底规范了“脏状态”的静默反馈链，提升了 B2B 复杂配置界面的结构连贯性。

## 代码模块 1:1 原子级解析 (Atomic Parsing & Porting)

### 1. 全局双栏尺寸规范收敛
- **模块定位:** `Design-new-setting-two-col.md` 
- **原子解释:** 将规范从业务描述抽象为绝对物理法则。规定 `<module-id>Master Rail</module-id>` 宽度强制为 `w-60` (240px)，`<module-id>Gutter</module-id>` 强制为 `gap-6`，`<module-id>Global Wrapper</module-id>` 强制为 `max-w-4xl`。
- **移植与同步指南:** 
  - <porting-contract>此为系统级基准</porting-contract>：未来所有引入左侧侧边栏设置的页面，其顶层 CSS Grid 结构必须 100% 对齐此设定，严禁使用柔性 `w-64` 或 `%` 导致跨 Tab 切换时的视觉跳动。

### 2. 记忆导航轨宽度对齐 (Master Rail Alignment)
- **模块定位:** `apps/web/src/pages/bots/components/bot-memory.vue` (Template > Left Rail)
- **原子解释:** 将左侧包裹容器的宽度类名从 `w-64` 修正为 `w-60`。
- **移植与同步指南:** 
  - 此项变更消除了 Memory Tab 与 Channels Tab 在切换时的闪烁。其他独立 Tab 开发需参考此 DOM 节点挂载尺寸。

### 3. Compact 压缩机制模态降维 (Modal to Popover)
- **模块定位:** `apps/web/src/pages/bots/components/bot-memory.vue` (Template & Script Setup)
- **原子解释:** 移除了底部的全屏 `<module-id>Dialog</module-id>`，在左侧导航栏头部的 `Brain` 按钮处嵌套了一层带有指向性的 `<module-id>Popover</module-id>`。将压缩比例单选列表变更为带 `Lucide` 图标的高密度行内单选按钮。状态绑定从 `<state-mutation>compactDialogOpen</state-mutation>` 变更为 `<state-mutation>compactPopoverOpen</state-mutation>`。修复了内部与 `<module-id>TooltipTrigger</module-id>` 作用域冲突的事件冒泡问题。
- **移植与同步指南:** 
  - 核心 Side Effect: 触发 `handleCompact` 成功后关闭 Popover 状态。此无头组件的事件传播隔离（通过引入纯净 `<div>` 容器）是前端构建同类高复合动作按钮时的标准 Paradigm。

### 4. 新建记忆的盒中盒重构 (Side-by-Side Modal)
- **模块定位:** `apps/web/src/pages/bots/components/bot-memory.vue` (Template > New Memory Dialog)
- **原子解释:** 将原本拥挤的垂直结构扩展为 `sm:max-w-4xl` 的横向 1:1 等分结构。左侧为历史检索框（带 `border-dashed` 空状态），右侧为 `Memory Content` 书写区（包裹在 `@memohai/ui` 的 `<module-id>Card</module-id>` 容器内），并在脏状态时引入 `*` 星号追踪。左右侧头部强行对齐为固定高度 `h-10`。
- **移植与同步指南:** 
  - 新增组件级契约：强制内部嵌套元素的曲率对齐为 `rounded-md`，避免父子边界圆角错位。

### 5. I18n 文案与注释重构
- **模块定位:** `apps/web/src/i18n/locales/zh.json` & `en.json`
- **原子解释:** 新增词条 `emptyHistory` 和 `unsavedChanges`。重写 `compactConfirm` 以符合 <porting-contract>humanize-zh</porting-contract> 指南（删除“可能”，直接陈述“减少条目数量”），去除了 AI 机器人的迂回语气。所有代码注释统一规范化为英语系统开发指令格式。
- **移植与同步指南:** 
  - 所有脏状态追踪依赖 `isDirty` Computed。未保存时的“幽灵提示 (Ghost Micro-copy)”现已实现，需配合 `Save` 按钮旁的淡入动效，属于跨端应同步保持的最高优先级状态感知。