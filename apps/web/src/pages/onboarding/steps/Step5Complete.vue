<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plug, AudioLines, Globe, AlertTriangle } from 'lucide-vue-next'
import { useOnboarding } from '@/composables/useOnboarding'
import { nextFrame } from '../useStepTransition'
import { ONBOARDING_KEYS } from '../constants'
import { readACPSelection } from './useACPSetup'

const { t } = useI18n()
const { complete, completing } = useOnboarding()

const visible = ref(false)
const exiting = ref(false)
const hasProvider = ref(true)

const cards = [
  { icon: Plug, titleKey: 'onboarding.complete.cards.im.title', descKey: 'onboarding.complete.cards.im.desc' },
  { icon: AudioLines, titleKey: 'onboarding.complete.cards.voice.title', descKey: 'onboarding.complete.cards.voice.desc' },
  { icon: Globe, titleKey: 'onboarding.complete.cards.search.title', descKey: 'onboarding.complete.cards.search.desc' },
] as const

onMounted(() => {
  const count = Number.parseInt(sessionStorage.getItem(ONBOARDING_KEYS.providerAddedCount) ?? '0', 10)
  hasProvider.value = (Number.isFinite(count) && count > 0) || !!readACPSelection()
  nextFrame(() => {
    visible.value = true
  })
})

async function handleComplete() {
  if (completing.value) return
  exiting.value = true
  sessionStorage.setItem(ONBOARDING_KEYS.entryAnimation, '1')
  const ok = await complete(175)
  if (!ok) {
    exiting.value = false
    sessionStorage.removeItem(ONBOARDING_KEYS.entryAnimation)
  }
}
</script>

<template>
  <div
    class="text-center transition-all duration-[175ms] ease-out"
    :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
  >
    <h2
      class="text-5xl font-semibold tracking-tight mb-4 transition-all duration-[350ms] ease-out"
      :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
    >
      {{ t('onboarding.complete.title') }}
    </h2>
    <p
      class="text-base text-muted-foreground mb-12 transition-all duration-[350ms] ease-out delay-[80ms]"
      :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
    >
      {{ t('onboarding.complete.subtitle') }}
    </p>

    <div
      class="grid grid-cols-3 gap-3 mb-12 text-left transition-all duration-[350ms] ease-out delay-[160ms]"
      :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
    >
      <div
        v-for="card in cards"
        :key="card.titleKey"
        class="rounded-xl border bg-muted/30 px-5 py-6"
      >
        <component
          :is="card.icon"
          class="size-5 text-muted-foreground mb-4"
        />
        <div class="text-sm font-medium mb-1.5">
          {{ t(card.titleKey) }}
        </div>
        <div class="text-xs text-muted-foreground leading-relaxed">
          {{ t(card.descKey) }}
        </div>
      </div>
    </div>

    <div
      v-if="!hasProvider"
      class="mb-8 flex justify-center transition-all duration-[350ms] ease-out delay-[200ms]"
      :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
    >
      <div class="inline-flex items-start gap-2.5 rounded-lg border border-yellow-500/30 bg-yellow-500/5 px-4 py-3 text-left">
        <AlertTriangle class="size-4 shrink-0 text-yellow-600 dark:text-yellow-500 mt-0.5" />
        <p class="text-xs text-muted-foreground leading-relaxed">
          {{ t('onboarding.complete.noProviderWarning') }}
        </p>
      </div>
    </div>

    <div
      class="transition-all duration-[350ms] ease-out delay-[240ms]"
      :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
    >
      <button
        class="inline-flex h-[42px] w-[240px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        :disabled="completing"
        @click="handleComplete"
      >
        {{ t('onboarding.complete.action') }}
      </button>
    </div>
  </div>
</template>
