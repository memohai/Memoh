<template>
  <section class="flex h-dvh overflow-hidden">
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
import { ref, watch } from 'vue'
import { SidebarProvider } from '@memohai/ui'
import { useMediaQuery } from '@vueuse/core'

const sidebarDefaultOpen = !document.cookie.includes('sidebar_state=false')
const isOpen = ref(sidebarDefaultOpen)

const isSmallScreen = useMediaQuery('(max-width: 1024px)')

watch(isSmallScreen, (isSmall) => {
  if (isSmall) {
    isOpen.value = false
  } else {
    isOpen.value = true
  }
}, { immediate: true })
</script>
