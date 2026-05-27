<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@memohai/ui'
import { useOnboarding } from '@/composables/useOnboarding'

import Step0Welcome from './steps/Step0Welcome.vue'
import Step1Intro from './steps/Step1Intro.vue'
import Step2Appearance from './steps/Step2Appearance.vue'
import Step3Provider from './steps/Step3Provider.vue'
import Step4BotCreate from './steps/Step4BotCreate.vue'
import Step5IM from './steps/Step5IM.vue'
import Step6Complete from './steps/Step6Complete.vue'

const { t } = useI18n()
const route = useRoute()
const {
  currentStep,
  completing,
  isFirstStep,
  isLastStep,
  nextStep,
  prevStep,
  goToStep,
  complete,
  detectAndApplyLocale,
} = useOnboarding()

const stepComponents = [
  Step0Welcome,
  Step1Intro,
  Step2Appearance,
  Step3Provider,
  Step4BotCreate,
  Step5IM,
  Step6Complete,
]

onMounted(() => {
  detectAndApplyLocale()
  const stepParam = route.query.step
  if (stepParam !== undefined) {
    const step = Number(stepParam)
    if (!isNaN(step) && step >= 0 && step <= 6) {
      goToStep(step)
    }
  }
})

watch(currentStep, (step) => {
  if (route.query.step !== String(step)) {
    window.history.replaceState(null, '', `?step=${step}`)
  }
})
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-background p-4">
    <div class="w-full max-w-lg">
      <!-- Step indicator -->
      <div class="flex items-center justify-center gap-2 mb-8">
        <div
          v-for="i in 7"
          :key="i"
          class="h-1.5 rounded-full transition-all duration-300"
          :class="i - 1 === currentStep
            ? 'w-6 bg-primary'
            : i - 1 < currentStep
              ? 'w-1.5 bg-primary/60'
              : 'w-1.5 bg-muted-foreground/30'"
        />
      </div>

      <!-- Step content -->
      <div class="mb-8">
        <component :is="stepComponents[currentStep]" />
      </div>

      <!-- Navigation -->
      <div class="flex items-center justify-between">
        <Button
          v-if="!isFirstStep"
          variant="outline"
          @click="prevStep"
        >
          {{ t('onboarding.prev') }}
        </Button>
        <div v-else />

        <div class="flex items-center gap-3">
          <Button
            v-if="!isLastStep"
            variant="ghost"
            class="text-muted-foreground"
            @click="nextStep"
          >
            {{ t('onboarding.skip') }}
          </Button>
          <Button
            v-if="!isLastStep"
            @click="nextStep"
          >
            {{ t('onboarding.next') }}
          </Button>
          <Button
            v-else
            :disabled="completing"
            @click="complete"
          >
            {{ t('onboarding.complete') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>
