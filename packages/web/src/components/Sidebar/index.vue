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
                    :class="` border border-transparent w-full flex justify-start ${curSlider === sidebarItem.name ? 'border-inherit' : ''}`"
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
    </Sidebar>
  </aside>
</template>
<script setup lang="ts">
import {
  Sidebar,
  SidebarContent,
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
import i18n from '@/i18n'
import { ref } from 'vue'


const router = useRouter()
const route = useRoute()

const { t } = i18n.global
const curSlider = ref()
const curSelectSlide = (cur: string) => computed(() => {
  return curSlider.value === cur || new RegExp(`^/main/${cur}$`).test(route.path)
})
const sidebarInfo = computed(() => [
  {
    title: t('sidebar.chat'),
    name: 'chat',
    icon: ['far', 'comments'],
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

</script>