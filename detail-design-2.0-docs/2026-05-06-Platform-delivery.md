---
date: 2026-05-06
modified_files: 
  - apps/web/src/pages/bots/components/bot-channels.vue
  - apps/web/src/pages/bots/components/channel-settings-panel.vue
dependencies_changed: []
architectural_impact: High
---

# 全局变更摘要 (Executive Summary)
今日完成了 AI 智能体通道配置界面（Channels）的深度 UI/UX 架构重构。将原本拥挤的四栏平铺布局降维为“静音枢纽轨 + 盒中盒右侧工作台”模式，并施加了严格的 `max-w-4xl` 物理融合宽度。重构统一了 Top Action Bar 级别的状态流管理，实现了微交互级的“柔性脏状态追踪”，并对危险操作区（Danger Zone）和高级配置区（Advanced Settings）进行了符合“渐进式披露”原则的隔离，极大地降低了认知负荷并对齐了系统的全局视觉规范。

# 代码模块 1:1 原子级解析 (Atomic Parsing & Porting)

## <module-id>L3: Silent Hub Rail (静音枢纽轨)</module-id>
- **模块定位:** `apps/web/src/pages/bots/components/bot-channels.vue` 
- **原子解释:** 
  - **变更:**移除了强烈的选中态背景色，压缩内边距至 `py-1.5`，并在列表项增加微型星号 `*` 标识脏状态。将原本无边界延展的父容器收拢为 `max-w-4xl` 的集中式布局。所有占位注释转为英文。
  - **架构意图 (Why):** 作为宏观导航枢纽，通过极致压缩与降噪处理，使其在不喧宾夺主的前提下提供“一页全感知”的能力，消解多栏布局带来的物理空间挤压感。
- **移植与同步指南 (Porting Guide):** 
  - 依赖 `@dirty-change` 事件发射器与本地 `<state-mutation>dirtyStates</state-mutation>` 状态同步，不涉及底层 API 改变。跨端同步时需遵循相似的高密度列表渲染和静音交互范式。

## <module-id>L4: Global Context Header & Micro-copy (全局主权头部与微文案)</module-id>
- **模块定位:** `apps/web/src/pages/bots/components/channel-settings-panel.vue` (Top Action Bar 区域)
- **原子解释:**
  - **变更:** 置顶引入了 `border-b pb-4 sticky top-0` 的头部架构，并将 <porting-contract>Save</porting-contract> 按钮强行提拉至该区域右上角，完全抛弃了原有的底部局部保存按钮。左侧加入基于 <state-mutation>isFormDirty</state-mutation> 计算的幽灵淡入淡出（Fade-in）微文案提示。
  - **架构意图 (Why):** 确立明确的“保存作用域”边界。解决用户在超长动态表单底部寻找保存操作的痛点，确保关键操作全局可见。
- **移植与同步指南:** 
  - 所有表单级别的状态变更必须向上传递给 Top Header。微文案的展示严格绑定 `isFormDirty` 和 `props.allDirtyStates`。

## <module-id>L4: Progressive Disclosure & Danger Zone (渐进式高级配置与危险区)</module-id>
- **模块定位:** `apps/web/src/pages/bots/components/channel-settings-panel.vue` (Advanced Settings & Danger Zone)
- **原子解释:**
  - **变更:** 非必填项被封装于带有 `Expand All / Collapse` 双控按钮的折叠面板中。底部插入严格隔离的 Danger Zone 容器（无色卡片，红色警告字），嵌套 `<ConfirmPopover>`。动态表单渲染逻辑从 JSX 重写回 Vue 原生 `v-if/v-else-if` 模板语法以修复 Vite 构建解析错误。
  - **架构意图 (Why):** 落实“分段式披露”原则，隐藏次要噪音。通过空间屏障 (`pt-4`) 和视觉克制增强破坏性操作的心理摩擦力。
- **移植与同步指南:**
  - 注意动态表单域 `<module-id>RenderFieldInput</module-id>` 已完全内联为模板代码。Danger Zone 的确认文本维持基于 `$t()` 的标准 i18n 变量映射以支持国际化。
