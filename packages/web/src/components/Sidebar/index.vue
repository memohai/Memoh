<template>
  <aside class="[&_[data-state=collapsed]_:is(.title-container,.exist-btn)]:hidden">
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <img
              src="../../../public/logo.png"
              width="80"
              class="m-auto"
              alt="logo.png"
            >
            <h4
              class="scroll-m-20 text-xl font-semibold tracking-tight text-center text-muted-foreground title-container"
            >
              Memoh
            </h4>
          </SidebarMenuItem>
        </SidebarMenu>
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
                    <svg-icon
                      type="mdi"
                      :path="sidebarItem.icon"
                    />
                    <span>{{ sidebarItem.title }}</span>
                  </Toggle>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem class="flex justify-center exist-btn">
            <Button
              class="flex-[0.7] mb-10"
              @click="exit"
            >
              {{ $t("login.exit") }}
            </Button>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarRail />
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
  SidebarRail,
  CollapsibleTrigger,
  Collapsible,
  Button,
  Toggle
} from '@memoh/ui'
import { computed } from 'vue'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiRobot, mdiChatOutline, mdiCogBox } from '@mdi/js'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/store/User.ts'
import i18n from '@/i18n'
import { ref } from 'vue'

const router = useRouter()

const { t } = i18n.global
const curSlider = ref('chat')
const curSelectSlide = (cur: string) => computed(() => {
  return curSlider.value === cur
})
const sidebarInfo = computed(() => [
  {
    title: t('slidebar.chat'),
    name: 'chat',
    icon: mdiChatOutline
  },
  // {
  //   title: t('slidebar.home'),
  //   name: 'home',
  //   icon: mdiHome
  // },
  {
    title: t('slidebar.model_setting'),
    name: 'models',
    icon: mdiRobot
  }, {
    title: t('slidebar.setting'),
    name: 'settings',
    icon: mdiCogBox
  },
  // {
  //   title: 'MCP',
  //   name: 'mcp',
  //   icon: mdiListBox
  // }, {
  //   title: t('slidebar.platform'),
  //   name: 'platform',
  //   icon: mdiBookArrowDown
  // }
])

const { exitLogin } = useUserStore()
const exit = () => {
  exitLogin()
  router.replace({ name: 'Login' })
}
</script>