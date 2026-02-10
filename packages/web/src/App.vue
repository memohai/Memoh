<script setup lang="ts">
import { RouterView } from 'vue-router'
import { Toaster } from '@memoh/ui'
import 'vue-sonner/style.css'
import { useSettingsStore } from '@/store/settings'

// 初始化设置（主题、语言），确保在任何页面进入时都已应用
useSettingsStore()
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>
