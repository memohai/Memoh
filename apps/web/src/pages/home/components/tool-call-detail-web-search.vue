<template>
  <div class="space-y-1.5">
    <div
      v-if="results.length"
      class="space-y-1.5"
    >
      <div
        v-for="(item, i) in results"
        :key="i"
        class="flex flex-col gap-0.5"
      >
        <a
          :href="item.url"
          target="_blank"
          rel="noopener noreferrer"
          class="text-xs text-primary hover:underline truncate"
          :title="item.title"
        >
          {{ item.title }}
        </a>
        <span
          v-if="item.url"
          class="text-caption text-muted-foreground truncate"
          :title="item.url"
        >
          {{ item.url }}
        </span>
        <span
          v-if="item.description"
          class="text-caption text-muted-foreground/80 line-clamp-2"
        >
          {{ item.description }}
        </span>
      </div>
    </div>
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noResults') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

interface SearchResult {
  title: string
  url: string
  description?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const results = computed<SearchResult[]>(() => {
  if (!props.block.done || !props.block.result) return []
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc?.results ?? result.results) as SearchResult[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
