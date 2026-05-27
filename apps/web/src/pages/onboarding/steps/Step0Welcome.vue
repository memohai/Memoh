<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="text-center">
    <h1 class="text-2xl font-bold mb-3">
      {{ t('onboarding.welcome.title') }}
    </h1>
    <p class="text-muted-foreground text-sm">
      {{ t('onboarding.welcome.description') }}
    </p>
  </div>
</template>
