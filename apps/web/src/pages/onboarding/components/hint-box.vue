<template>
  <!-- wizard 表单里的单行提示盒 owner:一段 text-xs 说明,可选前置图标。
       收编前 6 处手写盒在圆角(rounded-md/lg)、内边距(px-3 py-2 / py-2.5 /
       px-4 py-3)上三套写法互不一致;统一到 rounded-lg + px-3 py-2.5。
       与 CalloutBanner 是不同关系(那是"标题+描述+操作"的页面级横幅,这是
       表单内的无标题小提示,census 裁定过强并属 over-merge);与 ACP 身份
       横幅(Step4 的 agent banner)也不同——那是带品牌图标的 text-sm 状态行,
       留在调用方。 -->
  <div
    class="rounded-lg border px-3 py-2.5 text-xs leading-relaxed"
    :class="toneClass"
  >
    <div
      v-if="$slots.icon"
      class="flex items-start gap-2.5"
    >
      <slot name="icon" />
      <div class="min-w-0">
        <slot />
      </div>
    </div>
    <slot v-else />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  tone?: 'muted' | 'warning'
}>(), {
  tone: 'muted',
})

// 逐 tone 全量字面 class(Tailwind 扫描源码字面量,不能运行时拼接)
const toneClass = computed(() =>
  props.tone === 'warning'
    ? 'border-warning-border bg-warning-soft text-warning-foreground'
    : 'border-border bg-muted-soft text-muted-foreground',
)
</script>
