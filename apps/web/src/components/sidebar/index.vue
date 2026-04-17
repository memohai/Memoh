<template>
  <aside>
    <Sidebar collapsible="icon">
      <SidebarHeader class="p-0 border-0">
        <div class="h-10 flex items-center pl-2 group-data-[collapsible=icon]:pl-3 transition-[padding] duration-200 ease-linear">
          <Button
            variant="ghost"
            size="icon"
            class="size-6 text-muted-foreground hover:text-foreground shrink-0"
            aria-label="Toggle Sidebar"
            @click="toggleSidebar"
          >
            <PanelLeftClose class="size-3.5 group-data-[collapsible=icon]:hidden" />
            <PanelLeftOpen class="size-3.5 hidden group-data-[collapsible=icon]:block" />
          </Button>
          
          <div class="ml-auto mr-1.5 group-data-[collapsible=icon]:hidden">
            <Button
              variant="ghost"
              size="icon"
              class="size-6 text-muted-foreground hover:text-foreground shrink-0"
              :aria-label="t('bots.createBot')"
              @click="router.push({ name: 'bots' })"
            >
              <Plus class="size-3.5" />
            </Button>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent class="@container/bots">
        <SidebarGroup class="px-2 py-0">
          <SidebarGroupContent>
            <SidebarMenu class="gap-1">
              <SidebarMenuItem
                v-for="bot in bots"
                :key="bot.id"
              >
                <BotItem :bot="bot" />
              </SidebarMenuItem>
            </SidebarMenu>

            <div
              v-if="isLoading"
              class="flex justify-center py-4"
            >
              <LoaderCircle
                class="size-4 animate-spin text-muted-foreground"
              />
            </div>
            <div
              v-if="!isLoading && bots.length === 0"
              class="px-3 py-6 text-center text-xs text-muted-foreground @max-[50px]/bots:hidden"
            >
              {{ t('bots.emptyTitle') }}
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarRail />

      <SidebarFooter class="relative border-0 px-2 pb-3.5 pt-2.5">
        <div class="pointer-events-none absolute -top-30 left-0 h-38.25 w-full bg-linear-to-t from-(--sidebar-background) from-18% to-transparent z-10 group-data-[collapsible=icon]:hidden" />
        <SidebarMenu class="gap-2.5">
          <!-- <SidebarMenuItem>
            <SidebarMenuButton
              :tooltip="displayTitle"
              class="h-10 px-2.5"
              @click="router.push({ name: 'profile' })"
            >
              <div class="size-9 shrink-0 rounded-full border border-border bg-accent overflow-hidden p-[1.385px]">
                <img
                  v-if="userInfo.avatarUrl"
                  :src="userInfo.avatarUrl"
                  :alt="displayTitle"
                  class="size-full rounded-full object-cover"
                >
                <span
                  v-else
                  class="size-full flex items-center justify-center text-[10px] font-medium text-muted-foreground"
                >
                  {{ avatarFallback }}
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem> -->
          <SidebarMenuItem>
            <SidebarMenuButton
              :tooltip="t('sidebar.settings')"
              class="h-9 px-2.5 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
              :is-active="isSettingsActive"
              @click="router.push('/settings')"
            >
              <Settings
                class="size-3.5"
              />
              <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ t('sidebar.settings') }}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
// import { useUserStore } from '@/store/user'
// import { useAvatarInitials } from '@/composables/useAvatarInitials'
import {
  Button,
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@memohai/ui'
import { Plus, LoaderCircle, Settings, PanelLeftClose, PanelLeftOpen } from 'lucide-vue-next'
import BotItem from './bot-item.vue'
import { usePinnedBots } from '@/composables/usePinnedBots'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const { toggleSidebar } = useSidebar()
// const { userInfo } = useUserStore()
const { sortBots } = usePinnedBots()

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => sortBots(botData.value?.items ?? []))

const isSettingsActive = computed(() => route.path.startsWith('/settings'))

// const displayTitle = computed(() =>
//   userInfo.displayName || userInfo.username || userInfo.id || t('settings.user'),
// )
// const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')
</script>
