<template>
  <div class="space-y-1">
    <div
      v-for="(msg, idx) in messages"
      :key="msg.id || idx"
      class="group relative rounded-lg border p-3 transition-colors hover:bg-muted/50"
    >
      <div class="flex items-start gap-3">
        <!-- Role Icon -->
        <div
          class="flex size-8 shrink-0 items-center justify-center rounded-full"
          :class="roleIconClass(msg.role)"
        >
          <FontAwesomeIcon
            :icon="roleIcon(msg.role)"
            class="size-3.5 text-white"
          />
        </div>

        <!-- Content -->
        <div class="min-w-0 flex-1 space-y-1.5">
          <!-- Top row -->
          <div class="flex flex-wrap items-center gap-2 text-sm">
            <Badge
              :variant="roleBadgeVariant(msg.role)"
              class="text-xs font-medium"
            >
              {{ roleLabel(msg.role) }}
            </Badge>
            <span
              v-if="msg.sender_display_name"
              class="font-medium truncate max-w-[200px]"
            >
              {{ msg.sender_display_name }}
            </span>
            <Badge
              v-if="msg.platform"
              variant="outline"
              class="text-[10px] uppercase h-5"
            >
              {{ msg.platform }}
            </Badge>
            <span class="text-xs text-muted-foreground ml-auto shrink-0">
              {{ formatTime(msg.created_at) }}
            </span>
          </div>

          <!-- Message content -->
          <div
            class="text-sm leading-relaxed"
            :class="{ 'font-mono text-xs': msg.role === 'tool' || msg.role === 'system' }"
          >
            <div
              class="whitespace-pre-wrap break-words [overflow-wrap:anywhere]"
              :class="{ 'line-clamp-4': !expandedIds.includes(msgKey(msg, idx)) }"
            >
              {{ formatContent(msg.content) }}
            </div>
            <button
              v-if="isContentLong(msg.content)"
              class="mt-1 text-xs text-primary hover:underline"
              @click="toggleExpand(msgKey(msg, idx))"
            >
              {{ expandedIds.includes(msgKey(msg, idx)) ? collapseLabel : expandLabel }}
            </button>
          </div>

          <!-- Usage -->
          <div
            v-if="hasUsage(msg.usage)"
            class="flex items-center gap-3 text-xs text-muted-foreground pt-1"
          >
            <span class="inline-flex items-center gap-1">
              <FontAwesomeIcon
                :icon="['fas', 'chart-bar']"
                class="size-3"
              />
              {{ formatUsage(msg.usage) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@memoh/ui'
import { formatDateTime } from '@/utils/date-time'

export interface MessageItem {
  id?: string
  role?: string
  content?: unknown
  sender_display_name?: string
  sender_avatar_url?: string
  platform?: string
  created_at?: string
  usage?: unknown
  metadata?: Record<string, unknown>
  [key: string]: unknown
}

defineProps<{
  messages: MessageItem[]
}>()

const { t } = useI18n()

const expandedIds = ref<string[]>([])

const expandLabel = computed(() => t('bots.history.expandContent'))
const collapseLabel = computed(() => t('bots.history.collapseContent'))

function msgKey(msg: MessageItem, idx: number): string {
  return msg.id || String(idx)
}

function toggleExpand(id: string) {
  if (expandedIds.value.includes(id)) {
    expandedIds.value = expandedIds.value.filter(v => v !== id)
  } else {
    expandedIds.value = [...expandedIds.value, id]
  }
}

function roleIcon(role?: string): [string, string] {
  switch (role) {
    case 'user': return ['fas', 'user']
    case 'assistant': return ['fas', 'robot']
    case 'tool': return ['fas', 'wrench']
    case 'system': return ['fas', 'laptop-code']
    default: return ['fas', 'circle-question']
  }
}

function roleIconClass(role?: string): string {
  switch (role) {
    case 'user': return 'bg-blue-500'
    case 'assistant': return 'bg-emerald-500'
    case 'tool': return 'bg-amber-500'
    case 'system': return 'bg-slate-500'
    default: return 'bg-muted-foreground'
  }
}

function roleBadgeVariant(role?: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (role) {
    case 'user': return 'default'
    case 'assistant': return 'secondary'
    case 'tool': return 'outline'
    case 'system': return 'outline'
    default: return 'outline'
  }
}

function roleLabel(role?: string): string {
  const key = `bots.history.role.${role || 'system'}`
  const val = t(key)
  return val !== key ? val : (role || 'unknown')
}

function formatTime(val?: string): string {
  return formatDateTime(val, { fallback: '-' })
}

function formatContent(content: unknown): string {
  if (!content) return ''
  try {
    if (Array.isArray(content)) {
      const decoder = new TextDecoder()
      const str = decoder.decode(new Uint8Array(content as number[]))
      try {
        const parsed = JSON.parse(str)
        if (typeof parsed === 'string') return parsed
        return JSON.stringify(parsed, null, 2)
      } catch {
        return str
      }
    }
    if (typeof content === 'string') return content
    if (typeof content === 'object') return JSON.stringify(content, null, 2)
    return String(content)
  } catch {
    return String(content)
  }
}

function isContentLong(content: unknown): boolean {
  const text = formatContent(content)
  return text.length > 300 || text.split('\n').length > 4
}

function hasUsage(usage: unknown): boolean {
  if (!usage) return false
  if (Array.isArray(usage) && usage.length > 0) return true
  if (typeof usage === 'object' && Object.keys(usage as object).length > 0) return true
  return false
}

function formatUsage(usage: unknown): string {
  if (!usage) return ''
  try {
    if (Array.isArray(usage)) {
      const decoder = new TextDecoder()
      const str = decoder.decode(new Uint8Array(usage as number[]))
      try {
        const parsed = JSON.parse(str)
        if (typeof parsed === 'object' && parsed !== null) {
          const parts: string[] = []
          for (const [k, v] of Object.entries(parsed)) {
            parts.push(`${k}: ${v}`)
          }
          return parts.join(' | ')
        }
        return str
      } catch {
        return str
      }
    }
    if (typeof usage === 'object') {
      const parts: string[] = []
      for (const [k, v] of Object.entries(usage as Record<string, unknown>)) {
        parts.push(`${k}: ${v}`)
      }
      return parts.join(' | ')
    }
    return String(usage)
  } catch {
    return ''
  }
}
</script>
