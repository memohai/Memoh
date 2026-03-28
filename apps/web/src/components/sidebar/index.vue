<template>
  <aside>
    <Sidebar collapsible="icon">
      <SidebarHeader class="p-0 border-0">
        <div class="h-1.5 group-data-[collapsible=icon]:hidden" />
        <div class="h-[38px] flex items-center group-data-[collapsible=icon]:justify-center">
          <div class="flex items-center gap-1 ml-3 group-data-[collapsible=icon]:hidden">
            <span class="text-[10px] font-medium text-muted-foreground uppercase tracking-[0.7px]">
              {{ t('sidebar.bots') }}
            </span>
          </div>
          <Button
            variant="ghost"
            size="icon"
            class="ml-auto mr-1.5 size-6 text-muted-foreground hover:text-foreground group-data-[collapsible=icon]:ml-0 group-data-[collapsible=icon]:mr-0"
            :aria-label="t('bots.createBot')"
            @click="router.push({ name: 'bots' })"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="size-3.5"
            />
          </Button>
        </div>
      </SidebarHeader>

      <SidebarContent>
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
              <FontAwesomeIcon
                :icon="['fas', 'spinner']"
                class="size-4 animate-spin text-muted-foreground"
              />
            </div>
            <div
              v-if="!isLoading && bots.length === 0"
              class="px-3 py-6 text-center text-xs text-muted-foreground"
            >
              {{ t('bots.emptyTitle') }}
            </div>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter class="relative border-0 px-2 pb-3.5 pt-2.5">
        <div class="pointer-events-none absolute -top-[120px] left-0 h-[153px] w-full bg-linear-to-t from-(--sidebar-background) from-18% to-transparent z-10 group-data-[collapsible=icon]:hidden" />
        <SidebarMenu class="gap-2.5">
          <!-- <SidebarMenuItem>
            <SidebarMenuButton
              :tooltip="displayTitle"
              class="h-10 px-2.5"
              @click="router.push({ name: 'settings' })"
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
              class="h-9 px-2.5"
              :is-active="isSettingsActive"
              @click="router.push('/settings')"
            >
              <FontAwesomeIcon
                :icon="['fas', 'gear']"
                class="size-3.5"
              />
              <span class="text-xs font-medium">{{ t('sidebar.settings') }}</span>
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
} from '@memohai/ui'
import BotItem from './bot-item.vue'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
// const { userInfo } = useUserStore()

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

const isSettingsActive = computed(() => route.path.startsWith('/settings'))

// const displayTitle = computed(() =>
//   userInfo.displayName || userInfo.username || userInfo.id || t('settings.user'),
// )
// const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')
</script>
