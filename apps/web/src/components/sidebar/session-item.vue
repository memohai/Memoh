<template>
  <div
    role="button"
    :tabindex="disabled ? -1 : 0"
    :aria-disabled="disabled ? 'true' : undefined"
    class="group relative flex items-center min-h-[2.125rem] w-full rounded-[9px] px-[11px] text-left transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    :class="[
      isActive ? 'bg-sidebar-accent' : 'hover:bg-[color:var(--sidebar-hover)]',
      disabled ? 'pointer-events-none opacity-40' : '',
    ]"
    :title="hoverTitle"
    @click="handleSelect"
    @keydown.enter.prevent="handleSelect"
    @keydown.space.prevent="handleSelect"
  >
    <!-- Session rows are text-only: the title carries the whole row. No leading
         type glyph and no trailing timestamp — the list is a single
         human-conversation stream (chat/discuss/agent) ordered by recency, so
         per-row type icons and clock stamps only added noise. The title runs to
         the trailing edge; the actions button overlays its tail on hover. -->
    <!-- Title runs are split CJK vs Latin so the title renders with the SAME
         per-script treatment as the chat body (.sidebar-cjk / .sidebar-latin reuse
         the --chat-*-body weight + Latin size/tracking). The only thing dropped vs
         the body is streaming — a title is a static one-line label. -->
    <span class="flex-1 min-w-0 truncate text-control text-foreground dark:text-[color:oklch(0.92_0_0)]"><span
      v-for="(run, i) in titleRuns"
      :key="i"
      :class="run.script === 'cjk' ? 'sidebar-cjk' : 'sidebar-latin'"
    >{{ run.text }}</span></span>

    <!-- Trailing slot: spinner and the actions button share one 24px right
         slot so they sit at the same center and never both show at once. The
         spinner reserves the slot in flow (keeps the title tail clear while
         streaming and stops the title from jumping on hover); the actions
         button is the same size-6 box anchored to the same right edge, and on
         row hover or while the menu is open it fades in as the spinner fades
         out — streaming + hover no longer stacks the two icons. -->
    <div class="relative ml-1.5 flex h-6 shrink-0 items-center justify-end">
      <div
        v-if="streaming"
        class="flex h-6 w-6 items-center justify-center transition-opacity duration-150 group-hover:opacity-0"
        :class="menuOpen ? 'opacity-0' : 'opacity-100'"
      >
        <LoaderCircle
          class="size-3 animate-spin text-muted-foreground"
          :aria-label="t('chat.sessionStreaming')"
        />
      </div>

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
            {{ t('common.rename') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            variant="destructive"
            @select="$emit('delete', session)"
          >
            <Trash2 class="mr-2 size-3.5" />
            {{ t('common.delete') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { LoaderCircle, MoreHorizontal, Pencil, Trash2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { SessionSummary } from '@/composables/api/useChat'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import { acpAgentDisplayName, normalizeACPAgentID } from '@/utils/acp'
import { splitScriptRuns } from '@/utils/script-runs'

const props = defineProps<{
  session: SessionSummary
  isActive: boolean
  streaming?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  select: [session: SessionSummary]
  rename: [session: SessionSummary]
  delete: [session: SessionSummary]
}>()

const { t } = useI18n()

const menuOpen = ref(false)

function handleSelect() {
  if (props.disabled) return
  emit('select', props.session)
}

const titleRuns = computed(() =>
  splitScriptRuns((props.session.title ?? '').trim() || t('chat.untitledSession')),
)

const WEB_CHANNELS = new Set(['local', ''])

const isIMSession = computed(() => {
  const ct = (props.session.channel_type ?? '').trim().toLowerCase()
  return ct !== '' && !WEB_CHANNELS.has(ct)
})

const acpAgentId = computed(() => normalizeACPAgentID(props.session.metadata?.acp_agent_id))

function routeMeta(): Record<string, unknown> {
  return props.session.route_metadata ?? {}
}

const displayLabel = computed(() => {
  const meta = routeMeta()
  return (meta.conversation_name as string ?? '').trim()
    || (meta.sender_display_name as string ?? '').trim()
    || (meta.sender_username as string ?? '').trim()
    || ''
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
</script>
