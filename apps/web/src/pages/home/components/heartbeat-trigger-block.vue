<template>
  <div
    class="w-full rounded-lg border border-event-heartbeat-border bg-event-heartbeat-soft px-4 py-3 cursor-pointer transition-colors hover:bg-event-heartbeat-soft/80"
    @click="navigateToLogs"
  >
    <div class="flex items-center justify-between mb-2">
      <div class="flex items-center gap-2 text-xs font-medium text-event-heartbeat-foreground">
        <HeartPulse class="size-3.5" />
        {{ t('chat.heartbeatTrigger') }}
      </div>
      <div class="flex items-center gap-1 text-[11px] text-event-heartbeat-foreground/70">
        {{ t('chat.viewHeartbeatLogs') }}
        <ExternalLink class="size-3" />
      </div>
    </div>
    <div class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
      <span
        v-if="parsed.time"
        class="text-muted-foreground"
      >{{ t('chat.heartbeatTime') }}</span>
      <span v-if="parsed.time">{{ parsed.time }}</span>
      <span
        v-if="parsed.interval"
        class="text-muted-foreground"
      >{{ t('chat.heartbeatInterval') }}</span>
      <span v-if="parsed.interval">{{ parsed.interval }}</span>
      <span
        v-if="parsed.lastHeartbeat"
        class="text-muted-foreground"
      >{{ t('chat.heartbeatLastAt') }}</span>
      <span v-if="parsed.lastHeartbeat">{{ parsed.lastHeartbeat }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { HeartPulse, ExternalLink } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

const props = defineProps<{
  content: string
  botId?: string
}>()

const { t } = useI18n()
const router = useRouter()

interface HeartbeatInfo {
  interval?: string
  time?: string
  lastHeartbeat?: string
}

const parsed = computed<HeartbeatInfo>(() => {
  const text = props.content ?? ''
  const frontmatterMatch = text.match(/---\n([\s\S]*?)\n---/)
  if (!frontmatterMatch) return {}

  const lines = (frontmatterMatch[1] ?? '').split('\n')
  const info: HeartbeatInfo = {}
  for (const line of lines) {
    const idx = line.indexOf(':')
    if (idx < 0) continue
    const key = line.slice(0, idx).trim()
    const val = line.slice(idx + 1).trim()
    if (key === 'interval') info.interval = val
    else if (key === 'time') info.time = val
    else if (key === 'last_heartbeat') info.lastHeartbeat = val
  }
  return info
})

function navigateToLogs() {
  if (!props.botId) return
  router.push({ name: 'bot-detail', params: { botId: props.botId }, query: { tab: 'heartbeat' } })
}
</script>
