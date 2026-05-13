<template>
  <SidebarProvider
    class="min-h-[initial]! absolute inset-0 "
    :default-open="true"
  >
    <Sidebar
      class="relative! **:[[role=navigation]]:relative! sidebar-container h-full! w-60! border-0! [&_[data-sidebar=sidebar]]:bg-transparent!"
    >
      <SidebarContent class="overflow-hidden p-2 pb-4 pt-4 h-full flex flex-col">
        <div class="border border-border/60 bg-muted/10 rounded-lg flex-1 flex flex-col overflow-hidden min-h-0">
          <!-- Integrated Header (if provided) -->
          <div
            v-if="slots['sidebar-header']"
            class="shrink-0"
          >
            <slot name="sidebar-header" />
          </div>
          
          <!-- Content Group with ScrollArea -->
          <ScrollArea class="flex-1 min-h-0">
            <div class="p-2 flex flex-col gap-1">
              <slot name="sidebar-content" />
            </div>
          </ScrollArea>

          <!-- Integrated Footer (if provided) -->
          <SidebarFooter
            v-if="slots['sidebar-footer']"
            class="p-2 pt-0"
          >
            <slot name="sidebar-footer" />
          </SidebarFooter>
        </div>
      </SidebarContent>
    </Sidebar>

    <SidebarInset class="min-w-0 overflow-hidden">
      <section class="flex-1 min-w-0 relative min-h-0 overflow-hidden">
        <slot name="detail" />
      </section>

      <div class="absolute right-4 top-0 h-10 z-20 md:hidden flex items-center">
        <Menu
          class="cursor-pointer p-2 size-9"
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
    </SidebarInset>
  </SidebarProvider>
</template>

<script setup lang="ts">
import { Menu } from 'lucide-vue-next'
import {  ref } from 'vue'
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
  Sidebar,
  SidebarInset,
  ScrollArea
} from '@memohai/ui'
import { useSlots } from 'vue'

const slots=useSlots()

const mobileOpen = ref(false)
</script>