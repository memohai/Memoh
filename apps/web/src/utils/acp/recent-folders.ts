// Recently chosen project folders for the composer's agent picker. The list is
// a convenience shortcut only — the session's own metadata stays the source of
// truth for the active path — so it lives in localStorage rather than the
// backend and degrades to empty when storage is unavailable.
const STORAGE_KEY = 'memoh.acp.recent-project-folders'
const MAX_ENTRIES = 6

function read(): string[] {
  if (typeof localStorage === 'undefined') return []
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((item): item is string => typeof item === 'string' && item.trim() !== '').slice(0, MAX_ENTRIES)
  } catch {
    return []
  }
}

function write(folders: string[]): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(folders.slice(0, MAX_ENTRIES)))
  } catch {
    // Ignore quota / privacy-mode failures: the shortcut list is non-essential.
  }
}

export function readRecentACPFolders(): string[] {
  return read()
}

export function rememberACPFolder(path: string): string[] {
  const next = path.trim()
  if (!next) return read()
  const folders = [next, ...read().filter(item => item !== next)].slice(0, MAX_ENTRIES)
  write(folders)
  return folders
}
