import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { putUsersMe } from '@memohai/sdk'
import { useSettingsStore } from '@/store/settings'
import { detectLocale } from '@/utils/detect-locale'

export interface OnboardingStep {
  index: number
  title: string
}

const TOTAL_STEPS = 6

const stepTitles = [
  'onboarding.steps.welcome',
  'onboarding.steps.intro',
  'onboarding.steps.appearance',
  'onboarding.steps.provider',
  'onboarding.steps.bot',
  'onboarding.steps.im',
  'onboarding.steps.complete',
]

export function useOnboarding() {
  const router = useRouter()
  const settings = useSettingsStore()
  const currentStep = ref(0)
  const completing = ref(false)

  const isFirstStep = computed(() => currentStep.value === 0)
  const isLastStep = computed(() => currentStep.value === TOTAL_STEPS)

  const stepTitle = computed(() => stepTitles[currentStep.value] || '')

  function detectAndApplyLocale() {
    const stored = settings.language
    if (!stored || stored === 'en') {
      const detected = detectLocale()
      if (detected !== 'en') {
        settings.setLanguage(detected)
      }
    }
  }

  function nextStep() {
    if (currentStep.value < TOTAL_STEPS) {
      currentStep.value++
    }
  }

  function prevStep() {
    if (currentStep.value > 0) {
      currentStep.value--
    }
  }

  function goToStep(step: number) {
    if (step >= 0 && step <= TOTAL_STEPS) {
      currentStep.value = step
    }
  }

  async function complete() {
    completing.value = true
    try {
      localStorage.removeItem('memoh:dev:force-onboarding')
      await putUsersMe({
        body: {
          metadata: { onboarding_completed: true },
        },
      })
      router.push('/')
    } catch {
      router.push('/')
    } finally {
      completing.value = false
    }
  }

  function resetAndRestart() {
    localStorage.removeItem('memoh:dev:force-onboarding')
    currentStep.value = 0
    router.push('/onboarding')
  }

  return {
    currentStep,
    completing,
    isFirstStep,
    isLastStep,
    stepTitle,
    TOTAL_STEPS,
    nextStep,
    prevStep,
    goToStep,
    complete,
    resetAndRestart,
    detectAndApplyLocale,
  }
}
