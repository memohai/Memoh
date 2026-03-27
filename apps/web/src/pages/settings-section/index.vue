<template>
  <section>
    <MainLayout>
      <template #sidebar>
        <SettingsSidebar />
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
  </section>
</template>

<script setup lang="ts">
import { SidebarInset } from '@memohai/ui'
import MainLayout from '@/layout/main-layout/index.vue'
import SettingsSidebar from '@/components/settings-sidebar/index.vue'
</script>
