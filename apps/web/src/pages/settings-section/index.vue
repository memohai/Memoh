<template>
  <div class="flex h-dvh flex-col overflow-hidden">
    <!-- Full-width top bar: a drag region that clears the macOS traffic lights and
         visually separates the chrome from the sidebar/content below, so the sidebar
         no longer has to align its items to the traffic lights. Stays static while
         the view below slides on navigation. -->
    <header
      v-if="desktopShell"
      class="h-11 shrink-0 border-b border-border bg-background [-webkit-app-region:drag]"
    />

    <div class="min-h-0 flex-1">
      <!-- Whole settings view (sidebar + content) slides in from the right on open,
           faster slide-out on leave; navigation is held until the leave plays. -->
      <Transition
        appear
        enter-active-class="transition-all duration-[90ms] ease-out"
        enter-from-class="opacity-0 translate-x-2.5"
        leave-active-class="transition-all duration-[40ms] ease-in"
        leave-to-class="opacity-0 translate-x-2.5"
        @after-leave="onAfterLeave"
      >
        <div
          v-if="show"
          class="h-full"
        >
          <MainLayout>
            <template #sidebar>
              <!-- De-nest: inside a single bot, its own nav (rendered by
                   bots/detail.vue) takes over this column, so the settings nav
                   steps aside instead of stacking a second sidebar beside it. -->
              <SettingsSidebar v-if="!isBotDetail" />
            </template>
            <template #main>
              <SidebarInset class="flex flex-col overflow-hidden">
                <section class="flex-1 relative min-h-0 overflow-y-auto [scrollbar-gutter:stable]">
                  <router-view v-slot="{ Component }">
                    <KeepAlive>
                      <component :is="Component" />
                    </KeepAlive>
                  </router-view>
                </section>
              </SidebarInset>
            </template>
          </MainLayout>
        </div>
      </Transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { SidebarInset } from '@memohai/ui'
import MainLayout from '@/layout/main-layout/index.vue'
import SettingsSidebar from '@/components/settings-sidebar/index.vue'
import { DesktopShellKey } from '@/lib/desktop-shell'

const desktopShell = inject(DesktopShellKey, false)

const route = useRoute()
// On a single bot's detail page the bot's own nav owns the left column, so we
// drop the settings nav here to avoid two stacked sidebars (the "three-column"
// nesting). Every other settings route keeps the settings nav.
const isBotDetail = computed(() => route.name === 'bot-detail')

// Page transition: slide-in from the right on open, faster slide-out on leave.
// We hold the navigation until the leave animation has played.
const show = ref(true)
let pendingNext: (() => void) | null = null

onBeforeRouteLeave((_to, _from, next) => {
  show.value = false
  pendingNext = next
})

function onAfterLeave(): void {
  pendingNext?.()
  pendingNext = null
}
</script>
