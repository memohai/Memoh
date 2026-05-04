<script setup lang="ts">
import { provide } from 'vue'
import { Toaster, SidebarInset } from '@memohai/ui'
import 'vue-sonner/style.css'
import MainLayout from '@memohai/web/layout/main-layout/index.vue'
import SettingsSidebar from '@memohai/web/components/settings-sidebar/index.vue'
import { useSettingsStore } from '@memohai/web/store/settings'
import { DesktopShellKey } from '@memohai/web/lib/desktop-shell'

provide(DesktopShellKey, true)
useSettingsStore()
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <!-- Invisible 16px drag strip pinned to the very top edge of the
         window. Sized to match the routed sections' `p-4` top
         padding so it sits entirely within the page's existing dead
         space and never overlaps a button or input on standard
         pages. On MasterDetailSidebarLayout pages the inner sidebar
         menu only has `p-2` (8px), so the strip's lower 8px clips
         the very top of the first sidebar item — but those buttons
         carry `py-5` and remain fully usable since only ~8px of a
         ~50px-tall hit area is consumed. The SettingsSidebar's own
         fixed drag header sits at `z-20` above this layer, so the
         left half is visually unchanged (still `bg-sidebar` 36px);
         the right half gains a thin transparent grab zone. macOS
         only by intent — on Windows / Linux the native title bar
         handles dragging and this layer is harmless. -->
    <div
      class="fixed top-0 left-0 right-0 h-4 z-10 [-webkit-app-region:drag]"
      aria-hidden="true"
    />
    <MainLayout>
      <template #sidebar>
        <!-- Desktop hosts settings in a dedicated window, so the sidebar's
             "← Settings" header (back-to-chat affordance) is suppressed. -->
        <SettingsSidebar
          :hide-header="true"
          :exclude-items="['profile']"
        />
      </template>
      <template #main>
        <SidebarInset class="flex flex-col overflow-hidden">
          <section class="flex-1 overflow-y-auto">
            <router-view v-slot="{ Component }">
              <KeepAlive>
                <component :is="Component" />
              </KeepAlive>
            </router-view>
          </section>
        </SidebarInset>
      </template>
    </MainLayout>
    <Toaster position="top-center" />
  </section>
</template>
