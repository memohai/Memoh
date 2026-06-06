export interface WorkspaceTabCommandApi {
  onCloseCurrentWorkspaceTab(cb: () => void): void
}

export interface WorkspaceTabCommandStore {
  activeId: string | null
  closeTab(id: string): void
}

export function closeCurrentWorkspaceTab(store: WorkspaceTabCommandStore): boolean {
  const activeId = store.activeId
  if (!activeId) return false
  store.closeTab(activeId)
  return true
}

export function registerWorkspaceTabCommands(
  api: WorkspaceTabCommandApi,
  store: WorkspaceTabCommandStore,
): void {
  api.onCloseCurrentWorkspaceTab(() => {
    closeCurrentWorkspaceTab(store)
  })
}
