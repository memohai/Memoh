<template>
  <div
    class="flex flex-col h-dvh overflow-hidden"
    :class="macTopInset ? 'pt-0' : ''"
  >
    <!-- Full-width title bar — spans sidebar + content, draggable on macOS -->
    <header
      class="h-11 shrink-0 flex items-center border-b border-border"
      :class="macTopInset ? '[-webkit-app-region:drag]' : ''"
    >
      <!-- Traffic-light clearance: ~70px for the three buttons + inset -->
      <div
        v-if="macTopInset"
        class="w-[4.5rem] shrink-0"
      />
      <Button
        variant="ghost"
        size="sm"
        class="[-webkit-app-region:no-drag]"
        :class="macTopInset ? '' : 'ml-3'"
        @click="router.push(backToChatRoute)"
      >
        <ChevronLeft class="size-3.5" />
        {{ t('sidebar.settings') }}
      </Button>
    </header>

    <!-- Main layout takes remaining height; sidebar hides its own back-button header -->
    <div class="flex-1 min-h-0">
      <MainLayout>
        <template #sidebar>
          <SettingsSidebar :hide-header="true" />
        </template>
        <template #main>
          <SidebarInset class="flex flex-col overflow-hidden">
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
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, toValue } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { ChevronLeft } from 'lucide-vue-next'
import { getBotsById } from '@memohai/sdk'
import {
  Button,
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
import { useChatSelectionStore } from '@/store/chat-selection'
import { useChatStore } from '@/store/chat-list'
import { DesktopShellKey } from '@/lib/desktop-shell'

const desktopShell = inject(DesktopShellKey, false)
const macTopInset = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)

const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const selectionStore = useChatSelectionStore()
const { currentBotId } = storeToRefs(selectionStore)
const chatStore = useChatStore()
const { bots } = storeToRefs(chatStore)

const backToChatRoute = computed(() => {
  const botId = (currentBotId.value ?? '').trim()
  if (!botId) return { name: 'home' as const }
  const botName = bots.value.find((b) => b.id === botId)?.name ?? botId
  return { name: 'bot' as const, params: { botName } }
})

const { data: bot } = useQuery({
  key: () => ['bot', route.params.botName as string],
  query: async () => {
    const { data } = await getBotsById({
      path: { id: route.params.botName as string },
      throwOnError: true,
    })
    return data
  },
  enabled: () => route.name === 'bot-detail' && !!route.params.botName,
})

const breadcrumbs = computed(() => {
  const items = []
  const matched = route.matched
  for (const m of matched) {
    if (m.meta && m.meta.breadcrumb) {
      let label = ''
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
