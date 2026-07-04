<template>
  <!-- 面板/区域内容为空时的居中占位,三种呈现:
       - loading:横向 spinner + 默认插槽文案。
       - title(传了 title prop):两行强调式 —— 加粗前景标题 + 默认插槽作 muted 副标题。
         用于"整个区域为空"(未选中 bot、工作区无内容),语气比 icon 态更主动。
       - 缺省(空态):纵向 icon + 默认插槽文案 + 可选 #action 按钮。用于"单项无预览"。
       全部 h-full 借用容器高度,内容态切换不 reflow。
       抽出前这三套在 dockview 的 panel-asset/panel-preview、file-manager/file-viewer、
       home/index、chat-pane、workspace-watermark 各被逐字复制了一遍。
       刻意不复用 @memohai/ui 的 Empty:那是带虚线边框 + 大留白(p-12 gap-6)的卡片式空态,
       而这里要的是极简、填满面板、无边框的内容区占位 —— 两种不同的空态语言。
       icon 的具体样式(size/opacity/destructive 色)留给调用方在 #icon 内自持,因为各面板的
       图标语义不同(缺文件 vs 已删除),组件只统一容器、间距与文案排版。 -->
  <div
    v-if="loading"
    class="flex h-full items-center justify-center text-muted-foreground"
  >
    <Spinner class="mr-2" />
    <slot />
  </div>
  <div
    v-else-if="title"
    class="flex h-full items-center justify-center px-6 text-center"
  >
    <div>
      <p class="text-xs font-medium text-foreground">
        {{ title }}
      </p>
      <p class="mt-1 text-xs text-muted-foreground">
        <slot />
      </p>
    </div>
  </div>
  <div
    v-else
    class="flex h-full flex-col items-center justify-center gap-3 text-center text-muted-foreground"
  >
    <slot name="icon" />
    <p
      v-if="$slots.default"
      class="text-xs"
    >
      <slot />
    </p>
    <slot name="action" />
  </div>
</template>

<script setup lang="ts">
import { Spinner } from '@memohai/ui'

defineProps<{
  /** 加载态:横向渲染内置 Spinner + 默认插槽文案。与 title 互斥(loading 优先)。 */
  loading?: boolean
  /** 传入即切到两行强调式空态:此为加粗标题,默认插槽为其下的 muted 副标题。 */
  title?: string
}>()
</script>
