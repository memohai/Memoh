<script setup lang="ts">
import { provide, computed, toValue } from 'vue'
import { useRoute } from 'vue-router'
import { useQuery } from '@pinia/colada'
import { getBotsById } from '@memohai/sdk'
import {
  Toaster, SidebarInset,
  Breadcrumb, BreadcrumbList, BreadcrumbItem,
  BreadcrumbLink, BreadcrumbPage, BreadcrumbSeparator,
} from '@memohai/ui'
import 'vue-sonner/style.css'
import MainLayout from '@memohai/web/layout/main-layout/index.vue'
import SettingsSidebar from '@memohai/web/components/settings-sidebar/index.vue'
import { useSettingsStore } from '@memohai/web/store/settings'
import { DesktopShellKey } from '@memohai/web/lib/desktop-shell'

provide(DesktopShellKey, true)
useSettingsStore()

const route = useRoute()

// Fetch bot data in the layout to ensure reactive breadcrumb updates for bot-detail
const { data: bot } = useQuery({
  key: () => ['bot', route.params.botId as string],
  query: async () => {
    const { data } = await getBotsById({
      path: { id: route.params.botId as string },
      throwOnError: true,
    })
    return data
  },
  enabled: () => route.name === 'bot-detail' && !!route.params.botId,
})

const breadcrumbs = computed(() => {
  const items = []
  const matched = route.matched
  for (const m of matched) {
    if (m.meta && m.meta.breadcrumb) {
      let label = ''
      // Special case for bot-detail to use the reactive display name
      if (m.name === 'bot-detail' && bot.value?.display_name) {
        label = bot.value.display_name
      } else {
        const b = m.meta.breadcrumb
        label = typeof b === 'function' ? b(route) : toValue(b)
      }

      if (label) {
        items.push({
          label,
          to: m.name ? { name: m.name } : m.path,
          isLast: false,
        })
      }
    }
  }
  if (items.length > 0) {
    const lastItem = items[items.length - 1]
    if (lastItem) lastItem.isLast = true
  }
  return items
})
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
          <!-- Universal Settings Breadcrumb per Figma 5:937 & 5:807 -->
          <header
            v-if="breadcrumbs.length > 0"
            class="h-10 flex items-center px-6 shrink-0 border-b border-border/40"
          >
            <Breadcrumb class="w-full">
              <BreadcrumbList class="gap-1.5 flex-nowrap">
                <template
                  v-for="(item, index) in breadcrumbs"
                  :key="index"
                >
                  <BreadcrumbItem
                    v-if="!item.isLast"
                    class="shrink-0"
                  >
                    <BreadcrumbLink
                      as-child
                      class="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      <router-link :to="item.to">
                        <span class="text-[11px] font-medium leading-none">{{ item.label }}</span>
                      </router-link>
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator
                    v-if="!item.isLast"
                    class="text-muted-foreground/50 shrink-0 select-none"
                  >
                    <span class="text-[10px] font-normal">/</span>
                  </BreadcrumbSeparator>
                  <BreadcrumbItem
                    v-else
                    class="min-w-0 flex-1"
                  >
                    <BreadcrumbPage class="text-foreground text-[11px] font-medium truncate leading-none">
                      {{ item.label }}
                    </BreadcrumbPage>
                  </BreadcrumbItem>
                </template>
              </BreadcrumbList>
            </Breadcrumb>
          </header>

          <section class="flex-1 relative min-h-0">
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
