<script setup lang="ts">
import { computed } from 'vue'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'

const props = withDefaults(defineProps<{
  content: string
  class?: string
}>(), {
  class: undefined,
})

enableKatex()
enableMermaid()

const settings = useSettingsStore()
const isDark = computed(() => settings.theme === 'dark')
</script>

<template>
  <div :class="['h-full w-full min-h-0 overflow-auto bg-card', props.class]">
    <div class="prose prose-sm dark:prose-invert max-w-none px-6 py-4 *:first:mt-0">
      <MarkdownRender
        :content="props.content"
        :is-dark="isDark"
        :typewriter="false"
        :fade="false"
        custom-id="file-preview-md"
      />
    </div>
  </div>
</template>
