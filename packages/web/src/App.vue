<script setup lang="ts">
import { RouterView } from 'vue-router'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Toaster,
  Separator
} from '@memoh/ui'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiTranslate, mdiBrightness6 } from '@mdi/js'
import { useColorMode } from '@vueuse/core'
// @ts-ignore
import 'vue-sonner/style.css'

const mode = useColorMode()
const modeToggleMap: Record<'dark' | 'light', 'dark' | 'light'> = {
  dark: 'light',
  light: 'dark'
}
const toggleMode = () => {
  if (mode.value !== 'auto') {
    mode.value = modeToggleMap[mode.value]
  }

}
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <div
      class="fixed top-0 flex right-8 z-9999 [&:is(:has([data-state=open]))_.translate-icon]:opacity-100 align h-16 items-center"
    >
      <DropdownMenu>
        <DropdownMenuTrigger class="ml-auto mr-4 cursor-pointer">
          <svg-icon
            type="mdi"
            :path="mdiTranslate"
            class="translate-icon opacity-30 hover:opacity-100"
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem @click="$i18n.locale = 'zh'">
            中文
          </DropdownMenuItem>
          <DropdownMenuItem @click="$i18n.locale = 'en'">
            English
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <svg-icon
        type="mdi"
        :path="mdiBrightness6"
        class="translate-icon opacity-30 hover:opacity-100 cursor-pointer"
        @click="toggleMode"
      />
    </div>
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>