<template>
  <SidebarProvider
    class="min-h-[initial]! absolute inset-0"
  >
    <aside
      class="hidden md:flex flex-col w-(--sidebar-width) border-r border-border shrink-0 h-full"
      data-sidebar="sidebar"
    >
      <SidebarHeader>
        <slot name="sidebar-header" />
      </SidebarHeader>
      <SidebarContent class="px-2 scrollbar-none">
        <slot name="sidebar-content" />
      </SidebarContent>
      <SidebarFooter v-if="$slots['sidebar-footer']">
        <slot name="sidebar-footer" />
      </SidebarFooter>
    </aside>

    <section class="flex-1 min-w-0 h-full">
      <slot name="detail" />
    </section>

    <div class="fixed right-4 top-0 h-12 z-1000 md:hidden flex items-center">
      <FontAwesomeIcon
        :icon="['fas', 'bars']"
        class="cursor-pointer p-2"
        @click="mobileOpen = !mobileOpen"
      />
    </div>

    <Sheet
      :open="mobileOpen"
      @update:open="(v: boolean) => mobileOpen = v"
    >
      <SheetContent
        data-sidebar="sidebar"
        side="left"
        class="bg-sidebar text-sidebar-foreground w-72 p-0 [&>button]:hidden"
      >
        <SheetHeader class="sr-only">
          <SheetTitle>Sidebar</SheetTitle>
          <SheetDescription>Sidebar navigation</SheetDescription>
        </SheetHeader>
        <div class="flex h-full w-full flex-col">
          <SidebarHeader>
            <slot name="sidebar-header" />
          </SidebarHeader>
          <SidebarContent class="px-2 scrollbar-none">
            <slot name="sidebar-content" />
          </SidebarContent>
          <SidebarFooter v-if="$slots['sidebar-footer']">
            <slot name="sidebar-footer" />
          </SidebarFooter>
        </div>
      </SheetContent>
    </Sheet>
  </SidebarProvider>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarProvider,
} from '@memoh/ui'

const mobileOpen = ref(false)
</script>
