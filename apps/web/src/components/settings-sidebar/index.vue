<template>
  <aside class="relative h-full">
    <header
      v-if="topInset"
      class="fixed top-0 left-0 z-20 h-9 w-(--sidebar-width) bg-sidebar border-r border-sidebar-border [-webkit-app-region:drag]"
    />

    <Sidebar
      :collapsible="topInset ? 'none' : 'icon'"
      :class="topInset ? 'pt-9 h-dvh border-r border-sidebar-border' : ''"
    >
      <SidebarHeader
        v-if="!hideHeader"
        class="p-0 border-0"
      >
        <button
          class="h-[53px] flex items-center gap-2.5 px-3.5 w-full border-b border-border text-foreground hover:bg-accent/50 transition-colors group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          @click="router.push(backToChatRoute)"
        >
          <ChevronLeft
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
                  class="h-9 gap-2 relative before:absolute before:w-0.5 before:top-1.5 before:bottom-1.5 before:left-0 before:rounded-full data-[active=true]:before:bg-sidebar-primary group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
                  @click="router.push({ name: item.name })"
                >
                  <component
                    :is="item.icon"
                    class="size-3.5 ml-1.5 group-data-[collapsible=icon]:ml-0"
                  />
                  <span class="text-xs font-medium group-data-[collapsible=icon]:hidden">{{ item.title }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarRail v-if="!topInset" />
    </Sidebar>
  </aside>
</template>

<script setup lang="ts">
import { computed, inject, type Component } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, Bot, Boxes, Globe, Brain, Volume2, AudioLines, Mail, ChartLine, User, Store, Info, Palette } from 'lucide-vue-next'
import { useChatSelectionStore } from '@/store/chat-selection'
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@memohai/ui'
import { DesktopShellKey } from '@/lib/desktop-shell'

const props = withDefaults(defineProps<{
  hideHeader?: boolean
  excludeItems?: string[]
}>(), {
  hideHeader: false,
  excludeItems: () => [],
})

const topInset = inject(DesktopShellKey, false)

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const selectionStore = useChatSelectionStore()
const { currentBotId } = storeToRefs(selectionStore)

const backToChatRoute = computed(() => {
  const botId = (currentBotId.value ?? '').trim()
  if (!botId) return { name: 'home' as const }
  return {
    name: 'chat' as const,
    params: { botId },
  }
})

function isItemActive(name: string): boolean {
  if (name === 'bots') {
    return route.path.startsWith('/settings/bots')
  }
  return route.name === name
}

const allNavItems: { title: string; name: string; icon: Component }[] = [
  { title: t('sidebar.bots'), name: 'bots', icon: Bot },
  { title: t('sidebar.providers'), name: 'providers', icon: Boxes },
  { title: t('sidebar.webSearch'), name: 'web-search', icon: Globe },
  { title: t('sidebar.memory'), name: 'memory', icon: Brain },
  { title: t('sidebar.speech'), name: 'speech', icon: Volume2 },
  { title: t('sidebar.transcription'), name: 'transcription', icon: AudioLines },
  { title: t('sidebar.email'), name: 'email', icon: Mail },
  { title: t('sidebar.supermarket'), name: 'supermarket', icon: Store },
  { title: t('sidebar.usage'), name: 'usage', icon: ChartLine },
  { title: t('sidebar.appearance'), name: 'appearance', icon: Palette },
  { title: t('sidebar.profile'), name: 'profile', icon: User },
  { title: t('sidebar.about'), name: 'about', icon: Info },
]

const navItems = computed(() =>
  props.excludeItems.length > 0
    ? allNavItems.filter(item => !props.excludeItems.includes(item.name))
    : allNavItems,
)
</script>
