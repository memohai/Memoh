<template>
  <section class="flex">
    <sidebar-provider
      :default-open="sidebarDefaultOpen"
      class="**:data-[sidebar=sidebar]:bg-transparent"
    >
      <slot name="sidebar" />
      <slot name="main" />
    </sidebar-provider>
  </section>
</template>
<script setup lang="ts">
import { SidebarProvider } from '@memoh/ui'

const sidebarDefaultOpen = document.cookie.includes('sidebar_state=true')
</script>
