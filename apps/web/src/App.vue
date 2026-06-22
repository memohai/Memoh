<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterView, useRoute } from 'vue-router'
import { Toaster } from '@memohai/ui'
import { useSettingsStore } from '@/store/settings'
import MainSection from '@/pages/main-section/index.vue'

useSettingsStore()

const { t } = useI18n()
const route = useRoute()

// The chat workspace (MainSection: sidebar + dockview) is mounted persistently
// here rather than as a route component. This keeps the chat DOM attached and
// full-size while the user is in settings, so dockview never relayouts from a
// zero size, the message list keeps its scroll position, and returning has no
// black flash / no re-scroll.
// (KeepAlive can't achieve this: it detaches the subtree, which is what caused
// all three regressions.)
//   - chat routes (home/bot): only MainSection shows.
//   - settings routes: MainSection stays mounted, settings content renders above it.
//   - auth-boundary routes (login/onboarding/oauth/dev): MainSection is not
//     mounted at all; those render alone via RouterView.
const isChatRoute = computed(() => route.name === 'home' || route.name === 'bot')
const isSettingsRoute = computed(() => route.path.startsWith('/settings'))
const isAppArea = computed(() => isChatRoute.value || isSettingsRoute.value)

// Localized headings for auto-shaped long-blob toasts (raw backend errors etc.).
// @memohai/ui is i18n-agnostic, so the app supplies these; they react to locale.
const toastHeadings = computed(() => ({
  message: t('common.toast.notice'),
  success: t('common.toast.success'),
  error: t('common.toast.error'),
  warning: t('common.toast.warning'),
  info: t('common.toast.info'),
}))
</script>

<template>
  <section>
    <!-- Persistent chat area. Stays mounted (and full-size) across chat↔settings
         so its dockview/scroll survive untouched. Unmounts only when leaving the
         authenticated app area (login/onboarding). -->
    <MainSection v-if="isAppArea" />

    <!-- Single RouterView (v-slot) drives both the settings section and the
         auth-boundary pages. On chat routes the matched component is a null stub,
         so `Component` renders nothing. -->
    <RouterView v-slot="{ Component }">
      <!-- Settings section: a permanent fixed full-screen positioning layer
           (always in the DOM + compositor layer tree), toggled with `visibility`
           only — never v-if, never opacity. Why each matters:
           (1) v-if'ing the layer tore down a promoted compositor layer in one
               frame → Chromium recomposited chat underneath with a gap (flash).
           (2) The wrapper is TRANSPARENT (no bg). settings-section paints its own
               opaque bg-background, so while it slides/fades in or out the layer
               behind it is the live chat — never a black backdrop. Earlier the
               wrapper itself was near-black, so any fade/leave revealed a dark
               wash. Transparent + visibility flip = clean, no flash, no fade. -->
      <div
        class="fixed inset-0 z-40"
        :class="isSettingsRoute ? 'visible' : 'pointer-events-none invisible'"
      >
        <component
          :is="Component"
          v-if="isSettingsRoute"
        />
      </div>
      <!-- Auth-boundary routes (login/onboarding/oauth/dev) render full-screen on
           their own; MainSection and the settings content are both absent. -->
      <component
        :is="Component"
        v-if="!isAppArea"
      />
    </RouterView>

    <Toaster
      position="top-right"
      :headings="toastHeadings"
    />
  </section>
</template>
