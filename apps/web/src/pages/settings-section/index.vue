<template>
  <MainLayout>
    <template #sidebar>
      <SettingsSidebar />
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

        <section class="flex-1 relative min-h-0 overflow-y-auto">
          <router-view v-slot="{ Component }">
            <KeepAlive>
              <component :is="Component" />
            </KeepAlive>
          </router-view>
        </section>
      </SidebarInset>
    </template>
  </MainLayout>
</template>

<script setup lang="ts">
import { computed, toValue } from 'vue'
import { useRoute } from 'vue-router'
import { useQuery } from '@pinia/colada'
import { getBotsById } from '@memohai/sdk'
import {
  SidebarInset,
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@memohai/ui'
import MainLayout from '@/layout/main-layout/index.vue'
import SettingsSidebar from '@/components/settings-sidebar/index.vue'

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
    items[items.length - 1].isLast = true
  }
  return items
})
</script>
