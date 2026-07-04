<template>
  <div class="flex flex-col h-full min-w-0">
    <!-- Mode switcher where the old type filter used to live. A <TextButton>
         (ghost + text size = "clickable text with a hover chip") drives a
         DropdownMenu to pivot the list between human conversations (Recent) and
         system run streams (Schedule / Agent), restoring visibility of those
         runs without a separate history entry elsewhere. The button is
         naturally content-sized — only the text + chevron are the hit area. -->
    <div class="shrink-0 px-2 pb-0.5 pt-1">
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <TextButton class="text-xs font-[550] tracking-[-0.02em] pl-[11px] select-none">
            {{ t(activeMode.labelKey) }}
            <ChevronDown class="size-2.5" />
          </TextButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            v-for="m in MODES"
            :key="m.id"
            class="justify-between gap-4"
            @select="mode = m.id"
          >
            <span>{{ t(m.labelKey) }}</span>
            <Check
              class="size-3.5 shrink-0"
              :class="mode === m.id ? 'opacity-100' : 'opacity-0'"
            />
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <div class="flex-1 relative min-h-0">
      <!-- Native overflow scroll (not Reka ScrollArea) so the virtualizer can
           own the real scroll element. A bot's session list is unbounded — IM
           channels mint one session per chat/group — so rendering every row
           (each carrying a dropdown menu) froze the main thread for seconds when
           switching between two busy bots. Windowing keeps the mounted row count
           tied to the viewport, not the list length. -->
      <div
        ref="scrollEl"
        class="absolute inset-0 overflow-y-auto sidebar-scroll"
      >
        <div class="px-2 pr-3">
          <div
            v-if="visibleSessions.length > 0"
            :style="{ position: 'relative', width: '100%', height: `${totalSize}px` }"
          >
            <!-- pb-[2px] is the seam: the pill (SessionItem) keeps its own fill,
                 and the measured wrapper adds a thin transparent gap below it so
                 adjacent rows read as separate items instead of one block. Two
                 rows span 2×34px pill + 2px seam = 70px hover-top to hover-bottom. -->
            <div
              v-for="vRow in virtualRows"
              :key="vRow.key"
              :ref="measureRow"
              :data-index="vRow.index"
              class="pb-[2px]"
              :style="{ position: 'absolute', top: '0', left: '0', width: '100%', transform: `translateY(${vRow.start}px)` }"
            >
              <SessionItem
                :session="vRow.session"
                :is-active="sessionId === vRow.session.id"
                :streaming="chatStore.isSessionStreaming(vRow.session.id)"
                @select="handleSelect"
                @open-new-tab="handleOpenNewTab"
                @rename="openRenameSessionDialog"
                @delete="confirmDeleteSession"
              />
            </div>
          </div>

          <!-- Sentinel + bottom loader for cursor-paginated session history.
               Sits AFTER the virtualizer's height spacer so it lives at the
               natural end of the scroll content; the IntersectionObserver
               below uses scrollEl as its root and triggers the next page
               200px before the user actually reaches the bottom. -->
          <div
            v-if="hasMoreSessions"
            ref="loadMoreSentinel"
            data-testid="load-more-sentinel"
            class="h-px w-full"
          />

          <div
            v-if="loadingMoreSessions"
            class="flex justify-center py-3"
          >
            <LoaderCircle
              class="size-4 animate-spin text-muted-foreground"
            />
          </div>

          <div
            v-if="currentBotId && !loadingChats && visibleSessions.length === 0"
            class="px-3 py-6 text-center text-xs text-muted-foreground"
          >
            {{ t('chat.noSessions') }}
          </div>

          <div
            v-if="loadingChats"
            class="flex justify-center py-4"
          >
            <LoaderCircle
              class="size-4 animate-spin text-muted-foreground"
            />
          </div>
        </div>
      </div>
    </div>

    <Dialog v-model:open="deleteSessionDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('chat.deleteSession') }}</DialogTitle>
          <DialogDescription>{{ t('chat.deleteSessionConfirm') }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="deleteSessionLoading"
            @click="deleteSessionDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deleteSessionLoading"
            @click="handleDeleteSession"
          >
            <LoaderCircle
              v-if="deleteSessionLoading"
              class="mr-1 size-3 animate-spin"
            />
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="renameSessionDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('chat.renameSession') }}</DialogTitle>
          <DialogDescription>{{ t('chat.renameSessionDescription') }}</DialogDescription>
        </DialogHeader>
        <form
          class="space-y-4"
          @submit.prevent="handleRenameSession"
        >
          <Input
            v-model="renameSessionTitle"
            :placeholder="t('chat.renameSessionPlaceholder')"
            :disabled="renameSessionLoading"
            autofocus
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="renameSessionLoading"
              @click="renameSessionDialogOpen = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              :disabled="!renameSessionTitle.trim() || renameSessionLoading"
            >
              <LoaderCircle
                v-if="renameSessionLoading"
                class="mr-1 size-3 animate-spin"
              />
              {{ t('common.confirm') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import { Check, ChevronDown, LoaderCircle } from 'lucide-vue-next'
import { useLocalStorage } from '@vueuse/core'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useIntersectionObserver } from '@vueuse/core'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { isSessionVisibleInSidebarMode, sortByRecency, type SidebarSessionMode } from '@/store/chat-list.utils'
import type { SessionSummary } from '@/composables/api/useChat'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  Button,
  TextButton,
  Input,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@memohai/ui'
import SessionItem from './session-item.vue'

const { t } = useI18n()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const {
  sessions,
  sessionId,
  currentBotId,
  loadingChats,
  hasMoreSessions,
  loadingMoreSessions,
} = storeToRefs(chatStore)

// The list pivots between human conversations (Recent) and system run streams
// (Schedule / Agent) via the header button. Recent keeps the user's real
// chat/discuss timeline; Schedule and Agent surface the previously-hidden
// system runs so a user can re-open a run to see whether it broke — no
// separate history entry needed elsewhere.
type RecentMode = SidebarSessionMode
const MODES: { id: RecentMode, labelKey: string }[] = [
  { id: 'recent', labelKey: 'chat.recents' },
  { id: 'schedule', labelKey: 'chat.activityBar.schedule' },
  { id: 'agent', labelKey: 'chat.sessionTypeACPAgent' },
]
const mode = useLocalStorage<RecentMode>('workspace-sidebar-recents-mode', 'recent')
const activeMode = computed(() => MODES.find(m => m.id === mode.value) ?? MODES[0]!)

const visibleSessions = computed(() => {
  return sortByRecency(sessions.value.filter(s => isSessionVisibleInSidebarMode(s, activeMode.value.id)))
})

// ---- virtualized session list ----
// Rows are MEASURED, not pinned to a px stride: SessionItem sizes in rem (min-h-9)
// so its real height tracks the UI font setting / browser zoom. A fixed px estimate
// would desync the row offsets the moment the font scales, so we hand the virtualizer
// a rough estimate and let measureElement read each row's actual height.
const scrollEl = ref<HTMLElement | null>(null)
const virtualizer = useVirtualizer<HTMLElement, HTMLElement>(
  computed(() => ({
    count: visibleSessions.value.length,
    getScrollElement: () => scrollEl.value,
    estimateSize: () => 36,
    overscan: 10,
    getItemKey: (index: number) => visibleSessions.value[index]?.id ?? index,
  })),
)
const totalSize = computed(() => virtualizer.value.getTotalSize())
const virtualRows = computed(() =>
  virtualizer.value.getVirtualItems().flatMap((vi) => {
    const session = visibleSessions.value[vi.index]
    return session ? [{ key: String(vi.key), index: vi.index, start: vi.start, session }] : []
  }),
)

// Read each rendered row's real (rem-based) height so offsets stay correct when the
// UI font scales — pairs with the estimate above.
function measureRow(el: unknown) {
  if (el instanceof HTMLElement) virtualizer.value.measureElement(el)
}

// Cursor-paginated load-more: a sentinel sits at the natural bottom of the
// scroll content and fires the next page 200px before the user reaches it.
// The store's loadMoreSessions guards re-entry (loadingMoreSessions / cursor
// exhaustion), so the observer can fire freely as the user scrolls.
const loadMoreSentinel = ref<HTMLElement | null>(null)
useIntersectionObserver(
  loadMoreSentinel,
  ([entry]) => {
    if (!entry?.isIntersecting) return
    void chatStore.loadMoreSessions()
  },
  {
    root: scrollEl,
    rootMargin: '0px 0px 200px 0px',
    threshold: 0,
  },
)

// Switching bots swaps the whole list; start the new bot at the top instead of
// inheriting the previous bot's scroll offset (which could land past the end of
// a shorter list).
watch(currentBotId, () => {
  nextTick(() => {
    scrollEl.value?.scrollTo({ top: 0 })
    virtualizer.value.scrollToOffset(0)
  })
})

function handleSelect(session: SessionSummary) {
  // Opening (or focusing) the tab activates it, which selects the session.
  workspaceTabs.openSessionChat({
    sessionId: session.id,
    title: (session.title ?? '').trim() || t('chat.untitledSession'),
  })
}

// Right-click "Open": a separate pinned tab instead of reusing the preview slot.
function handleOpenNewTab(session: SessionSummary) {
  workspaceTabs.openSessionChatPinned({
    sessionId: session.id,
    title: (session.title ?? '').trim() || t('chat.untitledSession'),
  })
}

const deleteSessionDialogOpen = ref(false)
const deleteSessionLoading = ref(false)
const sessionPendingDelete = ref<SessionSummary | null>(null)
const renameSessionDialogOpen = ref(false)
const renameSessionLoading = ref(false)
const sessionPendingRename = ref<SessionSummary | null>(null)
const renameSessionTitle = ref('')

function confirmDeleteSession(session: SessionSummary) {
  sessionPendingDelete.value = session
  deleteSessionDialogOpen.value = true
}

function openRenameSessionDialog(session: SessionSummary) {
  sessionPendingRename.value = session
  renameSessionTitle.value = session.title?.trim() || ''
  renameSessionDialogOpen.value = true
}

async function handleRenameSession() {
  const target = sessionPendingRename.value
  const title = renameSessionTitle.value.trim()
  if (!target || !title || renameSessionLoading.value) return
  renameSessionLoading.value = true
  try {
    await chatStore.renameSession(target.id, title)
    renameSessionDialogOpen.value = false
    sessionPendingRename.value = null
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('chat.renameSessionFailed')))
  } finally {
    renameSessionLoading.value = false
  }
}

async function handleDeleteSession() {
  const target = sessionPendingDelete.value
  if (!target || deleteSessionLoading.value) return
  deleteSessionLoading.value = true
  try {
    await chatStore.removeSession(target.id, { fallbackMode: activeMode.value.id })
    deleteSessionDialogOpen.value = false
    sessionPendingDelete.value = null
  } finally {
    deleteSessionLoading.value = false
  }
}
</script>

<style scoped>
/* Sidebar-specific native scroll bar: narrow so it doesn't dominate the tight
 * session list. The 2px transparent border + padding-box clip insets the thumb
 * to a ~4px sliver — visible on hover/scroll but not a constant presence. The
 * extra pr-3 on the inner container keeps the session hover chip off the bar. */
.sidebar-scroll {
  scrollbar-width: thin;
  scrollbar-color: color-mix(in oklab, var(--foreground) 18%, transparent) transparent;
}
.sidebar-scroll::-webkit-scrollbar {
  width: 8px;
}
.sidebar-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.sidebar-scroll::-webkit-scrollbar-thumb {
  background-color: color-mix(in oklab, var(--foreground) 18%, transparent);
  background-clip: padding-box;
  border: 2px solid transparent;
  border-radius: 9999px;
}
</style>
