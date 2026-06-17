import { computed, ref } from 'vue'
import {
  desktopApiBridge,
  hostSurface,
  normalizeDesktopRuntimeMode,
  type DesktopRuntimeMode,
  type HostSurface,
} from '@/utils/desktop-runtime'

export function useDesktopRuntime() {
  const host = ref<HostSurface>(hostSurface())
  const desktopRuntimeMode = ref<DesktopRuntimeMode | null>(null)
  const loaded = ref(host.value === 'web')
  let loading: Promise<void> | null = null

  async function load(): Promise<void> {
    host.value = hostSurface()
    if (host.value === 'web') {
      desktopRuntimeMode.value = null
      loaded.value = true
      return
    }

    if (loading) return loading

    loading = (async () => {
      const status = await desktopApiBridge()?.desktop?.getServerStatus?.()
      desktopRuntimeMode.value = normalizeDesktopRuntimeMode(status?.mode)
      loaded.value = true
    })().catch((error) => {
      console.warn('failed to resolve desktop runtime mode', error)
      desktopRuntimeMode.value = null
      loaded.value = true
    }).finally(() => {
      loading = null
    })

    return loading
  }

  return {
    host,
    desktopRuntimeMode,
    loaded,
    isRemoteDesktop: computed(() => host.value === 'desktop' && desktopRuntimeMode.value === 'remote'),
    isLocalDesktop: computed(() => host.value === 'desktop' && desktopRuntimeMode.value === 'local'),
    load,
  }
}
