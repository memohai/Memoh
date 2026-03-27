<template>
  <aside>
    <Sidebar collapsible="icon">
      <SidebarHeader class="p-0 border-0">
        <button
          class="h-[53px] flex items-center gap-2.5 px-3.5 w-full border-b border-border text-foreground hover:bg-accent/50 transition-colors group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          @click="router.push({ name: 'home' })"
        >
          <FontAwesomeIcon
            :icon="['fas', 'chevron-left']"
            class="size-3 shrink-0"
          />
          <span class="text-xs font-semibold group-data-[collapsible=icon]:hidden">
            {{ t('sidebar.settings') }}
          </span>
        </button>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup class="px-2 py-2.5">
          <SidebarGroupContent>
            <SidebarMenu class="gap-0.5">
              <SidebarMenuItem
                v-for="item in navItems"
                :key="item.name"
              >
                <SidebarMenuButton
                  :tooltip="item.title"
                  :is-active="isItemActive(item.name)"
                  :aria-current="isItemActive(item.name) ? 'page' : undefined"
                  class="h-9 gap-2 text-muted-foreground relative before:absolute before:w-0.5 before:top-1.5 before:bottom-1.5 before:left-0 before:rounded-full data-[active=true]:before:bg-[#8B56E3]"
                  @click="router.push({ name: item.name })"
                >
                  <FontAwesomeIcon
                    :icon="item.icon"
                    class="size-3.5 ml-1.5"
                  />
                  <span class="text-xs font-medium">{{ item.title }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@memohai/ui'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()

function isItemActive(name: string): boolean {
  if (name === 'bots') {
    return route.path.startsWith('/settings/bots')
  }
  return route.name === name
}

const navItems = computed(() => [
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
