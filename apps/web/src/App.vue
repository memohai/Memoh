<script setup lang="ts">
import { RouterView } from 'vue-router'
import { Toaster } from '@memohai/ui'
import { useSettingsStore } from '@/store/settings'

useSettingsStore()
</script>

<template>
  <section>
    <RouterView />
    <Toaster position="top-right" />
  </section>
</template>
