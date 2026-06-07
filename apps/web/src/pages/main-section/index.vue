<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100'"
  >
    <AppShell>
      <template #sidebar>
        <SideBar />
      </template>
      <template #main>
        <MainContainer />
      </template>
    </AppShell>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import AppShell from '@/layout/app-shell/index.vue'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { safeSessionGet, safeSessionRemove } from '@/utils/safe-storage'

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
