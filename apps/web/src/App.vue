<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterView } from 'vue-router'
import { Toaster } from '@memohai/ui'
import { useSettingsStore } from '@/store/settings'

useSettingsStore()

const { t } = useI18n()

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
    <RouterView />
    <Toaster
      position="top-right"
      :headings="toastHeadings"
    />
  </section>
</template>
