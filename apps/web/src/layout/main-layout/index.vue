<template>
  <section class="flex w-full h-dvh overflow-hidden">
    <sidebar-provider
      v-model:open="isOpen"
      class="min-h-0 h-full"
      :default-open="sidebarDefaultOpen"
    >
      <section class="relative">
        <slot name="sidebar" />
      </section>
    
      <section class="main-left-section" />
      <slot name="main" />  
      <section class="main-right-section" />
    </sidebar-provider>
  </section>
</template>
<script setup lang="ts">
import { ref, watch, inject } from 'vue'
import { SidebarProvider } from '@memohai/ui'
import { useMediaQuery } from '@vueuse/core'
import { DesktopShellKey } from '@/lib/desktop-shell'

// In the desktop shell the sidebar collapse affordance is intentionally
// disabled — we keep the sidebar pinned open and skip the small-screen
// auto-collapse watcher so window resizes don't fight the layout.
const desktopShell = inject(DesktopShellKey, false)

const sidebarDefaultOpen = desktopShell || !document.cookie.includes('sidebar_state=false')
const isOpen = ref(sidebarDefaultOpen)

const isSmallScreen = useMediaQuery('(max-width: 1024px)')

watch(isSmallScreen, (isSmall) => {
  if (desktopShell) return
  if (isSmall) {
    isOpen.value = false
  } else {
    isOpen.value = true
  }
}, { immediate: true })
</script>
