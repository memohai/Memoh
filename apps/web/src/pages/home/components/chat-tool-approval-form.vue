<template>
  <ChatDecisionPanel>
    <div class="px-3 py-1.5">
      <p class="flex min-w-0 items-baseline gap-1.5 text-label font-medium text-foreground">
        <span class="shrink-0">{{ actionLabel }}</span>
        <span
          v-if="display.target"
          class="min-w-0 truncate font-normal"
          :title="display.fullTarget || display.target"
        >{{ display.target }}</span>
      </p>
      <p class="mt-0.5 text-body text-muted-foreground">
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
import { getToolDisplay } from './tool-call-registry'

const props = defineProps<{
  block: ToolCallBlock
}>()

const { t } = useI18n()
const chatStore = useChatStore()
const chatViewTarget = useChatViewTarget()
const display = computed(() => getToolDisplay(props.block))
const approval = computed(() => props.block.approval!)
const actionLabel = computed(() => {
  const key = `chat.tools.${display.value.actionKey}`
  return t(key, display.value.actionParams ?? {})
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
