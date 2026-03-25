<template>
  <aside>
    <Sidebar collapsible="icon">
      <SidebarHeader class=" h-12">
        <section class="my-auto px-2 flex gap-2 font-extrabold group-data-[collapsible=icon]:px-1">
          <img
            src="/logo.png"
            class="w-6! object-fit "
            alt="Memoh logo"
          >
          <h1 class="font-semibold group-data-[collapsible=icon]:hidden">
            Memoh
          </h1>
        </section>
        <!-- <div class="flex items-center gap-2  group-data-[collapsible=icon]:justify-center">
        
          <span class="text-lg font-bold text-muted-foreground truncate group-data-[collapsible=icon]:hidden">
            Memoh
          </span>
        </div> -->
      </SidebarHeader>
      <Separator />
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
                  class="py-5 text-muted-foreground relative before:absolute before:w-0.5! before:top-2 before:bottom-2 data-[active=true]:before:bg-[#8B56E3] hover:before:bg-[#8B56E3] before:left-0.5!"
                  @click="router.push({ name: sidebarItem.name })"
                >
                  <FontAwesomeIcon
                    class="ml-1"
                    :icon="sidebarItem.icon"
                  />
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
  Separator
} from '@memohai/ui'
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