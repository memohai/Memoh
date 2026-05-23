<template>
  <div
    class="flex shrink-0 h-full relative"
    :style="{ width: `${effectiveSidebarWidth}px` }"
  >
    <div
      class="flex flex-col h-full flex-1 min-w-0 bg-sidebar border-r border-border"
      :class="{ 'items-center': collapsed }"
    >
      <div
        class="flex shrink-0 border-b border-border bg-sidebar/60 [-webkit-app-region:drag]"
        :class="collapsed ? 'h-full w-full flex-col items-center py-1.5' : 'h-12 items-center'"
      >
        <div
          class="flex min-w-0 max-w-full gap-1 overflow-x-auto overflow-y-hidden px-1.5 py-1 [-webkit-app-region:no-drag]"
          :class="collapsed ? 'flex-col items-center overflow-visible' : 'items-center'"
        >
          <button
            v-for="tab in activityTabs"
            :key="tab.id"
            type="button"
            class="relative flex items-center justify-center size-8 shrink-0 rounded-md transition-colors before:absolute before:h-0.5 before:left-1.5 before:right-1.5 before:top-0 before:rounded-full"
            :class="activeTab === tab.id
              ? 'bg-sidebar-accent text-sidebar-accent-foreground before:bg-sidebar-primary'
              : 'text-muted-foreground hover:bg-sidebar-accent/40 hover:text-foreground before:bg-transparent'"
            :title="tab.label"
            :aria-label="tab.label"
            :aria-current="activeTab === tab.id ? 'page' : undefined"
            @click="handleTabClick(tab.id)"
          >
            <component
              :is="tab.icon"
              class="size-4"
            />
          </button>
        </div>

        <button
          type="button"
          class="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent/40 hover:text-foreground [-webkit-app-region:no-drag]"
          :class="collapsed ? 'mt-auto' : 'ml-auto mr-1.5'"
          :title="collapsed ? t('chat.expandSidebar') : t('chat.collapseSidebar')"
          :aria-label="collapsed ? t('chat.expandSidebar') : t('chat.collapseSidebar')"
          @click="toggleCollapsed"
        >
          <PanelLeftOpen
            v-if="collapsed"
            class="size-4"
          />
          <PanelLeftClose
            v-else
            class="size-4"
          />
        </button>
      </div>

      <div
        v-if="!collapsed"
        class="flex-1 min-h-0 relative"
      >
        <div
          v-show="activeTab === 'sessions'"
          class="absolute inset-0"
        >
          <ChatSidebarSessions />
        </div>
        <div
          v-show="activeTab === 'files'"
          class="absolute inset-0"
        >
          <ChatSidebarFiles
            v-if="currentBotId"
            ref="filesPanelRef"
            :bot-id="currentBotId"
          />
          <div
            v-else
            class="flex items-center justify-center h-full text-xs text-muted-foreground"
          >
            {{ t('chat.selectBotHint') }}
          </div>
        </div>
        <div
          v-show="activeTab === 'skills'"
          class="absolute inset-0"
        >
          <ChatSidebarSkills
            v-if="currentBotId"
            :bot-id="currentBotId"
          />
          <div
            v-else
            class="flex items-center justify-center h-full text-xs text-muted-foreground"
          >
            {{ t('chat.selectBotHint') }}
          </div>
        </div>
        <div
          v-show="activeTab === 'mcp'"
          class="absolute inset-0"
        >
          <ChatSidebarMcp
            v-if="currentBotId"
            :bot-id="currentBotId"
          />
          <div
            v-else
            class="flex items-center justify-center h-full text-xs text-muted-foreground"
          >
            {{ t('chat.selectBotHint') }}
          </div>
        </div>
        <div
          v-show="activeTab === 'schedule'"
          class="absolute inset-0"
        >
          <ChatSidebarSchedule
            v-if="currentBotId"
            :bot-id="currentBotId"
          />
          <div
            v-else
            class="flex items-center justify-center h-full text-xs text-muted-foreground"
          >
            {{ t('chat.selectBotHint') }}
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="!collapsed"
      class="absolute top-0 right-0 w-1 h-full cursor-col-resize z-10 group"
      @mousedown="onResizeStart"
    >
      <div
        class="w-full h-full transition-colors group-hover:bg-primary/20"
        :class="{ 'bg-primary/30': isResizing }"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, nextTick, onMounted, type Component } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { MessageSquare, Folder, Sparkles, Plug, CalendarClock, PanelLeftClose, PanelLeftOpen } from 'lucide-vue-next'
import { useChatStore } from '@/store/chat-list'
import ChatSidebarSessions from './chat-sidebar-sessions.vue'
import ChatSidebarFiles from './chat-sidebar-files.vue'
import ChatSidebarSkills from './chat-sidebar-skills.vue'
import ChatSidebarMcp from './chat-sidebar-mcp.vue'
import ChatSidebarSchedule from './chat-sidebar-schedule.vue'

type ActivityTabId = 'sessions' | 'files' | 'skills' | 'mcp' | 'schedule'

interface ActivityTab {
  id: ActivityTabId
  label: string
  icon: Component
}

const { t } = useI18n()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const activityTabs = computed<ActivityTab[]>(() => [
  { id: 'sessions', label: t('chat.activityTabSessions'), icon: MessageSquare },
  { id: 'files', label: t('chat.activityTabFiles'), icon: Folder },
  { id: 'skills', label: t('chat.activityTabSkills'), icon: Sparkles },
  { id: 'mcp', label: t('chat.activityTabMcp'), icon: Plug },
  { id: 'schedule', label: t('chat.activityTabSchedule'), icon: CalendarClock },
])

const activeTab = useLocalStorage<ActivityTabId>('chat-sidebar-active-tab', 'sessions')
const collapsed = useLocalStorage('chat-sidebar-collapsed', false)

if (!activityTabs.value.some((t) => t.id === activeTab.value)) {
  activeTab.value = 'sessions'
}

const filesPanelRef = ref<InstanceType<typeof ChatSidebarFiles> | null>(null)

const MIN_WIDTH = 200
const MAX_WIDTH = 520
const DEFAULT_WIDTH = 335
const COLLAPSED_WIDTH = 48

const sidebarWidth = useLocalStorage('chat-sidebar-width', DEFAULT_WIDTH)
const effectiveSidebarWidth = computed(() => collapsed.value ? COLLAPSED_WIDTH : sidebarWidth.value)
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

function openFilesAt(path: string) {
  collapsed.value = false
  activeTab.value = 'files'
  void nextTick(() => {
    filesPanelRef.value?.navigateTo(path)
  })
}

function setActiveTab(tab: ActivityTabId) {
  activeTab.value = tab
  collapsed.value = false
}

function handleTabClick(tab: ActivityTabId) {
  setActiveTab(tab)
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value
}

function handleExternalTab(event: Event) {
  const detail = (event as CustomEvent<{ tab?: ActivityTabId }>).detail
  const tab = detail?.tab
  if (!tab || !activityTabs.value.some((item) => item.id === tab)) return
  setActiveTab(tab)
}

onMounted(() => {
  window.addEventListener('memoh:chat-sidebar-tab', handleExternalTab)
})

onBeforeUnmount(() => {
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  window.removeEventListener('memoh:chat-sidebar-tab', handleExternalTab)
})

defineExpose({
  openFilesAt,
  setActiveTab,
})
</script>
