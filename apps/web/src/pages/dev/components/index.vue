<script setup lang="ts">
// Dev-only component wall. Reached via the `memoh:dev-tools` localStorage flag
// (web) or Cmd/Ctrl+Shift+D (desktop). Renders every @felinic/ui component and
// design token in one place so tokens/components can be tuned via Vite HMR.
//
// Owns its own scroll container (h-dvh + overflow-y-auto) so it scrolls inside
// the desktop shell, which locks body overflow.
import { onMounted, provide, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '@/store/settings'
import WallToolbar from './components/WallToolbar.vue'
import WallNav from './components/WallNav.vue'
import { wallSections } from './lib/registry'

// Bumped whenever theme/scheme changes so token swatches re-read their
// resolved CSS values. Injected by TokenSwatch.
const themeVersion = ref(0)
provide('wallThemeVersion', themeVersion)

const settings = useSettingsStore()
const { theme, colorScheme } = storeToRefs(settings)
watch([theme, colorScheme], () => {
  themeVersion.value++
})

onMounted(() => {
   
  console.info('[dev] component wall — Cmd/Ctrl+Shift+D to toggle (desktop)')
})
</script>

<template>
  <div class="h-dvh overflow-y-auto bg-background text-foreground">
    <WallToolbar />

    <div class="mx-auto flex w-full max-w-[1600px] gap-8 px-4 py-6 md:px-6">
      <aside class="hidden w-44 shrink-0 lg:block">
        <WallNav
          class="sticky top-20"
          :sections="wallSections"
        />
      </aside>

      <main class="flex min-w-0 flex-1 flex-col gap-12">
        <component
          :is="section.component"
          v-for="section in wallSections"
          :key="section.id"
        />
      </main>
    </div>
  </div>
</template>
