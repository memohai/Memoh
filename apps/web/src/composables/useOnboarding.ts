import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { putUsersMe } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { useUserStore } from '@/store/user'

export const LAST_STEP_INDEX = 6
export const STEP_COUNT = 7

const currentStep = ref(0)
const completing = ref(false)
const introTextVisible = ref(false)

export function resetOnboardingState() {
  currentStep.value = 0
  completing.value = false
  introTextVisible.value = false
}

export function useOnboarding() {
  const router = useRouter()
  const { t } = useI18n()

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
    try {
      await putUsersMe({
        body: { metadata: { onboarding_completed: true } },
        throwOnError: true,
      })
      const userStore = useUserStore()
      userStore.onboardingCompleted = true
    } catch {
      toast.error(t('onboarding.complete.saveFailed'))
      completing.value = false
      return
    }
    sessionStorage.removeItem('onboarding.provider.addedCount')
    localStorage.removeItem('memoh:dev:force-onboarding')
    await router.replace('/')
    completing.value = false
  }

  return {
    currentStep,
    completing,
    introTextVisible,
    isFirstStep,
    isLastStep,
    nextStep,
    prevStep,
    goToStep,
    skipToEnd,
    complete,
  }
}
