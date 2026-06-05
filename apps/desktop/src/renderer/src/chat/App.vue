<script setup lang="ts">
import { onBeforeUnmount, onMounted, provide } from 'vue'
import { RouterView, useRouter } from 'vue-router'
import { Toaster } from '@memohai/ui'
import 'vue-sonner/style.css'
import { useSettingsStore } from '@memohai/web/store/settings'
import { DesktopShellKey } from '@memohai/web/lib/desktop-shell'

provide(DesktopShellKey, true)
useSettingsStore()

// Dev-only: toggle the component wall / design-token reference with
// Cmd/Ctrl+Shift+D. No-op (and not registered) in production builds.
const router = useRouter()
function onDevKey(e: KeyboardEvent) {
  if (!import.meta.env.DEV) return
  if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'D' || e.key === 'd')) {
    e.preventDefault()
    const onWall = router.currentRoute.value.path.startsWith('/dev/')
    void router.push(onWall ? '/' : '/dev/components')
  }
}
onMounted(() => {
  if (import.meta.env.DEV) window.addEventListener('keydown', onDevKey)
})
onBeforeUnmount(() => window.removeEventListener('keydown', onDevKey))
</script>

<template>
  <section>
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>
