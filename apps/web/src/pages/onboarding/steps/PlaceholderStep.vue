<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useOnboarding } from '@/composables/useOnboarding'

const { t } = useI18n()
const { nextStep, prevStep } = useOnboarding()

defineProps<{
  titleKey: string
  descKey: string
}>()
</script>

<template>
  <div class="text-left min-h-[542px] flex flex-col">
    <h2 class="text-3xl font-semibold mb-4">
      {{ t(titleKey) }}
    </h2>
    <p class="text-muted-foreground text-sm">
      {{ t(descKey) }}
    </p>

    <div class="mt-auto pt-12 flex items-center justify-end gap-3">
      <button
        class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        @click="prevStep"
      >
        {{ t('onboarding.prev') }}
      </button>
      <button
        class="inline-flex h-[42px] w-[240px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        @click="nextStep"
      >
        {{ t('onboarding.next') }}
      </button>
    </div>
  </div>
</template>
