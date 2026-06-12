<template>
  <div class="flex h-full items-center gap-0.5 px-2">
    <!-- Open Preview to the Side: shown only when this group's active tab is a
         markdown/html file (VS Code's editor-title preview action). -->
    <Button
      v-if="previewPath"
      variant="ghost"
      class="size-7 p-0 text-muted-foreground hover:text-foreground"
      :title="t('chat.openPreviewToSide')"
      :aria-label="t('chat.openPreviewToSide')"
      @click="openPreviewToSide"
    >
      <Columns2 class="size-3.5" />
    </Button>
    <DropdownMenu v-if="hasAnyAction">
      <DropdownMenuTrigger as-child>
        <Button
          variant="ghost"
          class="size-7 p-0 text-muted-foreground hover:text-foreground"
          :title="t('chat.tabBarToolkit.menu')"
          :aria-label="t('chat.tabBarToolkit.menu')"
        >
          <Plus class="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          v-if="canWorkspaceExec"
          @select="store.openTerminal()"
        >
          <TerminalSquare class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.newTerminal') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="canManage && !isLocalWorkspace"
          @select="store.openBrowser()"
        >
          <Globe class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.openBrowser') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="canManage && !isLocalWorkspace"
          @select="store.openDisplay()"
        >
          <Monitor class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.openDisplay') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { Columns2, Globe, Monitor, Plus, TerminalSquare } from 'lucide-vue-next'
import {
  Button,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import type { DockviewApi, DockviewGroupPanelApi, IDockviewGroupPanel } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { isHtmlFile, isMarkdownFile } from '@/components/file-manager/utils'
import { isLocalWorkspaceBot } from '@/utils/bot-workspace'
import { hasBotPermission } from '@/utils/bot-permissions'

const props = defineProps<{
  params: {
    api: DockviewGroupPanelApi
    containerApi: DockviewApi
    group: IDockviewGroupPanel
  }
}>()

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const chatStore = useChatStore()
const { currentBotId, bots } = storeToRefs(chatStore)

const currentBot = computed(() =>
  bots.value.find(bot => bot.id === currentBotId.value) ?? null,
)
const currentPermissions = computed(() => currentBot.value?.current_user_permissions ?? [])
const canWorkspaceRead = computed(() => hasBotPermission(currentPermissions.value, 'workspace_read'))
const canWorkspaceExec = computed(() => hasBotPermission(currentPermissions.value, 'workspace_exec'))
const canManage = computed(() => hasBotPermission(currentPermissions.value, 'manage'))
const isLocalWorkspace = computed(() => isLocalWorkspaceBot(currentBot.value?.metadata))

const hasAnyAction = computed(() =>
  canWorkspaceExec.value || (canManage.value && !isLocalWorkspace.value),
)

// Track THIS group's active tab (not the globally active panel) so the preview
// action only appears in the header of the group currently showing a
// previewable file — correct for split layouts.
const activePanelId = ref<string | null>(props.params.group.activePanel?.id ?? null)
const activePanelSub = props.params.api.onDidActivePanelChange(() => {
  activePanelId.value = props.params.group.activePanel?.id ?? null
})
onBeforeUnmount(() => activePanelSub.dispose())

const previewPath = computed(() => {
  if (!canWorkspaceRead.value) return null
  const id = activePanelId.value
  if (!id || !id.startsWith('file:')) return null
  const path = id.slice('file:'.length)
  const name = path.slice(path.lastIndexOf('/') + 1)
  return isMarkdownFile(name) || isHtmlFile(name) ? path : null
})

function openPreviewToSide() {
  const path = previewPath.value
  if (!path) return
  const name = path.slice(path.lastIndexOf('/') + 1)
  store.openPreview(path, t('chat.previewTab', { name }))
}
</script>
