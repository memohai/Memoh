interface TerminalSnapshot {
  data: string
  updatedAt: number
}

const MAX_TERMINAL_SNAPSHOTS = 32
const snapshots = new Map<string, TerminalSnapshot>()

export function terminalCacheKey(botId: string, tabId: string): string {
  return `${botId.trim()}::${tabId.trim()}`
}

export function readTerminalSnapshot(key: string): string | null {
  return snapshots.get(key)?.data ?? null
}

export function writeTerminalSnapshot(key: string, data: string) {
  if (!key) return
  if (!data) {
    snapshots.delete(key)
    return
  }
  snapshots.set(key, { data, updatedAt: Date.now() })
  pruneTerminalSnapshots()
}

export function deleteTerminalSnapshot(key: string) {
  snapshots.delete(key)
}

export function clearTerminalSnapshotsForBot(botId: string) {
  const prefix = `${botId.trim()}::`
  for (const key of snapshots.keys()) {
    if (key.startsWith(prefix)) snapshots.delete(key)
  }
}

export function clearTerminalSnapshots() {
  snapshots.clear()
}

function pruneTerminalSnapshots() {
  if (snapshots.size <= MAX_TERMINAL_SNAPSHOTS) return
  const entries = [...snapshots.entries()].sort((a, b) => a[1].updatedAt - b[1].updatedAt)
  for (const [key] of entries.slice(0, snapshots.size - MAX_TERMINAL_SNAPSHOTS)) {
    snapshots.delete(key)
  }
}
