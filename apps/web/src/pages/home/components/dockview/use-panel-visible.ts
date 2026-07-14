import { onBeforeUnmount, ref, type Ref } from 'vue'
import type { DockviewPanelApi } from 'dockview-vue'

// Tracks dockview panel visibility. Visible-but-unfocused panels (e.g. in a
// split) still count as active for pane components: a terminal beside the
// chat keeps rendering output.
export function usePanelVisible(api: DockviewPanelApi): Ref<boolean> {
  const visible = ref(api.isVisible)
  const disposable = api.onDidVisibilityChange((event) => {
    visible.value = event.isVisible
  })
  onBeforeUnmount(() => disposable.dispose())
  return visible
}

export function usePanelFocused(api: DockviewPanelApi): Ref<boolean> {
  const focused = ref(api.isActive && api.isGroupActive)
  const sync = () => {
    focused.value = api.isActive && api.isGroupActive
  }
  const panelDisposable = api.onDidActiveChange(sync)
  const groupDisposable = api.onDidActiveGroupChange(sync)
  onBeforeUnmount(() => {
    panelDisposable.dispose()
    groupDisposable.dispose()
  })
  return focused
}
