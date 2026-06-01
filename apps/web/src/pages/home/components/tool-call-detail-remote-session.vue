<template>
  <div class="space-y-1.5">
    <div
      v-if="sessionId || status || cdpUrl"
      class="flex flex-col gap-0.5 text-xs"
    >
      <div
        v-if="status"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.status') }}
        </span>
        <span class="font-mono">{{ status }}</span>
      </div>
      <div
        v-if="sessionId"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.session') }}
        </span>
        <span
          class="font-mono truncate"
          :title="sessionId"
        >{{ sessionId }}</span>
      </div>
      <div
        v-if="cdpUrl"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.cdpUrl') }}
        </span>
        <span
          class="font-mono truncate"
          :title="cdpUrl"
        >{{ cdpUrl }}</span>
      </div>
    </div>

    <div
      v-if="targets.length"
      class="space-y-1"
    >
      <div class="text-[10px] uppercase tracking-wide text-muted-foreground/70">
        {{ t('chat.tools.detail.targets') }}
      </div>
      <div
        v-for="(target, i) in targets"
        :key="i"
        class="flex flex-col gap-0.5"
      >
        <span
          v-if="target.title"
          class="text-xs text-foreground truncate"
          :title="target.title"
        >{{ target.title }}</span>
        <span
          v-if="target.url"
          class="text-[10px] text-muted-foreground font-mono truncate"
          :title="target.url"
        >{{ target.url }}</span>
      </div>
    </div>

    <p
      v-if="!sessionId && !status && !cdpUrl && !targets.length"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noData') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

interface RemoteTarget {
  title?: string
  url?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

function resolveResult(): Record<string, unknown> {
  const result = asObject(props.block.result)
  const sc = asObject(result.structuredContent)
  return Object.keys(sc).length > 0 ? sc : result
}

const status = computed(() => {
  const r = resolveResult()
  return typeof r.status === 'string' ? r.status : ''
})

const sessionId = computed(() => {
  const r = resolveResult()
  return (typeof r.session_id === 'string' && r.session_id)
    || (typeof r.id === 'string' ? r.id : '')
})

const cdpUrl = computed(() => {
  const r = resolveResult()
  return (typeof r.cdp_url === 'string' && r.cdp_url)
    || (typeof r.connect_over_cdp === 'string' ? r.connect_over_cdp : '')
})

const targets = computed<RemoteTarget[]>(() => {
  const r = resolveResult()
  const raw = r.targets
  if (!Array.isArray(raw)) return []
  return raw
    .map((item) => {
      const obj = asObject(item)
      return {
        title: typeof obj.title === 'string' ? obj.title : '',
        url: typeof obj.url === 'string' ? obj.url : '',
      }
    })
    .filter(target => target.title || target.url)
})
</script>
