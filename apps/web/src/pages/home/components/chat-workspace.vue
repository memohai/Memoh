<template>
  <div
    ref="rootEl"
    class="flex flex-col flex-1 h-full min-w-0 bg-card"
  >
    <DockviewVue
      class="h-full w-full"
      :components="panelComponents"
      :watermark-component="watermarkComponent"
      :default-tab-component="defaultTabComponent"
      :right-header-actions-component="rightHeaderActionsComponent"
      :theme="memohTheme"
      :disable-floating-groups="true"
      :disable-tabs-overflow-list="true"
      :disable-auto-resizing="true"
      :get-tab-context-menu-items="getTabContextMenuItems"
      @ready="onReady"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
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

// dockview's own auto-resize is disabled at construction (:disable-auto-resizing);
// we drive layout from this ResizeObserver instead. The dock is a flex-1 sibling
// of the sidebar (push/pull model), so when the sidebar slides out/in this root's
// width animates frame-by-frame; the observer relays out on each of those frames,
// keeping panels matched to the container the whole way. The right edge is pinned
// (viewport edge), so the right-side actions ("+") never move while the width
// changes — no snap.
// (NOTE: dockview 6.6.1 ignores updateOptions({ disableAutoResizing }) at runtime
// — a guard checks the wrong key — so toggling the prop reactively does nothing;
// owning the observer is the reliable route.)
const rootEl = ref<HTMLElement | null>(null)
let resizeObserver: ResizeObserver | null = null

function applyLayout() {
  const dock = api.value
  const el = rootEl.value
  if (!dock || !el) return
  dock.layout(el.clientWidth, el.clientHeight)
}

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
  // Size the grid before adding panels (auto-resize is off, so without this the
  // grid would be 0×0 and the first panel would lay out empty).
  if (rootEl.value) event.api.layout(rootEl.value.clientWidth, rootEl.value.clientHeight)
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

onMounted(() => {
  resizeObserver = new ResizeObserver(() => applyLayout())
  if (rootEl.value) resizeObserver.observe(rootEl.value)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  store.releaseApi()
})
</script>
