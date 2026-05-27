<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@memohai/ui'
import { useOnboarding, STEP_COUNT } from '@/composables/useOnboarding'

import PlaceholderStep from './steps/PlaceholderStep.vue'
import Step2Appearance from './steps/Step2Appearance.vue'
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
  skipToEnd,
  complete,
} = useOnboarding()

const stepComponents = [
  { component: PlaceholderStep, props: { heading: true, titleKey: 'onboarding.welcome.title', descKey: 'onboarding.welcome.description' } },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.intro.title', descKey: 'onboarding.intro.placeholder' } },
  { component: Step2Appearance, props: {} },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.provider.title', descKey: 'onboarding.provider.placeholder' } },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.bot.title', descKey: 'onboarding.bot.placeholder' } },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.im.title', descKey: 'onboarding.im.placeholder' } },
  { component: Step6Complete, props: {} },
]

function readStepFromQuery(): number | null {
  const raw = route.query.step
  if (raw === undefined || raw === '') return null
  const step = Number.parseInt(String(raw), 10)
  if (!Number.isInteger(step) || step < 0 || step >= STEP_COUNT) return null
  return step
}

onMounted(() => {
  const step = readStepFromQuery()
  if (step !== null) {
    goToStep(step)
  }
})

watch(currentStep, (step) => {
  if (route.query.step !== String(step)) {
    const params = new URLSearchParams(window.location.search)
    params.set('step', String(step))
    window.history.replaceState(null, '', `?${params.toString()}`)
  }
})
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-background p-4">
    <div class="w-full max-w-lg">
      <!-- Step indicator -->
      <div class="flex items-center justify-center gap-2 mb-8">
        <div
          v-for="i in STEP_COUNT"
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
        <component :is="stepComponents[currentStep].component" v-bind="stepComponents[currentStep].props" />
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
            @click="skipToEnd"
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
