<template>
  <!-- dockview 面板宿主壳的唯一 owner。抽出前 7 个 panel-*.vue(chat/browser/display/
       terminal/file/preview/asset)各手写一遍同一副骨架:`flex flex-col h-full w-full`
       根 + `flex-1 min-h-0` 内容层;漂移全是意外——class 顺序翻转(asset)、`relative`
       只出现在 3/7 且无注释、`bg-surface-editor` 只在编辑器面。统一为:根恒为 relative
       (面板恰好铺满 dock 槽位,对无绝对定位后代的面无影响;browser/display/terminal 的
       浮层本就依赖它),编辑器底色收进 editorSurface prop。
       #header 放随面板宽度伸缩的顶部条(file/preview 的面包屑);默认 slot 是面板主体,
       由壳负责 min-h-0 约束 —— 各面不再自己记这条 flex 溢出咒语。 -->
  <div
    class="relative flex h-full w-full flex-col"
    :class="editorSurface && 'bg-surface-editor'"
  >
    <slot name="header" />
    <div class="min-h-0 flex-1">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  /** 编辑器类面板(file/preview/asset)的 bg-surface-editor 底色。 */
  editorSurface?: boolean
}>()
</script>
