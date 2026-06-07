<template>
  <section class="flex w-full h-dvh overflow-hidden bg-background text-foreground">
    <!-- Global Primary Sidebar (The narrow leftmost strip) -->
    <aside
      class="w-14 flex-shrink-0 border-r border-border flex flex-col items-center py-4 gap-4 bg-sidebar"
      :class="macTopInset ? 'pt-12' : ''"
    >
      <div
        v-if="macTopInset"
        class="fixed top-0 left-0 w-14 h-12 [-webkit-app-region:drag] z-50"
      />
      
      <!-- Top actions (e.g. Chat) -->
      <nav class="flex flex-col gap-3 w-full items-center">
        <Button
          variant="ghost"
          size="icon"
          class="size-10 rounded-xl"
          :class="!isSettingsActive ? 'bg-accent/50 text-accent-foreground' : 'text-muted-foreground hover:text-foreground'"
          @click="router.push('/')"
        >
          <MessageSquare class="size-5" />
        </Button>
      </nav>
      
      <div class="flex-1" />
      
      <!-- Bottom actions (e.g. Settings, Profile) -->
      <nav class="flex flex-col gap-3 w-full items-center">
        <Button
          variant="ghost"
          size="icon"
          class="size-10 rounded-xl"
          :class="isSettingsActive ? 'bg-accent/50 text-accent-foreground' : 'text-muted-foreground hover:text-foreground'"
          @click="router.push('/settings')"
        >
          <Settings class="size-5" />
        </Button>
      </nav>
    </aside>

    <!-- Secondary Sidebar + Main Content Area -->
    <sidebar-provider
      v-model:open="isOpen"
      class="flex-1 min-w-0 h-full"
      :default-open="sidebarDefaultOpen"
    >
      <section class="relative">
        <slot name="sidebar" />
      </section>
    
      <slot name="main" />  
    </sidebar-provider>
  </section>
</template>

<script setup lang="ts">
import { ref, watch, inject, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { SidebarProvider, Button } from '@memohai/ui'
import { useMediaQuery } from '@vueuse/core'
import { MessageSquare, Settings } from 'lucide-vue-next'
import { DesktopShellKey } from '@/lib/desktop-shell'

const router = useRouter()
const route = useRoute()

const desktopShell = inject(DesktopShellKey, false)
const macTopInset = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)

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

const isSettingsActive = computed(() => route.path.startsWith('/settings'))
</script>
