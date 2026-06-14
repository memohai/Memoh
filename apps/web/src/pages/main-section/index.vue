<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100'"
  >
    <!-- PUSH/PULL. The sidebar (a flex sibling) slides out left and the dock,
         flex-1, grows to fill the freed space — content shifts rather than being
         covered. dockview relays out per frame (no gating) to keep panels matched
         the whole way. -->
    <div class="flex h-full min-h-0 overflow-hidden">
      <SideBar :mac-traffic-reserve="macTrafficReserve" />
      <div class="flex min-w-0 min-h-0 flex-1 flex-col">
        <MainContainer />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, onMounted } from 'vue'
import { DesktopShellKey } from '@/lib/desktop-shell'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { safeSessionGet, safeSessionRemove } from '@/utils/safe-storage'

const desktopShell = inject(DesktopShellKey, false)
const macTrafficReserve = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)

const shouldAnimateEntry = safeSessionGet(ONBOARDING_KEYS.entryAnimation) === '1'
if (shouldAnimateEntry) {
  safeSessionRemove(ONBOARDING_KEYS.entryAnimation)
}

const entering = ref(shouldAnimateEntry)

onMounted(() => {
  if (!shouldAnimateEntry) return
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      entering.value = false
    })
  })
})
</script>
