<script setup lang="ts">
import { RouterView } from 'vue-router'
import { Toaster } from '@memohai/ui'
import 'vue-sonner/style.css'
import { useSettingsStore } from '@/store/settings'

useSettingsStore()
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>
