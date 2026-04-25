<script setup lang="ts">
import { provide } from 'vue'
import { RouterView } from 'vue-router'
import { Toaster } from '@memohai/ui'
import 'vue-sonner/style.css'
import { useSettingsStore } from '@memohai/web/store/settings'
import { DesktopShellKey } from '@memohai/web/lib/desktop-shell'

provide(DesktopShellKey, true)
useSettingsStore()
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>
