import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { putUsersMe } from '@memohai/sdk'

export const LAST_STEP_INDEX = 6
export const STEP_COUNT = 7

export function useOnboarding() {
  const router = useRouter()
  const currentStep = ref(0)
  const completing = ref(false)

  const isFirstStep = computed(() => currentStep.value === 0)
  const isLastStep = computed(() => currentStep.value === LAST_STEP_INDEX)

  function nextStep() {
    if (currentStep.value < LAST_STEP_INDEX) {
      currentStep.value++
    }
  }

  function prevStep() {
    if (currentStep.value > 0) {
      currentStep.value--
    }
  }

  function goToStep(step: number) {
    if (step >= 0 && step <= LAST_STEP_INDEX) {
      currentStep.value = step
    }
  }

  function skipToEnd() {
    currentStep.value = LAST_STEP_INDEX
  }

  async function complete() {
    completing.value = true
    localStorage.removeItem('memoh:dev:force-onboarding')
    try {
      await putUsersMe({
        body: {
          metadata: { onboarding_completed: true },
        },
      })
      localStorage.setItem('memoh:onboarding:completed', '1')
    } catch {
      // API failed, but don't block the user — the guard will retry next load
    }
    router.push('/')
    completing.value = false
  }

  return {
    currentStep,
    completing,
    isFirstStep,
    isLastStep,
    nextStep,
    prevStep,
    goToStep,
    skipToEnd,
    complete,
  }
}
