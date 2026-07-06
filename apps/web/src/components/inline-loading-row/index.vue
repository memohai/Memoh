<template>
  <!-- 行内左对齐的加载态:一个小 spinner + 一段 muted 文案,占一行。
       区别于 PanePlaceholder 的居中占位 —— 这个不撑满高度、不居中,就地占据它所在的
       那一行(设置行、列表头、面板顶部)。抽出前 dockview/bots 多个面各写一遍同一套
       `flex items-center gap-2 ... text-muted-foreground` + Spinner + 文案;其中 backup /
       import 两处的"带框"变体(rounded-md border p-3)更是逐字复制。
       定位用的内边距(px-2 / py-8 等)因所在容器而异,留给调用方经 class 传入 —— 单根节点,
       Vue 会把外部 class 与内部基础 class 合并。组件只统一:flex 行、gap、字号、muted 色、
       spinner 尺寸,以及可选的带框盒子。 -->
  <div
    :class="[
      'flex items-center gap-2 text-muted-foreground',
      size === 'md' ? 'text-sm' : 'text-xs',
      bordered && 'rounded-md border border-border/60 bg-background p-3',
    ]"
  >
    <Spinner :class="size === 'md' ? 'size-4' : 'size-3.5'" />
    <span v-if="$slots.default"><slot /></span>
  </div>
</template>

<script setup lang="ts">
import { Spinner } from '@memohai/ui'

withDefaults(defineProps<{
  /** spinner + 字号档:sm=text-xs/size-3.5(默认),md=text-sm/size-4。 */
  size?: 'sm' | 'md'
  /** 带框变体:套一层 rounded-md border + bg-background + p-3(用于对话体内的读取态)。 */
  bordered?: boolean
}>(), {
  size: 'sm',
})
</script>
