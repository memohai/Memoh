<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="text-center">
    <h2 class="text-xl font-semibold mb-3">
      {{ t('onboarding.complete.title') }}
    </h2>
    <p class="text-muted-foreground text-sm mb-2">
      {{ t('onboarding.complete.subtitle') }}
    </p>
    <p class="text-muted-foreground text-xs">
      {{ t('onboarding.complete.hint') }}
    </p>
  </div>
</template>
