<template>
  <FilesPane
    v-if="currentBotId && canWorkspaceRead"
    ref="filesPaneRef"
    :bot-id="currentBotId"
    :can-write="canWorkspaceWrite"
  />
  <div
    v-else
    class="flex items-center justify-center h-full text-xs text-muted-foreground"
  >
    {{ t('chat.selectBotHint') }}
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { hasBotPermission } from '@/utils/bot-permissions'
import FilesPane from './files-pane.vue'

const { t } = useI18n()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId, bots } = storeToRefs(chatStore)
const { pendingFilesPath } = storeToRefs(workspaceTabs)

const currentBot = computed(() =>
  bots.value.find(bot => bot.id === currentBotId.value) ?? null,
)
const currentPermissions = computed(() => currentBot.value?.current_user_permissions ?? [])
const canWorkspaceRead = computed(() => hasBotPermission(currentPermissions.value, 'workspace_read'))
const canWorkspaceWrite = computed(() => hasBotPermission(currentPermissions.value, 'workspace_write'))

const filesPaneRef = ref<InstanceType<typeof FilesPane> | null>(null)

// Consume one-shot navigation requests issued via store.openFilesAt().
watch([pendingFilesPath, filesPaneRef], async ([path, pane]) => {
  if (!path || !pane) return
  await nextTick()
  const target = workspaceTabs.consumePendingFilesPath()
  if (target) {
    pane.navigateTo(target)
  }
}, { immediate: true })
</script>
