<template>
  <div
    class="w-full rounded-lg border border-event-schedule-border bg-event-schedule-soft px-4 py-3 cursor-pointer transition-colors hover:bg-event-schedule-soft/80"
    @click="navigateToSchedule"
  >
    <div class="flex items-center justify-between mb-2">
      <div class="flex items-center gap-2 text-xs font-medium text-event-schedule-foreground">
        <Clock class="size-3.5" />
        {{ t('chat.scheduleTrigger') }}
      </div>
      <div class="flex items-center gap-1 text-[11px] text-event-schedule-foreground/70">
        {{ t('chat.viewSchedule') }}
        <ExternalLink class="size-3" />
      </div>
    </div>
    <div class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
      <span
        v-if="parsed.name"
        class="text-muted-foreground"
      >{{ t('chat.scheduleName') }}</span>
      <span v-if="parsed.name">{{ parsed.name }}</span>
      <span
        v-if="parsed.pattern"
        class="text-muted-foreground"
      >{{ t('chat.schedulePattern') }}</span>
      <span v-if="parsed.pattern">
        <code class="text-[11px] px-1 py-0.5 rounded bg-event-schedule/10">{{ parsed.pattern }}</code>
      </span>
      <span
        v-if="parsed.description"
        class="text-muted-foreground"
      >{{ t('chat.scheduleDescription') }}</span>
      <span v-if="parsed.description">{{ parsed.description }}</span>
      <span
        v-if="parsed.maxCalls"
        class="text-muted-foreground"
      >{{ t('chat.scheduleMaxCalls') }}</span>
      <span v-if="parsed.maxCalls">{{ parsed.maxCalls }}</span>
    </div>
    <div
      v-if="parsed.command"
      class="mt-2 text-xs text-muted-foreground border-t border-event-schedule-border/60 pt-2"
    >
      <pre class="whitespace-pre-wrap break-all font-mono text-[11px]">{{ parsed.command }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Clock, ExternalLink } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

const props = defineProps<{
  content: string
  botId?: string
}>()

const { t } = useI18n()
const router = useRouter()

interface ScheduleInfo {
  name?: string
  description?: string
  pattern?: string
  maxCalls?: string
  command?: string
}

const parsed = computed<ScheduleInfo>(() => {
  const text = props.content ?? ''
  const frontmatterMatch = text.match(/---\n([\s\S]*?)\n---/)
  if (!frontmatterMatch) return {}

  const lines = (frontmatterMatch[1] ?? '').split('\n')
  const info: ScheduleInfo = {}
  for (const line of lines) {
    const idx = line.indexOf(':')
    if (idx < 0) continue
    const key = line.slice(0, idx).trim()
    const val = line.slice(idx + 1).trim()
    if (key === 'schedule-name') info.name = val
    else if (key === 'schedule-description') info.description = val
    else if (key === 'cron-pattern') info.pattern = val
    else if (key === 'max-calls') info.maxCalls = val
  }

  const afterFrontmatter = text.slice(text.indexOf('---', text.indexOf('---') + 3) + 3).trim()
  if (afterFrontmatter) {
    info.command = afterFrontmatter
  }

  return info
})

function navigateToSchedule() {
  if (!props.botId) return
  router.push({ name: 'bot-detail', params: { botId: props.botId }, query: { tab: 'schedule' } })
}
</script>
