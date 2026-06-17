<template>
  <div
    class="font-[400]"
    :class="inGroup ? '' : 'text-[0.90625rem]'"
  >
    <div
      v-if="expandable"
      role="button"
      tabindex="0"
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-px select-none"
      :class="rowClass"
      @click="toggleOpen"
      @keydown.enter.prevent="toggleOpen"
      @keydown.space.prevent="toggleOpen"
    >
      <span
        v-if="showActionLabel"
        class="shrink-0"
        :class="actionClass"
      >{{ renderedActionLabel }}</span>
      <button
        v-if="display.target && canOpenInFiles"
        class="truncate min-w-0 hover:underline cursor-pointer"
        :class="targetClass"
        :title="display.fullTarget || display.target"
        @click.stop="handleOpenInFiles"
      >
        {{ display.target }}
      </button>
      <span
        v-else-if="display.target"
        class="truncate min-w-0"
        :class="targetClass"
        :title="display.fullTarget || display.target"
      >{{ display.target }}</span>
      <span
        v-if="display.diffAdd"
        class="font-mono shrink-0 text-success-foreground"
      >+{{ display.diffAdd }}</span>
      <span
        v-if="display.diffRemove"
        class="font-mono shrink-0 text-destructive"
      >-{{ display.diffRemove }}</span>
      <span
        v-if="display.errorSuffix"
        class="font-mono shrink-0"
      >{{ display.errorSuffix }}</span>
      <span
        v-if="approvalLabel"
        class="font-mono shrink-0 text-xs text-warning-foreground"
      >{{ approvalLabel }}</span>
      <span
        v-if="userInputLabel"
        class="font-mono shrink-0 text-xs text-warning-foreground"
      >{{ userInputLabel }}</span>
      <ChevronRight
        v-if="!open"
        class="size-3.5 shrink-0 ml-0.5 opacity-60 group-hover:opacity-100"
      />
      <ChevronDown
        v-else
        class="size-3.5 shrink-0 ml-0.5 opacity-60 group-hover:opacity-100"
      />
    </div>

    <div
      v-else
      class="flex items-center gap-1.5 w-full py-px"
      :class="rowClass"
    >
      <span
        v-if="showActionLabel"
        class="shrink-0"
        :class="actionClass"
      >{{ renderedActionLabel }}</span>
      <button
        v-if="display.target && canOpenInFiles"
        class="truncate min-w-0 hover:underline cursor-pointer"
        :class="targetClass"
        :title="display.fullTarget || display.target"
        @click="handleOpenInFiles"
      >
        {{ display.target }}
      </button>
      <span
        v-else-if="display.target"
        class="truncate min-w-0"
        :class="targetClass"
        :title="display.fullTarget || display.target"
      >{{ display.target }}</span>
      <span
        v-if="display.diffAdd"
        class="font-mono shrink-0 text-success-foreground"
      >+{{ display.diffAdd }}</span>
      <span
        v-if="display.diffRemove"
        class="font-mono shrink-0 text-destructive"
      >-{{ display.diffRemove }}</span>
      <span
        v-if="display.errorSuffix"
        class="font-mono shrink-0"
      >{{ display.errorSuffix }}</span>
      <span
        v-if="approvalLabel"
        class="font-mono shrink-0 text-xs text-warning-foreground"
      >{{ approvalLabel }}</span>
      <span
        v-if="userInputLabel"
        class="font-mono shrink-0 text-xs text-warning-foreground"
      >{{ userInputLabel }}</span>
    </div>

    <CollapseSection
      v-if="expandable"
      :open="open && !isPending"
    >
      <div :class="detailClass">
        <component
          :is="display.detail"
          v-if="display.detail"
          :block="block"
        />
        <ToolCallDetailGeneric
          v-else
          :block="block"
        />
      </div>
    </CollapseSection>

    <div
      v-if="canRespondApproval"
      class="mt-1.5 ml-5 flex items-center gap-2"
    >
      <Button
        size="sm"
        class="bg-success hover:bg-success/90 text-success-foreground"
        @click="handleApproval('approve')"
      >
        {{ t('chat.tools.approve', 'Allow') }}
      </Button>
      <Button
        variant="outline"
        size="sm"
        class="hover:bg-destructive hover:text-destructive-foreground hover:border-destructive"
        @click="handleApproval('reject')"
      >
        {{ t('chat.tools.reject', 'Reject') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref, watch } from 'vue'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import { useChatStore } from '@/store/chat-list'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import {
  getToolDisplay,
  isDirPathTool,
  isFilePathTool,
} from './tool-call-registry'
import ToolCallDetailGeneric from './tool-call-detail-generic.vue'
import CollapseSection from './collapse-section.vue'
import { getCollapseOpen, setCollapseOpen, toolCollapseKey } from './process-collapse'
import { CHAT_PANE_SESSION_ID } from './chat-pane-context'

const props = defineProps<{ block: ToolCallBlock, inGroup?: boolean }>()
const { t } = useI18n()
const chatStore = useChatStore()
// The session this tool block is rendered in (a pinned chat tab may not be the
// active session); falls back to the active session when not provided.
const paneSessionId = inject(CHAT_PANE_SESSION_ID, undefined)

// Two detail styles:
//  - 'inline' = half-embedded key:value list (params), just indented, no card.
//  - 'card'  = output/diff/file content on a fill surface (card-in-card by color).
// Detail text sits one small step above body text so dense output stays readable.
const detailClass = computed(() => {
  if (display.value.detailVariant === 'inline') return 'mt-1 pl-3 font-[400]'
  return props.inGroup
    ? 'mt-1.5 rounded-sm bg-card px-2.5 py-2 font-[400]'
    : 'mt-1.5 rounded-md bg-muted px-3 py-2 font-[400]'
})

const openInFileManager = inject(openInFileManagerKey, undefined)

const display = computed(() => getToolDisplay(props.block))

// Persisted, user-driven toggle (survives the post-turn refetch/remount).
const collapseKey = computed(() => toolCollapseKey(props.block))
const open = ref(getCollapseOpen(collapseKey.value) || display.value.defaultOpen === true)
watch(collapseKey, (key) => {
  open.value = getCollapseOpen(key) || display.value.defaultOpen === true
})

const expandable = computed(
  () => Boolean(display.value.detail) || display.value.expandable === true,
)

const actionLabel = computed(() => {
  const key = `chat.tools.${display.value.actionKey}`
  return t(key, display.value.actionParams ?? {})
})

// A tool is "pending" while it is running and its input arguments have not
// streamed in yet (tool_call_input_start fires before the full call). In that
// window tools like write/edit hide their action label and have no target, so
// only a bare icon would show. We surface a placeholder label instead.
const isPending = computed(() => {
  if (props.block.done) return false
  const input = props.block.input
  return !(
    input
    && typeof input === 'object'
    && Object.keys(input as Record<string, unknown>).length > 0
  )
})

const showsBareIconWhenPending = computed(
  () => display.value.hideAction === true && !display.value.target,
)

const showPendingLabel = computed(
  () => isPending.value && showsBareIconWhenPending.value,
)

const pendingLabel = computed(
  () => t(`chat.tools.pending.${display.value.actionKey}`, t('chat.tools.pending.generic')),
)

const showActionLabel = computed(
  () => showPendingLabel.value || !display.value.hideAction,
)

const renderedActionLabel = computed(
  () => (showPendingLabel.value ? pendingLabel.value : actionLabel.value),
)

// Every row is gray at rest and animates to near-black (foreground) on hover:
// one neutral material, with color expressing interaction. Rest ink matches the
// process/thinking headers (--cop-title) so a lone tool row and a collapsed
// group read at the same weight.
const rowClass = computed(() => {
  if (display.value.isError) return 'text-destructive transition-colors duration-75'
  return 'text-cop-title hover:text-foreground transition-colors duration-75'
})

// Brief tools (e.g. send/memory) finish in <100ms. Showing the running
// shimmer for them flickers, so we only display it after a short delay.
const showRunning = ref(false)
let runningTimer: ReturnType<typeof setTimeout> | null = null
const RUNNING_SHIMMER_DELAY_MS = 250

function clearRunningTimer() {
  if (runningTimer !== null) {
    clearTimeout(runningTimer)
    runningTimer = null
  }
}

watch(
  () => props.block.done,
  (done) => {
    clearRunningTimer()
    if (done) {
      showRunning.value = false
      return
    }
    runningTimer = setTimeout(() => {
      showRunning.value = true
      runningTimer = null
    }, RUNNING_SHIMMER_DELAY_MS)
  },
  { immediate: true },
)

onBeforeUnmount(clearRunningTimer)

const targetClass = computed(() => {
  if (showRunning.value) return 'tool-shimmer-text'
  if (display.value.isError) return 'text-destructive'
  return '' // inherit the row's gray→black hover color
})

const actionClass = computed(() => {
  if (showPendingLabel.value) return 'tool-shimmer-text'
  if (showRunning.value && !display.value.target) return 'tool-shimmer-text'
  return ''
})

const approvalLabel = computed(() => {
  const approval = props.block.approval
  if (!approval?.approval_id) return ''
  const id = approval.short_id ? `#${approval.short_id}` : ''
  if (approval.status === 'pending') return `${id} ${t('chat.tools.pendingApproval', 'pending approval')}`.trim()
  return `${id} ${approval.status}`.trim()
})

const userInputLabel = computed(() => {
  const userInput = props.block.userInput
  if (!userInput?.user_input_id) return ''
  if (userInput.status === 'pending') return ''
  return userInputStatusLabel(userInput.status)
})

function userInputStatusLabel(status: string) {
  const normalized = status.trim().toLowerCase()
  switch (normalized) {
    case 'submitted':
      return t('chat.tools.userInputSubmitted', 'answered')
    case 'canceled':
      return t('chat.tools.userInputCanceled', 'canceled')
    case 'failed':
      return t('chat.tools.userInputFailed', 'failed')
    case 'expired':
      return t('chat.tools.userInputExpired', 'expired')
    default:
      return status
  }
}

const canRespondApproval = computed(() => {
  const approval = props.block.approval
  return Boolean(approval?.approval_id && approval.status === 'pending' && approval.can_approve !== false)
})

const filePath = computed(() => {
  if (!isFilePathTool(props.block.toolName)) return ''
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})

const canOpenInFiles = computed(
  () => Boolean(filePath.value) && Boolean(openInFileManager),
)

function toggleOpen() {
  open.value = !open.value
  setCollapseOpen(collapseKey.value, open.value)
}

function handleOpenInFiles() {
  if (!filePath.value || !openInFileManager) return
  openInFileManager(filePath.value, isDirPathTool(props.block.toolName))
}

function handleApproval(decision: 'approve' | 'reject') {
  const approval = props.block.approval
  if (!approval) return
  void chatStore.respondToolApproval(approval, decision, paneSessionId?.value ?? undefined)
}
</script>
