<template>
  <div class="flex h-full items-center gap-0.5 px-2">
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
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { Globe, Monitor, Plus, TerminalSquare } from 'lucide-vue-next'
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
import { isLocalWorkspaceBot } from '@/utils/bot-workspace'
import { hasBotPermission } from '@/utils/bot-permissions'

defineProps<{
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

const hasAnyAction = computed(() =>
  canWorkspaceExec.value || (canManage.value && !isLocalWorkspace.value),
)
</script>
