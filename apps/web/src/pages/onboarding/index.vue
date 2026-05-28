<script setup lang="ts">
import { onMounted, watch, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useOnboarding, STEP_COUNT } from '@/composables/useOnboarding'

import PlaceholderStep from './steps/PlaceholderStep.vue'
import Step1Intro from './steps/Step1Intro.vue'
import Step2Appearance from './steps/Step2Appearance.vue'
import Step3Provider from './steps/Step3Provider.vue'
import Step6Complete from './steps/Step6Complete.vue'

const route = useRoute()
const { currentStep, introTextVisible, goToStep } = useOnboarding()

const dotsVisible = computed(() => currentStep.value > 0 || introTextVisible.value)

const stepComponents = [
  { component: Step1Intro, props: {} },
  { component: Step2Appearance, props: {} },
  { component: Step3Provider, props: {} },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.bot.title', descKey: 'onboarding.bot.placeholder' } },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.im.title', descKey: 'onboarding.im.placeholder' } },
  { component: PlaceholderStep, props: { titleKey: 'onboarding.welcome.title', descKey: 'onboarding.welcome.description' } },
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
  <div class="min-h-screen flex flex-col bg-background p-4">
    <div class="flex-1 flex items-center justify-center">
      <div class="w-full max-w-lg">
        <component
          :is="stepComponents[currentStep].component"
          v-bind="stepComponents[currentStep].props"
        />
      </div>
    </div>

    <div
      class="flex items-center justify-center gap-2 pb-6 transition-opacity duration-500 ease-out"
      :class="dotsVisible ? 'opacity-100' : 'opacity-0'"
    >
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
  </div>
</template>
