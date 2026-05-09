<template>
  <div class="text-sm leading-relaxed">
    <div
      v-if="expandable"
      role="button"
      tabindex="0"
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-0.5 select-none"
      :class="rowClass"
      @click="toggleOpen"
      @keydown.enter.prevent="toggleOpen"
      @keydown.space.prevent="toggleOpen"
    >
      <component
        :is="display.icon"
        class="size-3.5 shrink-0"
      />
      <span
        v-if="!display.hideAction"
        class="shrink-0"
        :class="actionClass"
      >{{ actionLabel }}</span>
      <button
        v-if="display.target && canOpenInFiles"
        class="font-mono truncate hover:underline cursor-pointer"
        :class="targetClass"
        :title="display.fullTarget || display.target"
        @click.stop="handleOpenInFiles"
      >
        {{ display.target }}
      </button>
      <span
        v-else-if="display.target"
        class="font-mono truncate"
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
      <ChevronRight
        v-if="!open"
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
      <ChevronDown
        v-else
        class="size-3.5 shrink-0 ml-auto opacity-60 group-hover:opacity-100"
      />
    </div>

    <div
      v-else
      class="flex items-center gap-1.5 w-full py-0.5"
      :class="rowClass"
    >
      <component
        :is="display.icon"
        class="size-3.5 shrink-0"
      />
      <span
        v-if="!display.hideAction"
        class="shrink-0"
        :class="actionClass"
      >{{ actionLabel }}</span>
      <button
        v-if="display.target && canOpenInFiles"
        class="font-mono truncate hover:underline cursor-pointer"
        :class="targetClass"
        :title="display.fullTarget || display.target"
        @click="handleOpenInFiles"
      >
        {{ display.target }}
      </button>
      <span
        v-else-if="display.target"
        class="font-mono truncate"
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
    </div>

    <div
      v-if="expandable && open"
      class="mt-1 ml-5 py-1 space-y-1.5"
    >
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

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()
const chatStore = useChatStore()

const openInFileManager = inject(openInFileManagerKey, undefined)

const display = computed(() => getToolDisplay(props.block))

const open = ref(getToolDisplay(props.block).defaultOpen === true)

const expandable = computed(
  () => Boolean(display.value.detail) || display.value.expandable === true,
)

const actionLabel = computed(() => {
  const key = `chat.tools.${display.value.actionKey}`
  return t(key, display.value.actionParams ?? {})
})

const rowClass = computed(() => {
  if (!expandable.value) {
    return display.value.isError ? 'text-destructive' : 'text-muted-foreground'
  }
  return display.value.isError
    ? 'text-destructive hover:text-destructive/90'
    : 'text-muted-foreground hover:text-foreground'
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
  return 'text-foreground/80'
})

const actionClass = computed(() => {
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
}

function handleOpenInFiles() {
  if (!filePath.value || !openInFileManager) return
  openInFileManager(filePath.value, isDirPathTool(props.block.toolName))
}

function handleApproval(decision: 'approve' | 'reject') {
  const approval = props.block.approval
  if (!approval) return
  void chatStore.respondToolApproval(approval, decision)
}
</script>
