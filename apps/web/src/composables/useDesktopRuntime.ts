import { computed, ref } from 'vue'
import {
  hostSurface,
  type HostSurface,
} from '@/utils/desktop-runtime'

export function useDesktopRuntime() {
  const host = ref<HostSurface>(hostSurface())
  const loaded = ref(host.value === 'web')

  async function load(): Promise<void> {
    host.value = hostSurface()
    loaded.value = true
  }

  return {
    host,
    loaded,
    isDesktop: computed(() => host.value === 'desktop'),
    load,
  }
}
