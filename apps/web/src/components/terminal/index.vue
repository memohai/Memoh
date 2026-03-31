<script setup lang="ts">
import { ref, reactive, shallowReactive, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SerializeAddon } from '@xterm/addon-serialize'
import { client } from '@memohai/sdk/client'
import { Button } from '@memohai/ui'
import { useTerminalCache } from '@/composables/useTerminalCache'
import type { TerminalCacheState } from '@/composables/useTerminalCache'
import '@xterm/xterm/css/xterm.css'

const props = withDefaults(defineProps<{
  botId: string
  visible?: boolean
}>(), {
  visible: true,
})

const { t } = useI18n()
const { loadCache, saveCache } = useTerminalCache()

const TERMINAL_OPTIONS = {
  cursorBlink: true,
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  theme: {
    background: '#1a1b26',
    foreground: '#a9b1d6',
    cursor: '#c0caf5',
    selectionBackground: '#33467c',
  },
} as const

interface TerminalTab {
  id: string
  label: string
  terminal: Terminal | null
  fitAddon: FitAddon | null
  serializeAddon: SerializeAddon | null
  ws: WebSocket | null
  status: 'idle' | 'connecting' | 'connected' | 'disconnected'
  containerEl: HTMLDivElement | null
  wsDisposables: Array<{ dispose(): void }>
}

function makeTab(id: string, label: string): TerminalTab {
  return shallowReactive<TerminalTab>({
    id,
    label,
    terminal: null,
    fitAddon: null,
    serializeAddon: null,
    ws: null,
    status: 'idle',
    containerEl: null,
    wsDisposables: [],
  })
}

const tabs = reactive<TerminalTab[]>([])
const activeTabId = ref('')
const wrapperRef = ref<HTMLDivElement | null>(null)
let tabCounter = 0
let resizeObserver: ResizeObserver | null = null
let fitTimer: ReturnType<typeof setTimeout> | null = null
let cacheTimer: ReturnType<typeof setTimeout> | null = null
const CACHE_DEBOUNCE_MS = 2000

const activeTermTab = computed(() => tabs.find((t) => t.id === activeTabId.value))

