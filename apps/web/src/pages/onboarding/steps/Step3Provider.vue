<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="text-center">
    <h2 class="text-xl font-semibold mb-4">
      {{ t('onboarding.provider.title') }}
    </h2>
    <p class="text-muted-foreground text-sm">
      {{ t('onboarding.provider.placeholder') }}
    </p>
  </div>
</template>
