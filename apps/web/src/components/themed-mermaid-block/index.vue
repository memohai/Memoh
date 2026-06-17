<script setup lang="ts">
import { computed } from 'vue'
import { MermaidBlockNode, type CodeBlockNode } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
import { applyMermaidThemeToSource, resolveMermaidIsDark } from '@/store/settings/mermaid'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  node: CodeBlockNode
  loading?: boolean
  isDark?: boolean
}>()

const settings = useSettingsStore()

const themedNode = computed<CodeBlockNode>(() => {
  if (settings.mermaidTheme === 'auto') return props.node
  const content = props.node?.content ?? ''
  const next = applyMermaidThemeToSource(content, settings.mermaidTheme)
  if (next === content) return props.node
  return { ...props.node, content: next }
})

const themedIsDark = computed(() =>
  resolveMermaidIsDark(settings.mermaidTheme, Boolean(props.isDark)),
)
</script>

<template>
  <MermaidBlockNode
    v-bind="$attrs"
    :node="themedNode"
    :loading="loading"
    :is-dark="themedIsDark"
  />
</template>
