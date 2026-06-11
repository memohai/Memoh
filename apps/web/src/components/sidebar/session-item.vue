<template>
  <div
    role="button"
    tabindex="0"
    class="group relative flex items-center h-9 w-full rounded-md px-2.5 text-left transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    :class="isActive ? 'bg-sidebar-accent' : 'hover:bg-[color:var(--sidebar-hover)]'"
    :title="hoverTitle"
    @click="$emit('select', session)"
    @keydown.enter.prevent="$emit('select', session)"
    @keydown.space.prevent="$emit('select', session)"
  >
    <Avatar
      v-if="isIMSession"
      class="size-6 shrink-0 mr-2"
    >
      <AvatarImage
        v-if="avatarUrl"
        :src="avatarUrl"
        :alt="displayLabel"
      />
      <AvatarFallback class="text-[9px]">
        {{ avatarFallback }}
      </AvatarFallback>
    </Avatar>
    <component
      :is="typeIcon"
      v-else-if="typeIcon"
      class="size-4 shrink-0 mr-2"
      :class="typeIconClass"
    />

    <span class="flex-1 min-w-0 truncate text-control text-foreground">
      {{ session.title || t('chat.untitledSession') }}
    </span>

    <!-- Trailing slot: the timestamp and the actions button share one spot and
         CROSSFADE on row hover (time fades out, dots fade in) instead of a hard
         display swap. The time stays in flow (it sizes the slot so nothing
         shifts); the button overlays it absolutely and only takes pointer/focus
         while it's actually shown. -->
    <div class="relative ml-1.5 flex h-6 shrink-0 items-center justify-end">
      <LoaderCircle
        v-if="streaming"
        class="size-3 animate-spin text-muted-foreground"
        :aria-label="t('chat.sessionStreaming')"
      />
      <span
        v-else-if="session.updated_at"
        class="text-caption text-muted-foreground transition-opacity duration-150"
        :class="menuOpen ? 'opacity-0' : 'opacity-100 group-hover:opacity-0'"
      >
        {{ formatTime(session.updated_at) }}
      </span>

      <DropdownMenu v-model:open="menuOpen">
        <DropdownMenuTrigger as-child>
          <!-- Plain button (not <Button variant="ghost">): the ghost chip color
               (--btn-ghost-hover) is the same gray as the row's own hover, so the
               button's hover was invisible while sitting on a hovered row. A
               translucent foreground mix darkens whatever is behind it, so the
               chip reads clearly on top of both the hover and active row fills. -->
          <button
            type="button"
            class="absolute inset-y-0 right-0 my-auto inline-flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground outline-none transition-[opacity,background-color,color] duration-150 hover:bg-[color-mix(in_oklab,var(--foreground)_12%,transparent)] hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-[color-mix(in_oklab,var(--foreground)_12%,transparent)] data-[state=open]:text-foreground"
            :class="menuOpen ? 'opacity-100' : 'opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto'"
            :aria-label="t('chat.sessionActions')"
            @click.stop
            @keydown.enter.stop
            @keydown.space.stop
          >
            <MoreHorizontal class="size-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          @click.stop
        >
          <DropdownMenuItem
            @select="$emit('rename', session)"
          >
            <Pencil class="mr-2 size-3.5" />
            {{ t('chat.renameSession') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            class="text-destructive focus:text-destructive"
            @select="$emit('delete', session)"
          >
            <Trash2 class="mr-2 size-3.5" />
            {{ t('chat.deleteSession') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { HeartPulse, Clock, GitBranch, LoaderCircle, MoreHorizontal, Pencil, Trash2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { SessionSummary } from '@/composables/api/useChat'
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import { acpAgentDisplayName, acpAgentIcon, normalizeACPAgentID } from '@/utils/acp'

const props = defineProps<{
  session: SessionSummary
  isActive: boolean
  streaming?: boolean
}>()

defineEmits<{
  select: [session: SessionSummary]
  rename: [session: SessionSummary]
  delete: [session: SessionSummary]
}>()

const { t } = useI18n()

const menuOpen = ref(false)

const WEB_CHANNELS = new Set(['local', ''])

const isIMSession = computed(() => {
  const ct = (props.session.channel_type ?? '').trim().toLowerCase()
  return ct !== '' && !WEB_CHANNELS.has(ct)
})

// Plain chat/discuss rows are text-only; typed sessions get a small inline
// glyph in their event accent color. No circular icon containers.
const typeIcon = computed<Component | null>(() => {
  switch (props.session.type) {
    case 'heartbeat': return HeartPulse
    case 'schedule': return Clock
    case 'subagent': return GitBranch
    case 'acp_agent': return acpAgentIcon(acpAgentId.value, true)
    default: return null
  }
})

const typeIconClass = computed(() => {
  switch (props.session.type) {
    case 'heartbeat': return 'text-event-heartbeat'
    case 'schedule': return 'text-event-schedule'
    case 'subagent': return 'text-event-subagent'
    default: return 'text-muted-foreground'
  }
})

const acpAgentId = computed(() => normalizeACPAgentID(props.session.metadata?.acp_agent_id))

function routeMeta(): Record<string, unknown> {
  return props.session.route_metadata ?? {}
}

function isGroupConversation(): boolean {
  const ct = (props.session.route_conversation_type ?? '').trim().toLowerCase()
  return ct === 'group' || ct === 'supergroup' || ct === 'channel'
}

const avatarUrl = computed<string | null>(() => {
  const meta = routeMeta()
  if (isGroupConversation()) {
    const convAvatar = (meta.conversation_avatar_url as string ?? '').trim()
    if (convAvatar) return convAvatar
  }
  const url = (meta.sender_avatar_url as string ?? '').trim()
  return url || null
})

const displayLabel = computed(() => {
  const meta = routeMeta()
  return (meta.conversation_name as string ?? '').trim()
    || (meta.sender_display_name as string ?? '').trim()
    || (meta.sender_username as string ?? '').trim()
    || ''
})

const avatarFallback = computed(() => {
  return displayLabel.value ? displayLabel.value.charAt(0).toUpperCase() : '?'
})

// The old two-line subLabel is folded into the native tooltip: channel handle
// for IM sessions, agent name for ACP sessions.
const hoverTitle = computed(() => {
  const title = props.session.title || t('chat.untitledSession')
  if (props.session.type === 'acp_agent') {
    return `${title} — ${acpAgentDisplayName(acpAgentId.value, t('chat.sessionTypeACPAgent'))}`
  }
  if (!isIMSession.value) return title
  const meta = routeMeta()
  const handle = (meta.conversation_handle as string ?? '').trim()
    || (meta.sender_username as string ?? '').trim()
    || displayLabel.value
  return handle ? `${title} — @${handle.replace(/^@/, '')}` : title
})

function formatTime(dateStr: string): string {
  try {
    const d = new Date(dateStr)
    if (Number.isNaN(d.getTime())) return ''
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const day = 86400000
    if (diff < day) return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
    if (diff < 7 * day) return d.toLocaleDateString(undefined, { weekday: 'short' })
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  } catch {
    return ''
  }
}
</script>
