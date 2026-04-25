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
    <MainLayout>
      <template #sidebar>
        <!-- Desktop hosts settings in a dedicated window, so the sidebar's
             "← Settings" header (back-to-chat affordance) is suppressed. -->
        <SettingsSidebar :hide-header="true" />
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
