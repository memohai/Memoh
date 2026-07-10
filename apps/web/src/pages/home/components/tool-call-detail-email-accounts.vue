<template>
  <div class="space-y-1.5">
    <div
      v-if="accounts.length"
      class="space-y-1"
    >
      <div
        v-for="(item, i) in accounts"
        :key="item.id ?? item.email ?? i"
        class="flex items-center gap-2 text-xs"
      >
        <span class="text-foreground truncate flex-1">{{ item.email || item.id }}</span>
        <Badge
          v-if="item.provider"
          variant="secondary"
          size="sm"
          font="mono"
          class="shrink-0"
        >
          {{ item.provider }}
        </Badge>
      </div>
    </div>
    <EmptyRow v-else>
      {{ t('chat.tools.detail.noAccounts') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@felinic/ui'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'

interface AccountItem {
  id?: string
  email?: string
  provider?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const accounts = computed<AccountItem[]>(() => {
  if (!props.block.done) return []
  const r = resolveResult()
  if (!r) return []
  const items = r.accounts as AccountItem[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
