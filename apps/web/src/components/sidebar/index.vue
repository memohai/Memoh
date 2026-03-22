<template>
  <aside aria-label="Primary">
    <Sidebar
      collapsible="icon"
      role="navigation"
      aria-label="Primary"
    >
      <SidebarHeader class="h-[53px] justify-center px-3.5">
        <div class="flex items-center gap-2.5 group-data-[collapsible=icon]:justify-center">
          <img
            src="/logo.png"
            class="size-[22px] shrink-0"
            alt="Memoh logo"
          >
          <span class="text-sm font-semibold text-foreground truncate group-data-[collapsible=icon]:hidden">
            Memoh
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem
                v-for="sidebarItem in sidebarInfo"
                :key="sidebarItem.title"
              >
                <SidebarMenuButton
                  :tooltip="sidebarItem.title"
                  :is-active="isItemActive(sidebarItem.name)"
                  :aria-current="isItemActive(sidebarItem.name) ? 'page' : undefined"
                  class="text-muted-foreground"
                  @click="router.push({ name: sidebarItem.name })"
                >
                  <FontAwesomeIcon :icon="sidebarItem.icon" />
                  <span>{{ sidebarItem.title }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter class="border-t p-2">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              :tooltip="displayTitle"
              class="text-muted-foreground"
              @click="router.push({ name: 'settings' })"
            >
              <Avatar class="size-4 shrink-0">
                <AvatarImage
                  v-if="userInfo.avatarUrl"
                  :src="userInfo.avatarUrl"
                  :alt="displayTitle"
                />
                <AvatarFallback class="text-[7px] text-muted-foreground">
                  {{ avatarFallback }}
                </AvatarFallback>
              </Avatar>
              <span class="truncate text-sm">{{ displayNameLabel }}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@memoh/ui'
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@/store/user'
import { useAvatarInitials } from '@/composables/useAvatarInitials'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const { userInfo } = useUserStore()

const displayNameLabel = computed(() =>
  userInfo.displayName || userInfo.username || userInfo.id || '-',
)
const displayTitle = computed(() =>
  userInfo.displayName || userInfo.username || userInfo.id || t('settings.user'),
)
const avatarFallback = useAvatarInitials(() => displayTitle.value, 'U')

function isItemActive(name: string): boolean {
  return new RegExp(`^/${name}(\\b|/)`).test(route.path)
}

const sidebarInfo = computed(() => [
  {
    title: t('sidebar.chat'),
    name: 'chat',
    icon: ['fas', 'comment-dots'],
  },
  {
    title: t('sidebar.bots'),
    name: 'bots',
    icon: ['fas', 'robot'],
  },
  {
    title: t('sidebar.models'),
    name: 'models',
    icon: ['fas', 'cubes'],
  },
  {
    title: t('sidebar.searchProvider'),
    name: 'search-providers',
    icon: ['fas', 'globe'],
  },
  {
    title: t('sidebar.memoryProvider'),
    name: 'memory-providers',
    icon: ['fas', 'brain'],
  },
  {
    title: t('sidebar.ttsProvider'),
    name: 'tts-providers',
    icon: ['fas', 'volume-high'],
  },
  {
    title: t('sidebar.emailProvider'),
    name: 'email-providers',
    icon: ['fas', 'envelope'],
  },
  {
    title: t('sidebar.browserContexts'),
    name: 'browser-contexts',
    icon: ['fas', 'window-maximize'],
  },
  {
    title: t('sidebar.usage'),
    name: 'usage',
    icon: ['fas', 'chart-line'],
  },
  {
    title: t('sidebar.settings'),
    name: 'settings',
    icon: ['fas', 'gear'],
  },
])
</script>
