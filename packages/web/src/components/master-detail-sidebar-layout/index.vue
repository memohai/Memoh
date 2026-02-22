<template>
  <div class="w-full mx-auto">
    <div class="model-select">
      <SidebarProvider
        :open="true"
        class="min-h-[initial]! flex **:data-[sidebar=sidebar]:bg-transparent absolute inset-0"
      >
        <Sidebar class="h-full relative top-0">
          <SidebarHeader>
            <slot name="sidebar-header" />
          </SidebarHeader>
          <SidebarContent class="px-2 scrollbar-none">
            <slot name="sidebar-content" />
          </SidebarContent>
          <SidebarFooter>
            <slot name="sidebar-footer" />
          </SidebarFooter>
        </Sidebar>
        <section class="flex-1 h-full">
          <slot name="detail" />
        </section>
      </SidebarProvider>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarProvider,
} from '@memoh/ui'
</script>
