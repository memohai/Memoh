<script setup lang="ts">
import { computed } from 'vue'
import MarkdownRender, { enableKatex, enableMermaid, setCustomComponents } from 'markstream-vue'
import ThemedMermaidBlock from '@/components/themed-mermaid-block/index.vue'
import { useSettingsStore } from '@/store/settings'

const props = withDefaults(defineProps<{
  content: string
  class?: string
}>(), {
  class: undefined,
})

enableKatex()
enableMermaid()
setCustomComponents({ mermaid: ThemedMermaidBlock })

const settings = useSettingsStore()
const isDark = computed(() => settings.theme === 'dark')
const codeBlockMonacoOptions = computed(() => ({
  fontFamily: settings.codeFontStack,
  fontSize: settings.codeFontSizePx,
}))
const codeBlockTheme = computed(() => ({
  light: settings.shikiThemeLight,
  dark: settings.shikiThemeDark,
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
        :code-block-monaco-options="codeBlockMonacoOptions"
        :theme="codeBlockTheme"
        custom-id="file-preview-md"
      />
    </div>
  </div>
</template>
