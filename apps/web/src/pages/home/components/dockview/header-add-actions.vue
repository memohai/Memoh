<template>
  <div class="flex h-full items-center">
    <!-- Terminal group: direct "+" to spawn another session (no dropdown). -->
    <Button
      v-if="isTerminalGroup && canWorkspaceExec"
      variant="ghost"
      size="icon-sm"
      class="size-[1.6875rem] shrink-0 rounded-sm p-0 text-muted-foreground/70 hover:bg-[color:var(--sidebar-hover)] hover:text-foreground"
      :title="t('chat.tabBarToolkit.newTerminal')"
      :aria-label="t('chat.tabBarToolkit.newTerminal')"
      @click="store.openTerminalInPanel(props.params.group.id)"
    >
      <Plus class="size-3.5" />
    </Button>
    <!-- Editor groups: unified "+" menu for new panels and splits. -->
    <DropdownMenu v-else-if="hasAnyAction">
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
          @select="store.openTerminalInPanel(props.params.group.id)"
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
        <template v-if="canSplit">
          <DropdownMenuSeparator v-if="canWorkspaceExec || canSplitExtras" />
          <DropdownMenuItem @select="store.splitGroup(props.params.group.id, 'right')">
            <Columns2 class="mr-2 size-3.5" />
            {{ t('chat.tabBarToolkit.splitRight') }}
          </DropdownMenuItem>
          <DropdownMenuItem @select="store.splitGroup(props.params.group.id, 'below')">
            <Rows2 class="mr-2 size-3.5" />
            {{ t('chat.tabBarToolkit.splitDown') }}
          </DropdownMenuItem>
        </template>
      </DropdownMenuContent>
    </DropdownMenu>
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
const canWorkspaceExec = computed(() => hasBotPermission(currentPermissions.value, 'workspace_exec'))
const canManage = computed(() => hasBotPermission(currentPermissions.value, 'manage'))
const isLocalWorkspace = computed(() => isLocalWorkspaceBot(currentBot.value?.metadata))

const isTerminalGroup = computed(() => {
  const panels = props.params.group.panels
  return panels.length > 0 && panels.every(p => p.id.startsWith('terminal:'))
})

const activePanelId = ref<string | null>(props.params.group.activePanel?.id ?? null)
const activePanelSub = props.params.api.onDidActivePanelChange(() => {
  activePanelId.value = props.params.group.activePanel?.id ?? null
})

onBeforeUnmount(() => activePanelSub.dispose())

// Browser / desktop are container-only and need manage permission; a local
// (trusted-host) workspace exposes neither, so its menu is terminal + split only.
const canSplitExtras = computed(() => canManage.value && !isLocalWorkspace.value)

// Splitting duplicates the active tab into a second pane. Chat keeps its stable
// primary id for routing/title sync, but split copies use generated ids.
const canSplit = computed(() => {
  return !!activePanelId.value
})

const hasAnyAction = computed(() =>
  canWorkspaceExec.value || canSplitExtras.value || canSplit.value,
)
</script>
