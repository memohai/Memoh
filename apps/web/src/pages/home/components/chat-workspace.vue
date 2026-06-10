<template>
  <div class="flex flex-col flex-1 h-full min-w-0 bg-card">
    <DockviewVue
      class="h-full w-full"
      :components="panelComponents"
      :watermark-component="watermarkComponent"
      :default-tab-component="defaultTabComponent"
      :right-header-actions-component="rightHeaderActionsComponent"
      :theme="memohTheme"
      :disable-floating-groups="true"
      :get-tab-context-menu-items="getTabContextMenuItems"
      @ready="onReady"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, provide, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import {
  DockviewVue,
  type DockviewReadyEvent,
  type DockviewTheme,
  type GetTabContextMenuItemsParams,
  type ContextMenuItem,
  type VueComponent,
} from 'dockview-vue'
import 'dockview-vue/dist/styles/dockview.css'
import '@/styles/dockview-theme.css'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import PanelChat from './dockview/panel-chat.vue'
import PanelFile from './dockview/panel-file.vue'
import PanelTerminal from './dockview/panel-terminal.vue'
import PanelBrowser from './dockview/panel-browser.vue'
import PanelDisplay from './dockview/panel-display.vue'
import WorkspaceWatermark from './dockview/workspace-watermark.vue'
import WorkspaceTab from './dockview/workspace-tab.vue'
import GroupActions from './dockview/group-actions.vue'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { api } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId, sessionId, activeSession } = storeToRefs(chatStore)

// Panel components are looked up by name when panels are added or restored
// from a serialized layout. dockview-vue types frameworks components as
// DefineComponent<T> (single generic), which script-setup SFC types do not
// structurally match, hence the casts.
const panelComponents: Record<string, VueComponent> = {
  chat: PanelChat as unknown as VueComponent,
  file: PanelFile as unknown as VueComponent,
  terminal: PanelTerminal as unknown as VueComponent,
  browser: PanelBrowser as unknown as VueComponent,
  display: PanelDisplay as unknown as VueComponent,
}

const watermarkComponent = WorkspaceWatermark as unknown as VueComponent
const defaultTabComponent = WorkspaceTab as unknown as VueComponent
const rightHeaderActionsComponent = GroupActions as unknown as VueComponent

const memohTheme: DockviewTheme = {
  name: 'memoh',
  className: 'dockview-theme-memoh',
  gap: 0,
  dndTabIndicator: 'fill',
}

function getTabContextMenuItems({ panel, group }: GetTabContextMenuItemsParams): ContextMenuItem[] {
  return [
    {
      label: t('chat.tabMenu.close'),
      action: () => panel.api.close(),
    },
    {
      label: t('chat.tabMenu.closeOthers'),
      disabled: group.panels.length <= 1,
      action: () => {
        for (const other of [...group.panels]) {
          if (other.id !== panel.id) other.api.close()
        }
      },
    },
    {
      label: t('chat.tabMenu.closeAll'),
      action: () => {
        for (const other of [...group.panels]) {
          other.api.close()
        }
      },
    },
  ]
}

const chatPanelTitle = computed(() => {
  if (!sessionId.value) return t('chat.newSession')
  return (activeSession.value?.title ?? '').trim() || t('chat.untitledSession')
})

function onReady(event: DockviewReadyEvent) {
  store.registerApi(event.api)
  ensureChatPanel()
}

function ensureChatPanel() {
  if (!currentBotId.value || !api.value) return
  store.openChat(chatPanelTitle.value)
}

// Bot ready/switch: the store restores the persisted layout; make sure the
// chat panel exists afterwards (it may have been closed in a past session).
watch(currentBotId, (bid) => {
  if (bid && api.value) ensureChatPanel()
})

// Keep the singleton chat tab title in sync with the active session.
watch(chatPanelTitle, (title) => {
  store.setChatTitle(title)
}, { immediate: true })

const FILE_MANAGER_ROOT = '/data'

function normalizeFileManagerPath(path: string): string {
  const trimmedPath = path.trim()
  if (!trimmedPath) return FILE_MANAGER_ROOT
  if (trimmedPath === FILE_MANAGER_ROOT || trimmedPath.startsWith(`${FILE_MANAGER_ROOT}/`)) {
    return trimmedPath
  }
  if (trimmedPath === '/') return FILE_MANAGER_ROOT
  if (trimmedPath.startsWith('/')) {
    return `${FILE_MANAGER_ROOT}${trimmedPath}`
  }
  return `${FILE_MANAGER_ROOT}/${trimmedPath}`
}

provide(openInFileManagerKey, (path: string, isDir = false) => {
  const normalizedPath = normalizeFileManagerPath(path)
  if (isDir) {
    store.openFilesAt(normalizedPath)
  } else {
    store.openFile(normalizedPath)
  }
})

onBeforeUnmount(() => {
  store.releaseApi()
})
</script>
