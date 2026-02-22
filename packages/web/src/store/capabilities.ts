import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getPing } from '@memoh/sdk'

export const useCapabilitiesStore = defineStore('capabilities', () => {
  const containerBackend = ref('containerd')
  const snapshotSupported = ref(true)
  const loaded = ref(false)

  async function load() {
    if (loaded.value) return
    try {
      const { data } = await getPing()
      if (data) {
        containerBackend.value = data.container_backend ?? 'containerd'
        snapshotSupported.value = data.snapshot_supported !== false
      }
    } catch {
      // fallback: assume containerd
    }
    loaded.value = true
  }

  return { containerBackend, snapshotSupported, loaded, load }
})
