<template>
  <!-- 行内左对齐的加载态:一个小 spinner + 一段 muted 文案,占一行。
       区别于 PanePlaceholder 的居中占位 —— 这个不撑满高度、不居中,就地占据它所在的
       那一行(设置行、列表头、面板顶部)。抽出前 dockview/bots 多个面各写一遍同一套
       `flex items-center gap-2 ... text-muted-foreground` + Spinner + 文案;其中 backup /
       import 两处的"带框"变体(rounded-md border p-3)更是逐字复制。
       2026-07-06 教训:调用方定位 class 曾整段手抄,一处把"卡片内列表行"家族误抄成
       "整 tab"家族,读作半空的高盒子而非紧凑一行。两种具名家族收进 `surface` prop,
       调用方不再能抄错;真正局部、未具名的定位仍走 class 传入(单根节点,Vue 把外部
       class 与内部基础 class 合并)。 -->
  <div
    :class="[
      'flex items-center gap-2 text-muted-foreground',
      size === 'md' ? 'text-sm' : 'text-xs',
      bordered && 'rounded-md border border-border-soft bg-background p-3',
      surface === 'card-row' && 'mx-4 min-h-[3.75rem] border-b border-border py-3 last:border-b-0',
      surface === 'tab' && 'px-2 py-8',
    ]"
  >
    <Spinner :class="size === 'md' ? 'size-4' : 'size-3.5'" />
    <span v-if="$slots.default"><slot /></span>
  </div>
</template>

<script setup lang="ts">
import { Spinner } from '@felinic/ui'

withDefaults(defineProps<{
  /** spinner + 字号档:sm=text-xs/size-3.5(默认),md=text-sm/size-4。 */
  size?: 'sm' | 'md'
  /** 带框变体:套一层 rounded-md border + bg-background + p-3(用于对话体内的读取态)。 */
  bordered?: boolean
  /**
   * 具名定位家族,取代调用方手抄同一串 class:
   * `card-row` = SettingsSection 卡片内的列表行占位(带下边框,末行去边框);
   * `tab` = 整个 tab 无卡片包裹时的居中留白占位。
   * 不传则不加任何定位 class,由调用方经根 class 传入(局部、未具名的定位场景)。
   */
  surface?: 'card-row' | 'tab'
}>(), {
  size: 'sm',
})
</script>