function resolveTerminalWsUrl(botIdValue: string, cols: number, rows: number): string {
  const baseUrl = String(client.getConfig().baseUrl || '').trim()
  const token = localStorage.getItem('token') ?? ''
  const path = `/bots/${encodeURIComponent(botIdValue)}/container/terminal/ws`
  const query = `?token=${encodeURIComponent(token)}&cols=${cols}&rows=${rows}`

  if (!baseUrl || baseUrl.startsWith('/')) {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    const base = baseUrl || '/api'
    return `${proto}//${loc.host}${base.replace(/\/+$/, '')}${path}${query}`
  }

  try {
    const url = new URL(path, baseUrl)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    return url.toString() + query
  } catch {
    const loc = window.location
    const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${loc.host}/api${path}${query}`
  }
}

function createTerminalTab(label?: string, cachedContent?: string): TerminalTab {
  tabCounter++
  const id = `term-${Date.now()}-${tabCounter}`
  const tab = makeTab(id, label ?? `${t('bots.terminal.defaultTabLabel')} ${tabCounter}`)
  tabs.push(tab)

  nextTick(() => {
    const el = document.getElementById(id)
    if (!el) return
    tab.containerEl = el as HTMLDivElement
    initTerminal(tab, cachedContent)
  })

  return tab
}

function initTerminal(tab: TerminalTab, cachedContent?: string) {
  if (!tab.containerEl) return

  const terminal = new Terminal({ ...TERMINAL_OPTIONS })
  const fitAddon = new FitAddon()
  const serializeAddon = new SerializeAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(serializeAddon)
  terminal.open(tab.containerEl)

  tab.terminal = terminal
  tab.fitAddon = fitAddon
  tab.serializeAddon = serializeAddon

  if (cachedContent) {
    terminal.write(cachedContent)
    terminal.write('\x1b[2K\r')
  }

  nextTick(() => {
    if (tab.id === activeTabId.value) fitAddon.fit()
    connectWs(tab)
  })
}

function connectWs(tab: TerminalTab) {
  if (!tab.terminal) return
  closeWs(tab)

  if (tab.id === activeTabId.value) {
    tab.fitAddon?.fit()
  }

  const cols = tab.terminal.cols
  const rows = tab.terminal.rows

  tab.status = 'connecting'
  const url = resolveTerminalWsUrl(props.botId, cols, rows)
  const ws = new WebSocket(url)
  ws.binaryType = 'arraybuffer'
  tab.ws = ws

  ws.onopen = () => {
    tab.status = 'connected'
  }

  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      tab.terminal?.write(new Uint8Array(event.data))
    } else if (typeof event.data === 'string') {
      tab.terminal?.write(event.data)
    }
    debouncedPersistCache()
  }

  ws.onclose = () => {
    tab.status = 'disconnected'
    tab.terminal?.write('\r\n\x1b[31m[Connection closed]\x1b[0m\r\n')
  }

  ws.onerror = () => {
    tab.status = 'disconnected'
  }

  for (const d of tab.wsDisposables) d.dispose()
  tab.wsDisposables = []

  tab.wsDisposables.push(
    tab.terminal.onData((data) => {
      if (tab.ws && tab.ws.readyState === WebSocket.OPEN) {
        tab.ws.send(new TextEncoder().encode(data))
      }
      debouncedPersistCache()
    }),
    tab.terminal.onResize(({ cols: c, rows: r }) => {
      if (tab.ws && tab.ws.readyState === WebSocket.OPEN) {
        tab.ws.send(JSON.stringify({ type: 'resize', cols: c, rows: r }))
      }
    }),
  )
}

function closeWs(tab: TerminalTab) {
  if (tab.ws) {
    tab.ws.onclose = null
    tab.ws.onerror = null
    tab.ws.onmessage = null
    tab.ws.close()
    tab.ws = null
  }
}

function destroyTab(tab: TerminalTab) {
  closeWs(tab)
  for (const d of tab.wsDisposables) d.dispose()
  tab.wsDisposables = []
  tab.terminal?.dispose()
  tab.terminal = null
  tab.fitAddon = null
  tab.serializeAddon = null
  tab.containerEl = null
}

function handleAddTab() {
  const tab = createTerminalTab()
  activeTabId.value = tab.id
  debouncedPersistCache()
}

function handleCloseTab(tabId: string) {
  const idx = tabs.findIndex((t) => t.id === tabId)
  if (idx < 0) return

  const target = tabs[idx]
  if (target) destroyTab(target)
  tabs.splice(idx, 1)

  if (tabs.length === 0) {
    handleAddTab()
    return
  }

  if (activeTabId.value === tabId) {
    const nextIdx = Math.min(idx, tabs.length - 1)
    const next = tabs[nextIdx]
    if (next) {
      activeTabId.value = next.id
      nextTick(() => next.fitAddon?.fit())
    }
  }
  debouncedPersistCache()
}

function handleSwitchTab(tabId: string) {
  if (activeTabId.value === tabId) return
  activeTabId.value = tabId
  debouncedPersistCache()
  nextTick(() => {
    const tab = tabs.find((t) => t.id === tabId)
    tab?.fitAddon?.fit()
  })
}

function handleReconnect() {
  const tab = activeTermTab.value
  if (tab) connectWs(tab)
}

function persistCache() {
  if (!props.botId || tabs.length === 0) return
  const state: TerminalCacheState = {
    activeTabId: activeTabId.value,
    tabs: tabs.map((tab) => ({
      id: tab.id,
      label: tab.label,
      content: tab.serializeAddon?.serialize() ?? '',
      savedAt: Date.now(),
    })),
  }
  saveCache(props.botId, state)
}

function debouncedPersistCache() {
  if (cacheTimer) clearTimeout(cacheTimer)
  cacheTimer = setTimeout(() => {
    persistCache()
  }, CACHE_DEBOUNCE_MS)
}

function restoreFromCache() {
  const cached = loadCache(props.botId)
  if (!cached || cached.tabs.length === 0) {
    handleAddTab()
    return
  }

  for (const cachedTab of cached.tabs) {
    tabCounter++
    const tab = makeTab(cachedTab.id, cachedTab.label)
    tabs.push(tab)
    const content = cachedTab.content

    nextTick(() => {
      const el = document.getElementById(tab.id)
      if (!el) return
      tab.containerEl = el as HTMLDivElement
      initTerminal(tab, content)
    })
  }

  const firstTab = cached.tabs[0]
  const targetId = cached.tabs.find((ct) => ct.id === cached.activeTabId)
    ? cached.activeTabId
    : firstTab?.id ?? ''
  activeTabId.value = targetId
}

function onBeforeUnload() {
  persistCache()
}

function cleanupAll() {
  if (fitTimer) {
    clearTimeout(fitTimer)
    fitTimer = null
  }
  if (cacheTimer) {
    clearTimeout(cacheTimer)
    cacheTimer = null
  }
  resizeObserver?.disconnect()
  resizeObserver = null
  for (const tab of tabs) {
    destroyTab(tab)
  }
  tabs.length = 0
}

function setupResizeObserver() {
  if (resizeObserver || !wrapperRef.value) return
  resizeObserver = new ResizeObserver(() => {
    if (fitTimer) clearTimeout(fitTimer)
    fitTimer = setTimeout(() => {
      activeTermTab.value?.fitAddon?.fit()
    }, 50)
  })
  resizeObserver.observe(wrapperRef.value)
}

function init() {
  window.addEventListener('beforeunload', onBeforeUnload)
  restoreFromCache()
  nextTick(() => setupResizeObserver())
}

onMounted(() => {
  if (props.visible) {
    init()
  }
})

watch(() => props.visible, (visible) => {
  if (visible && tabs.length === 0) {
    nextTick(() => init())
  }
  if (!visible && tabs.length > 0) {
    persistCache()
  }
  if (visible) {
    nextTick(() => activeTermTab.value?.fitAddon?.fit())
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  persistCache()
  cleanupAll()
})
</script>

<template>
  <div class="flex flex-col absolute inset-0 p-4">
    <!-- Tab bar -->
    <div class="flex items-center gap-1 mb-2 min-h-[36px]">
      <div class="flex items-center gap-1 flex-1 overflow-x-auto">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md border transition-colors whitespace-nowrap"
          :class="tab.id === activeTabId
            ? 'bg-accent text-accent-foreground border-border'
            : 'text-muted-foreground border-transparent hover:bg-accent/50'"
          @click="handleSwitchTab(tab.id)"
        >
          <span
            class="size-1.5 rounded-full shrink-0"
            :class="{
              'bg-green-500': tab.status === 'connected',
              'bg-yellow-500': tab.status === 'connecting',
              'bg-muted-foreground': tab.status === 'idle' || tab.status === 'disconnected',
            }"
          />
          <span>{{ tab.label }}</span>
          <span
            class="ml-1 size-4 inline-flex cursor-pointer items-center justify-center rounded hover:bg-destructive/20 hover:text-destructive"
            role="button"
            tabindex="0"
            :title="t('bots.terminal.closeTab')"
            @click.stop="handleCloseTab(tab.id)"
            @keydown.enter.prevent.stop="handleCloseTab(tab.id)"
            @keydown.space.prevent.stop="handleCloseTab(tab.id)"
          >
            &times;
          </span>
        </button>
        <button
          class="inline-flex items-center justify-center size-7 rounded-md border border-dashed border-border text-muted-foreground hover:bg-accent/50 hover:text-accent-foreground transition-colors shrink-0"
          :title="t('bots.terminal.newTab')"
          @click="handleAddTab"
        >
          +
        </button>
      </div>

      <div class="flex items-center gap-2 shrink-0 ml-2">
        <span
          v-if="activeTermTab"
          class="inline-flex items-center gap-1.5 text-xs"
          :class="{
            'text-green-500': activeTermTab.status === 'connected',
            'text-yellow-500': activeTermTab.status === 'connecting',
            'text-muted-foreground': activeTermTab.status === 'idle' || activeTermTab.status === 'disconnected',
          }"
        >
          {{ t(`bots.terminal.status.${activeTermTab.status}`) }}
        </span>
        <Button
          v-if="activeTermTab?.status === 'disconnected'"
          size="sm"
          variant="outline"
          @click="handleReconnect"
        >
          {{ t('bots.terminal.reconnect') }}
        </Button>
      </div>
    </div>

    <!-- Terminal area -->
    <div
      ref="wrapperRef"
      class="flex-1 relative min-h-0 rounded-md overflow-hidden border border-border terminal-wrapper"
    >
      <div
        v-for="tab in tabs"
        :id="tab.id"
        :key="tab.id"
        class="absolute inset-0 terminal-container"
        :style="{ display: tab.id === activeTabId ? 'block' : 'none' }"
      />
    </div>
  </div>
</template>

<style scoped>
.terminal-wrapper {
  background-color: #1a1b26;
}

.terminal-container {
  top: 8px;
  left: 8px;
  right: 8px;
  bottom: 8px;
}
</style>
