<template>
  <aside
    aria-label="Primary"
    class="[&_[data-state=collapsed]_:is(.title-container,.exist-btn)]:hidden"
  >
    <Sidebar
      collapsible="icon"
      role="navigation"
      aria-label="Primary"
    >
      <SidebarHeader class="group-data-[state=collapsed]:hidden">
        <div class="flex items-center gap-2 px-3 py-2">
          <img
            src="/logo.png"
            class="size-8"
            alt="Memoh logo"
          >
          <span class="text-xl font-bold text-gray-500 dark:text-gray-400">
            Memoh
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent class="[&_ul+ul]:mt-2!">
            <SidebarMenu
              v-for="sidebarItem in sidebarInfo"
              :key="sidebarItem.title"
            >
              <SidebarMenuItem class="[&_[aria-pressed=true]]:bg-accent!">
                <SidebarMenuButton
                  as-child
                  class="justify-start py-5! px-4"
                  :tooltip="sidebarItem.title"
                >
                  <Toggle
                    class="border border-transparent w-full flex justify-start"
                    :class="{ 'border-inherit': isActive(sidebarItem.name as string) }"
                    :model-value="isActive(sidebarItem.name as string)"
                    :aria-current="isActive(sidebarItem.name as string) ? 'page' : undefined"
                    @click="router.push({ name: sidebarItem.name })"
                  >
                    <FontAwesomeIcon :icon="sidebarItem.icon" />
                    <span>{{ sidebarItem.title }}</span>
                  </Toggle>
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
              class="justify-start px-2 py-2"
              :tooltip="displayTitle"
              @click="router.push({ name: 'settings' })"
            >
              <Avatar class="size-7 shrink-0">
                <AvatarImage
                  v-if="userInfo.avatarUrl"
                  :src="userInfo.avatarUrl"
                  :alt="displayTitle"
                />
                <AvatarFallback class="text-[10px] text-gray-600 dark:text-gray-300">
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
  Toggle,
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

function isActive(name: string) {
  return route.name === name || route.path === `/${name}`
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
    title: t('sidebar.settings'),
    name: 'settings',
    icon: ['fas', 'gear'],
  },
])

</script>
