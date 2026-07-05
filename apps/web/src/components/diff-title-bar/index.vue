<template>
  <!-- 编辑器对比/差异视图顶部的标题条。抽取前 file-viewer(compare 视图)与
       monaco-editor/diff.vue 各手写一份逐字节相同的容器 class,靠手工同步。
       条内内容两处结构不同(一处是复合文案+操作按钮,一处是左右文件名),
       所以 owner 只收容器:布局、分隔线、内边距、字号、muted 色;
       内容经默认 slot(左侧)与 #actions(右侧,可选)传入。
       目前仅这 2 处调用;若第三个调用方需要不同的容器布局,优先考虑
       内联回调用点或分叉,而不是给这里加 prop。 -->
  <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border px-3 py-1.5 text-caption text-muted-foreground">
    <slot />
    <div
      v-if="$slots.actions"
      class="flex items-center gap-1.5"
    >
      <slot name="actions" />
    </div>
  </div>
</template>
