<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useOnboarding } from '@/composables/useOnboarding'

const { t } = useI18n()
const { complete, completing } = useOnboarding()
</script>

<template>
  <div class="text-center">
    <h2 class="text-3xl font-semibold mb-3">
      {{ t('onboarding.complete.title') }}
    </h2>
    <p class="text-muted-foreground text-sm mb-2">
      {{ t('onboarding.complete.subtitle') }}
    </p>
    <p class="text-muted-foreground text-xs mb-10">
      {{ t('onboarding.complete.hint') }}
    </p>
    <button
      class="inline-flex h-[42px] w-[240px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      :disabled="completing"
      @click="complete"
    >
      {{ t('onboarding.complete.action') }}
    </button>
  </div>
</template>
