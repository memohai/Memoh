<template>
  <aside class="flex h-full shrink-0 flex-col border-r border-sidebar-border bg-sidebar">
    <header
      class="flex h-9 shrink-0 items-center gap-1 border-b border-sidebar-border bg-sidebar pr-1 [-webkit-app-region:drag]"
      :class="macTrafficReserve ? 'pl-[76px]' : 'pl-2'"
      :style="{ width: `${headerWidth}px` }"
    >
      <div class="min-w-0 flex-1 [-webkit-app-region:no-drag]">
        <BotSwitcher variant="row" />
      </div>
    </header>

    <div class="flex min-h-0 flex-1">
      <div class="flex w-11 shrink-0 flex-col items-center gap-1 border-r border-sidebar-border bg-sidebar py-2">
        <Button
          v-for="view in availableViews"
          :key="view.id"
          variant="ghost"
          size="icon-sm"
          class="shrink-0 text-muted-foreground hover:text-foreground"
          :class="sidebarOpen && sidebarView === view.id && 'bg-sidebar-accent text-foreground!'"
          :title="view.label"
          :aria-label="view.label"
          :aria-pressed="sidebarOpen && sidebarView === view.id"
          @click="store.selectSidebarView(view.id)"
        >
          <component
            :is="view.icon"
            :stroke-width="1.75"
            class="size-4"
          />
        </Button>

        <div class="flex-1" />

        <Button
          variant="ghost"
          size="icon-sm"
          class="shrink-0 text-muted-foreground hover:text-foreground"
          :class="isSettingsActive && 'bg-sidebar-accent text-foreground!'"
          :title="t('sidebar.settings')"
          :aria-label="t('sidebar.settings')"
          @click="router.push('/settings')"
        >
          <Settings
            :stroke-width="1.75"
            class="size-4"
          />
        </Button>
      </div>

      <div
        v-if="sidebarOpen"
        class="relative flex h-full shrink-0 flex-col bg-sidebar"
        :style="{ width: `${sidebarWidth}px` }"
      >
        <div class="min-h-0 flex-1">
          <PanelSessions
            v-show="sidebarView === 'sessions'"
            class="h-full"
          />
          <PanelFiles
            v-if="canWorkspaceRead"
            v-show="sidebarView === 'files'"
            class="h-full"
          />
          <PanelSearch
            v-if="sidebarView === 'search'"
            class="h-full"
          />
        </div>

        <div
          class="group absolute right-0 top-0 z-10 h-full w-1 cursor-col-resize"
          @mousedown="onResizeStart"
        >
          <div
            class="h-full w-full transition-colors group-hover:bg-border"
            :class="{ 'bg-ring': isResizing }"
          />
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, type Component } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { Folder, MessageSquare, Search, Settings } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore, type SidebarView } from '@/store/workspace-tabs'
import { hasBotPermission } from '@/utils/bot-permissions'
import BotSwitcher from './bot-switcher.vue'
import PanelSessions from './panel-sessions.vue'
import PanelFiles from './panel-files.vue'
import PanelSearch from './panel-search.vue'

const props = defineProps<{
  macTrafficReserve?: boolean
}>()

interface ActivityView {
  id: SidebarView
  label: string
  icon: Component
}

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { sidebarView, sidebarOpen, sidebarWidth } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId, bots } = storeToRefs(chatStore)

const currentBot = computed(() =>
  bots.value.find(bot => bot.id === currentBotId.value) ?? null,
)
const canWorkspaceRead = computed(() =>
  hasBotPermission(currentBot.value?.current_user_permissions, 'workspace_read'),
)

const availableViews = computed<ActivityView[]>(() => {
  const views: ActivityView[] = [
    { id: 'sessions', label: t('chat.activityBar.sessions'), icon: MessageSquare },
  ]
  if (canWorkspaceRead.value) {
    views.push({ id: 'files', label: t('chat.activityBar.files'), icon: Folder })
  }
  views.push({ id: 'search', label: t('chat.activityBar.search'), icon: Search })
  return views
})

// If the persisted view becomes unavailable (e.g. permission lost), fall back.
watch(availableViews, (views) => {
  if (!views.some((view) => view.id === sidebarView.value)) {
    sidebarView.value = 'sessions'
  }
}, { immediate: true })

const isSettingsActive = computed(() => route.path.startsWith('/settings'))

const MIN_WIDTH = 200
const MAX_WIDTH = 480
const RAIL_WIDTH = 44
const MAC_TRAFFIC_HEADER_MIN_WIDTH = 176

const headerWidth = computed(() => {
  const sidebarContentWidth = RAIL_WIDTH + (sidebarOpen.value ? sidebarWidth.value : 0)
  return props.macTrafficReserve
    ? Math.max(MAC_TRAFFIC_HEADER_MIN_WIDTH, sidebarContentWidth)
    : sidebarContentWidth
})

const isResizing = ref(false)

function onResizeStart(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  const startX = e.clientX
  const startWidth = sidebarWidth.value

  function onMouseMove(ev: MouseEvent) {
    const delta = ev.clientX - startX
    sidebarWidth.value = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth + delta))
  }

  function onMouseUp() {
    isResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }

  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

onBeforeUnmount(() => {
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})
</script>
