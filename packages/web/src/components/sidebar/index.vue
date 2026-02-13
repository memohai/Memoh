<template>
  <aside class="[&_[data-state=collapsed]_:is(.title-container,.exist-btn)]:hidden">
    <Sidebar collapsible="icon">
      <SidebarHeader class="group-data-[state=collapsed]:hidden">
        <div class="flex items-center gap-2 px-3 py-2">
          <img
            src="/logo.png"
            class="size-8"
            alt="logo"
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
                    :class="`border border-transparent w-full flex justify-start ${curSlider === sidebarItem.name ? 'border-inherit' : ''}`"
                    :model-value="curSelectSlide(sidebarItem.name as string).value"
                    @update:model-value="(isSelect) => {
                      if (isSelect) {
                        curSlider = sidebarItem.name
                      }
                    }"
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
              @click="onUserAction"
            >
              <Avatar class="size-7 shrink-0">
                <AvatarImage
                  v-if="userInfo.avatarUrl"
                  :src="userInfo.avatarUrl"
                  :alt="displayTitle"
                />
                <AvatarFallback class="text-[10px]">
                  {{ avatarFallback }}
                </AvatarFallback>
              </Avatar>
              <span class="truncate text-sm">{{ displayNameLabel }}</span>
              <FontAwesomeIcon
                :icon="['fas', 'gear']"
                class="ml-auto size-3.5 text-muted-foreground"
              />
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
import { computed, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@/store/user'

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
const avatarFallback = computed(() =>
  displayTitle.value.slice(0, 2).toUpperCase() || 'U',
)

const curSlider = ref()
const curSelectSlide = (cur: string) => computed(() => {
  return curSlider.value === cur || new RegExp(`^/${cur}$`).test(route.path)
})

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
    title: t('sidebar.settings'),
    name: 'settings',
    icon: ['fas', 'gear'],
  },
])

function onUserAction() {
  if (route.name === 'settings-user') {
    void router.push({ name: 'settings' }).catch(() => undefined)
  } else {
    void router.push({ name: 'settings-user' }).catch(() => undefined)
  }
}
</script>
