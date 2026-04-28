<template>
  <div class="flex items-center h-12 shrink-0 border-b border-border bg-background px-1.5 pt-2 pb-1 gap-1 overflow-x-auto overflow-y-hidden whitespace-nowrap">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      type="button"
      class="group inline-flex items-center gap-1.5 h-8 shrink-0 rounded-md px-2.5 text-xs transition-colors max-w-[200px]"
      :class="tab.id === activeId
        ? 'bg-sidebar-accent text-sidebar-accent-foreground'
        : 'text-muted-foreground hover:bg-sidebar-accent/40 hover:text-foreground'"
      :title="resolveTitle(tab)"
      @click="store.setActive(tab.id)"
    >
      <component
        :is="tabIcon(tab)"
        class="size-3.5 shrink-0"
      />
      <span class="truncate">
        {{ resolveTitle(tab) }}
      </span>
      <span
        role="button"
        tabindex="0"
        class="inline-flex items-center justify-center size-4 rounded-sm shrink-0 opacity-0 group-hover:opacity-100 hover:bg-muted-foreground/20 transition-opacity"
        :class="{ 'opacity-100': tab.id === activeId }"
        :aria-label="t('chat.tabClose')"
        @click.stop="store.closeTab(tab.id)"
        @keydown.enter.prevent.stop="store.closeTab(tab.id)"
        @keydown.space.prevent.stop="store.closeTab(tab.id)"
      >
        <X class="size-3" />
      </span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { File as FileIcon, MessageSquare, X } from 'lucide-vue-next'
import { useWorkspaceTabsStore, type WorkspaceTab } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { tabs, activeId } = storeToRefs(store)

const chatStore = useChatStore()
const { sessions } = storeToRefs(chatStore)

const sessionTitleById = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  for (const s of sessions.value) {
    out[s.id] = (s.title ?? '').trim() || t('chat.untitledSession')
  }
  return out
})

function tabIcon(tab: WorkspaceTab): Component {
  return tab.type === 'chat' ? MessageSquare : FileIcon
}

function resolveTitle(tab: WorkspaceTab): string {
  if (tab.type === 'chat') {
    return sessionTitleById.value[tab.sessionId] || tab.title || t('chat.untitledSession')
  }
  return tab.title || tab.filePath
}
</script>
