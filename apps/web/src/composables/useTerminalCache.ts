const STORAGE_PREFIX = 'terminal-cache:'
const MAX_CONTENT_BYTES = 100 * 1024
const MAX_CACHED_TABS = 10

export interface TerminalTabState {
  id: string
  label: string
  content: string
  savedAt: number
}

export interface TerminalCacheState {
  tabs: TerminalTabState[]
  activeTabId: string
}

function storageKey(botId: string): string {
  return `${STORAGE_PREFIX}${botId}`
}

function truncateContent(content: string): string {
  if (content.length <= MAX_CONTENT_BYTES) return content
  return content.slice(content.length - MAX_CONTENT_BYTES)
}

export function useTerminalCache() {
  function loadCache(botId: string): TerminalCacheState | null {
    try {
      const raw = localStorage.getItem(storageKey(botId))
      if (!raw) return null
      const parsed = JSON.parse(raw) as TerminalCacheState
      if (!Array.isArray(parsed.tabs) || !parsed.activeTabId) return null
      return parsed
    } catch {
      return null
    }
  }

  function saveCache(botId: string, state: TerminalCacheState) {
    try {
      const trimmed: TerminalCacheState = {
        activeTabId: state.activeTabId,
        tabs: state.tabs.slice(0, MAX_CACHED_TABS).map((tab) => ({
          id: tab.id,
          label: tab.label,
          content: truncateContent(tab.content),
          savedAt: Date.now(),
        })),
      }
      localStorage.setItem(storageKey(botId), JSON.stringify(trimmed))
    } catch {
      // localStorage full or unavailable
    }
  }

  function clearCache(botId: string) {
    try {
      localStorage.removeItem(storageKey(botId))
    } catch {
      // ignore
    }
  }

  return { loadCache, saveCache, clearCache }
}
