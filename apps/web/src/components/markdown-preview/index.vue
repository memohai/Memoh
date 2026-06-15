<script setup lang="ts">
import { computed } from 'vue'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
import { registerSharedMarkdownComponents } from '@/components/markdown'

const props = withDefaults(defineProps<{
  content: string
  class?: string
}>(), {
  class: undefined,
})

enableKatex()
enableMermaid()
// File preview reuses the chat's design-system node components (library
// Checkbox task markers, link-language footnotes). It keeps markstream's own
// Monaco code block, so no code_block override here.
registerSharedMarkdownComponents('file-preview-md')

const settings = useSettingsStore()
const isDark = computed(() => settings.theme === 'dark')
const codeBlockMonacoOptions = computed(() => ({
  fontFamily: settings.codeFontStack,
  fontSize: settings.codeFontSizePx,
}))
const codeFontRenderKey = computed(() => settings.codeFontStack)
</script>

<template>
  <div :class="['h-full w-full min-h-0 overflow-auto bg-surface-editor', props.class]">
    <div class="prose prose-sm dark:prose-invert max-w-none px-6 py-4 *:first:mt-0">
      <MarkdownRender
        :key="codeFontRenderKey"
        :content="props.content"
        :is-dark="isDark"
        :typewriter="false"
        :fade="false"
        :show-tooltips="false"
        :mermaid-props="{ showTooltips: false }"
        :code-block-monaco-options="codeBlockMonacoOptions"
        custom-id="file-preview-md"
      />
    </div>
  </div>
</template>
