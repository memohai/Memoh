<template>
  <ChatDecisionPanel>
    <div class="px-3 py-1.5">
      <p
        data-slot="tool-approval-title"
        class="flex min-w-0 items-baseline gap-1.5 text-label font-medium text-foreground"
      >
        <span class="shrink-0">{{ approvalTitle }}</span>
        <span
          v-if="displayTarget"
          class="min-w-0 truncate font-normal"
          :title="display.fullTarget || display.target"
        >{{ displayTarget }}</span>
      </p>

      <Capsule
        v-if="codePreview || detailPreview"
        data-slot="tool-approval-preview"
        class="mt-2"
      >
        <div
          v-if="codePreview"
          class="max-h-48 overflow-auto"
        >
          <CodeBlock
            :code="codePreview.code"
            :lang="codePreview.lang"
            class="text-body leading-relaxed"
          />
        </div>
        <component
          :is="detailPreview"
          v-else
          :block="block"
        />
      </Capsule>

      <p
        class="text-body text-muted-foreground"
        :class="codePreview || detailPreview ? 'mt-2' : 'mt-0.5'"
      >
        <span v-if="approval.short_id">#{{ approval.short_id }} </span>{{ $t('chat.tools.pendingApproval') }}<span v-if="executionLocationLabel"> · {{ executionLocationLabel }}</span>
      </p>
    </div>

    <template #actions>
      <Button
        type="button"
        class="flex-1"
        @click="respond('approve')"
      >
        {{ $t('chat.tools.approve') }}
      </Button>
      <Button
        type="button"
        variant="secondary"
        class="flex-1"
        @click="respond('reject')"
      >
        {{ $t('chat.tools.reject') }}
      </Button>
    </template>
  </ChatDecisionPanel>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@felinic/ui'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import { useChatStore } from '@/store/chat-list'
import { useChatViewTarget } from '../composables/useChatViewContext'
import ChatDecisionPanel from './chat-decision-panel.vue'
import CodeBlock from './code-block.vue'
import Capsule from './tool-detail/capsule.vue'
import { getToolDisplay } from './tool-call-registry'

const props = defineProps<{
  block: ToolCallBlock
}>()

const { t } = useI18n()
const chatStore = useChatStore()
const chatViewTarget = useChatViewTarget()
const display = computed(() => getToolDisplay(props.block))
const approval = computed(() => props.block.approval!)
const input = computed(() => (
  props.block.input && typeof props.block.input === 'object'
    ? props.block.input as Record<string, unknown>
    : {}
))
const actionLabel = computed(() => {
  const key = `chat.tools.${display.value.actionKey}`
  return t(key, display.value.actionParams ?? {})
})
const approvalTitle = computed(() => {
  if (props.block.toolName === 'exec') return t('bots.toolApproval.toolNames.exec')
  if (props.block.toolName === 'write') return t('bots.toolApproval.toolNames.write')
  return actionLabel.value
})
const displayTarget = computed(() => (
  props.block.toolName === 'exec' ? '' : display.value.target
))
const codePreview = computed(() => {
  if (props.block.toolName === 'exec') {
    const command = input.value.command
    return typeof command === 'string' && command
      ? { code: command, lang: 'bash' }
      : null
  }
  return null
})
const detailPreview = computed(() => {
  const toolName = props.block.toolName
  if (toolName === 'write') {
    const content = input.value.content
    return typeof content === 'string' && content ? display.value.detail ?? null : null
  }
  if (toolName === 'edit') {
    const hasChanges = typeof input.value.old_text === 'string' || typeof input.value.new_text === 'string'
    return hasChanges ? display.value.detail ?? null : null
  }
  if (toolName === 'apply_patch') {
    const patch = input.value.patch
    return typeof patch === 'string' && patch ? display.value.detail ?? null : null
  }
  return null
})
const executionLocationLabel = computed(() => {
  const location = props.block.execution_location
  if (!location) return ''
  if (location.kind === 'native') return t('bots.remoteRuntime.nativeWorkspace')
  return location.name?.trim() || ''
})

function respond(decision: 'approve' | 'reject') {
  void chatStore.respondToolApproval(approval.value, decision, chatViewTarget.value)
}
</script>
