<template>
  <div class="flex items-center h-12 shrink-0 border-b border-border bg-background gap-1">
    <div
      ref="tabsContainerRef"
      class="flex items-center flex-1 min-w-0 px-1.5 pt-1 pb-1 gap-1 overflow-x-auto overflow-y-hidden whitespace-nowrap"
    >
      <button
        v-for="tab in tabs"
        :ref="(el) => setTabRef(tab.id, el as Element | null)"
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

    <div class="flex items-center shrink-0 px-1.5 pt-2 pb-1 gap-0.5 border-l border-border">
      <button
        type="button"
        class="inline-flex items-center justify-center size-8 rounded-md text-muted-foreground hover:bg-sidebar-accent/40 hover:text-foreground transition-colors"
        :title="t('chat.tabBarToolkit.newTerminal')"
        :aria-label="t('chat.tabBarToolkit.newTerminal')"
        @click="store.openTerminal()"
      >
        <TerminalSquare class="size-4" />
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <button
            type="button"
            class="inline-flex items-center justify-center size-8 rounded-md text-muted-foreground hover:bg-sidebar-accent/40 hover:text-foreground transition-colors"
            :title="t('chat.tabBarToolkit.menu')"
            :aria-label="t('chat.tabBarToolkit.menu')"
          >
            <MoreHorizontal class="size-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            :disabled="!tabs.length"
            @select="store.closeAll()"
          >
            {{ t('chat.tabBarToolkit.closeAll') }}
          </DropdownMenuItem>
          <DropdownMenuItem
            :disabled="!tabs.length"
            @select="store.closeFinished()"
          >
            {{ t('chat.tabBarToolkit.closeFinished') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { File as FileIcon, MessageSquare, MoreHorizontal, TerminalSquare, X } from 'lucide-vue-next'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import { useWorkspaceTabsStore, type WorkspaceTab } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { tabs, activeId } = storeToRefs(store)

const tabsContainerRef = ref<HTMLElement | null>(null)
const tabRefs = new Map<string, HTMLElement>()

function setTabRef(id: string, el: Element | null) {
  if (el) tabRefs.set(id, el as HTMLElement)
  else tabRefs.delete(id)
}

function scrollIntoViewHorizontal(container: HTMLElement, target: HTMLElement) {
  const cRect = container.getBoundingClientRect()
  const tRect = target.getBoundingClientRect()
  const margin = 8
  if (tRect.left < cRect.left) {
    container.scrollBy({ left: tRect.left - cRect.left - margin, behavior: 'smooth' })
  } else if (tRect.right > cRect.right) {
    container.scrollBy({ left: tRect.right - cRect.right + margin, behavior: 'smooth' })
  }
}

watch(
  [activeId, () => tabs.value.length],
  async () => {
    const id = activeId.value
    if (!id) return
    await nextTick()
    const container = tabsContainerRef.value
    const target = tabRefs.get(id)
    if (!container || !target) return
    scrollIntoViewHorizontal(container, target)
  },
  { flush: 'post' },
)

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
  switch (tab.type) {
    case 'chat': return MessageSquare
    case 'draft': return MessageSquare
    case 'file': return FileIcon
    case 'terminal': return TerminalSquare
  }
}

function resolveTitle(tab: WorkspaceTab): string {
  if (tab.type === 'chat') {
    return sessionTitleById.value[tab.sessionId] || tab.title || t('chat.untitledSession')
  }
  if (tab.type === 'draft') {
    return tab.title || t('chat.newSession')
  }
  if (tab.type === 'terminal') {
    return tab.title
  }
  return tab.title || tab.filePath
}
</script>
