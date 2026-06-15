<template>
  <div class="flex h-full w-full items-center gap-0.5 pr-2">
    <!-- Unified "+": always opens a menu (never a direct terminal shortcut). New
         panels and split actions live in the dropdown; sits first so it hugs the
         last tab. -->
    <DropdownMenu v-if="hasAnyAction">
      <DropdownMenuTrigger as-child>
        <Button
          variant="ghost"
          size="icon-sm"
          class="size-7 shrink-0 rounded-full p-0 text-muted-foreground hover:text-foreground data-[state=open]:text-foreground"
          :title="t('chat.tabBarToolkit.openMenu')"
          :aria-label="t('chat.tabBarToolkit.openMenu')"
        >
          <Plus class="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuItem
          v-if="canWorkspaceExec"
          @select="store.openTerminal(props.params.group.id)"
        >
          <Terminal class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.newTerminal') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="canSplitExtras"
          @select="store.openBrowser(props.params.group.id)"
        >
          <Globe class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.openBrowser') }}
        </DropdownMenuItem>
        <DropdownMenuItem
          v-if="canSplitExtras"
          @select="store.openDisplay(props.params.group.id)"
        >
          <Monitor class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.openDesktop') }}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem @select="store.splitGroup(props.params.group.id, 'right')">
          <Columns2 class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.splitRight') }}
        </DropdownMenuItem>
        <DropdownMenuItem @select="store.splitGroup(props.params.group.id, 'below')">
          <Rows2 class="mr-2 size-3.5" />
          {{ t('chat.tabBarToolkit.splitDown') }}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <!-- Draggable spacer: the window drag handle AND what pushes Preview to the
         strip's far right. When there's no Preview the spacer simply extends, so
         the "+" cluster never shifts (no reserved empty slot like before). -->
    <div class="h-full flex-1 [-webkit-app-region:drag]" />
    <!-- Open Preview to the Side: only when this group's active tab is a
         markdown/html file (VS Code's editor-title preview action). Pinned to the
         far right rather than beside the tabs so a plain tab leaves no gap. -->
    <Button
      v-if="previewPath"
      variant="ghost"
      class="size-7 shrink-0 rounded-full p-0 text-muted-foreground hover:text-foreground"
      :title="t('chat.openPreviewToSide')"
      :aria-label="t('chat.openPreviewToSide')"
      @click="openPreviewToSide"
    >
      <Columns2 class="size-3.5" />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { Columns2, Globe, Monitor, Plus, Rows2, Terminal } from 'lucide-vue-next'
import {
  Button,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
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

// Browser / desktop are container-only and need manage permission; a local
// (trusted-host) workspace exposes neither, so its menu is terminal + split only.
const canSplitExtras = computed(() => canManage.value && !isLocalWorkspace.value)

const hasAnyAction = computed(() =>
  canWorkspaceExec.value || canSplitExtras.value || canWorkspaceRead.value,
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
  store.openPreview(path, t('chat.previewTab', { name }), props.params.group.id)
}
</script>
