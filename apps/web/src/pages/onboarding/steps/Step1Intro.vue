<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="text-center">
    <h2 class="text-xl font-semibold mb-4">
      {{ t('onboarding.intro.title') }}
    </h2>
    <p class="text-muted-foreground text-sm">
      {{ t('onboarding.intro.placeholder') }}
    </p>
  </div>
</template>
