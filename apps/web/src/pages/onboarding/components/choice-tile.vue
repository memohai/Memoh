<template>
  <!-- onboarding 三选一网格里的选项砖:h-16 横排 图标+标签。Step3 原本三个 <button>
       手写同一形状(自定义/预设/ACP);代码还没漂移,趁早锁进 owner。两种语气:
       solid=既有对象(实框,hover 提边框+浅底),dashed=新建入口(虚框,muted 文字,
       hover 转正色)。图标尺寸留给调用方(ProviderIcon/lucide 尺寸接口不同)。
       单根 <button>,@click 等监听器自然落根,无需 emit 声明。 -->
  <button
    type="button"
    :class="[
      'flex h-16 items-center gap-2.5 rounded-lg border bg-background px-3 transition-colors',
      variant === 'dashed' ? dashedClass : solidClass,
    ]"
  >
    <slot name="icon" />
    <span class="truncate text-sm font-medium">{{ label }}</span>
  </button>
</template>

<script setup lang="ts">
// hover 是 owner 级刻意交互反馈:dashed 由 muted 转正色,solid 提边框+浅底
const dashedClass = 'border-dashed border-border text-muted-foreground hover:border-foreground/50 hover:text-foreground' /* ui-allow-style */
const solidClass = 'border-border hover:border-muted-foreground/50 hover:bg-accent/40' /* ui-allow-style */

withDefaults(defineProps<{
  label?: string
  /** solid=选既有对象;dashed=新建/自定义入口。 */
  variant?: 'solid' | 'dashed'
}>(), {
  variant: 'solid',
})
</script>
