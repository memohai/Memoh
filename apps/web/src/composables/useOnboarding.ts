import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { putUsersMe } from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { useUserStore } from '@/store/user'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { readACPSelection, clearACPSelection } from '@/pages/onboarding/steps/useACPSetup'

export const LAST_STEP_INDEX = 4
export const STEP_COUNT = 5

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

  async function complete(minTransitionMs = 0): Promise<boolean> {
    completing.value = true
    const minWait = new Promise<void>((resolve) => setTimeout(resolve, minTransitionMs))
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
      return false
    }
    await minWait
    const createdBotId = sessionStorage.getItem(ONBOARDING_KEYS.createdBotId)
    const acpAgentId = readACPSelection()?.agentId ?? ''
    sessionStorage.removeItem(ONBOARDING_KEYS.createdBotId)
    sessionStorage.removeItem(ONBOARDING_KEYS.providerAddedCount)
    clearACPSelection()
    localStorage.removeItem(ONBOARDING_KEYS.forceOnboarding)
    if (createdBotId) {
      // Navigate to the `bot` route directly (not the `/chat/...` redirect,
      // which drops the query) so the chat page can read `?acp=` on landing.
      await router.replace(
        acpAgentId
          ? { name: 'bot', params: { botName: createdBotId }, query: { acp: acpAgentId } }
          : { name: 'bot', params: { botName: createdBotId } },
      )
    } else {
      await router.replace('/')
    }
    completing.value = false
    return true
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
