<template>
  <section class="flex h-dvh overflow-hidden">
    <sidebar-provider
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
import { SidebarProvider } from '@memohai/ui'

const sidebarDefaultOpen = !document.cookie.includes('sidebar_state=false')
</script>
