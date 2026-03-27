<template>
  <SidebarProvider class="min-h-[initial]! absolute inset-0 ">
    <Sidebar
      class="relative! **:[[role=navigation]]:relative! sidebar-container h-full!"
    >
      <SidebarHeader v-if="slots['sidebar-header']">
        <slot name="sidebar-header" />
      </SidebarHeader>
      <SidebarContent>
        <ScrollArea class="h-full">
          <SidebarMenu class="p-2">           
            <slot name="sidebar-content" />
          </SidebarMenu>
        </ScrollArea>
      </SidebarContent>
      <SidebarFooter v-if="$slots['sidebar-footer']">
        <slot name="sidebar-footer" />
      </SidebarFooter>
    </Sidebar>

    <SidebarInset>
      <ScrollArea class="h-full">
        <section class="flex-1 min-w-0">
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
      </ScrollArea>
    </SidebarInset>
  </SidebarProvider>
</template>

<script setup lang="ts">
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
  ScrollArea,
  SidebarMenu
} from '@memohai/ui'
import { useSlots } from 'vue'

const slots=useSlots()

const mobileOpen = ref(false)
</script>