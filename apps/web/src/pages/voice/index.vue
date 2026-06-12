<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Button } from '@memohai/ui'
import { getSpeechProviders, getTranscriptionProviders } from '@memohai/sdk'
import type { AudioSpeechProviderResponse } from '@memohai/sdk'
import { Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddProvider from '@/components/add-provider/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import SpeechSetting from '@/pages/speech/components/provider-setting.vue'
import TranscriptionSetting from '@/pages/transcription/provider-setting.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: speechData } = useQuery({
  key: () => ['speech-providers'],
  query: async () => {
    const { data } = await getSpeechProviders({ throwOnError: true })
    return data
  },
})
const { data: transcriptionData } = useQuery({
  key: () => ['transcription-providers'],
  query: async () => {
    const { data } = await getTranscriptionProviders({ throwOnError: true })
    return (data ?? []) as AudioSpeechProviderResponse[]
  },
})

const curTts = ref<AudioSpeechProviderResponse>()
const curTranscription = ref<AudioSpeechProviderResponse>()
provide('curTtsProvider', curTts)
provide('curTranscriptionProvider', curTranscription)

const { view, direction, openDetail, backToList } = useViewSwap()
const detailKind = ref<'speech' | 'transcription'>('speech')
const openStatus = reactive({ addOpen: false })

function sortByEnabled<T extends { enable?: boolean }>(list: T[]) {
  return [...list].sort((a, b) => Number(b.enable !== false) - Number(a.enable !== false))
}

const speechProviders = computed<AudioSpeechProviderResponse[]>(() =>
  Array.isArray(speechData.value) ? sortByEnabled(speechData.value) : [],
)
const transcriptionProviders = computed<AudioSpeechProviderResponse[]>(() =>
  Array.isArray(transcriptionData.value) ? sortByEnabled(transcriptionData.value) : [],
)

const addProviderNames = computed(() => [
  ...speechProviders.value.map((p) => ({ name: p.name })),
  ...transcriptionProviders.value.map((p) => ({ name: p.name })),
])

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function openSpeech(provider: AudioSpeechProviderResponse) {
  curTts.value = provider
  detailKind.value = 'speech'
  openDetail()
}

function openTranscription(provider: AudioSpeechProviderResponse) {
  curTranscription.value = provider
  detailKind.value = 'transcription'
  openDetail()
}

// A provider becomes voice-capable once added under Providers, so refresh both
// lists when the add dialog closes.
watch(() => openStatus.addOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['speech-providers'] })
    queryCache.invalidateQueries({ key: ['transcription-providers'] })
  }
})

watch(speechProviders, (list) => {
  const id = curTts.value?.id
  if (!id) return
  const found = list.find((p) => p.id === id)
  if (found) curTts.value = found
  else if (view.value === 'detail' && detailKind.value === 'speech') backToList()
})
watch(transcriptionProviders, (list) => {
  const id = curTranscription.value?.id
  if (!id) return
  const found = list.find((p) => p.id === id)
  if (found) curTranscription.value = found
  else if (view.value === 'detail' && detailKind.value === 'transcription') backToList()
})
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Two capability sections -->
    <section
      v-if="view === 'list'"
      class="mx-auto max-w-3xl px-6 pt-10 pb-12 space-y-8"
    >
      <h1 class="px-2 text-lg font-semibold">
        {{ t('voice.title') }}
      </h1>

      <!-- Speaking (TTS) -->
      <section class="space-y-2.5">
        <div class="flex items-center justify-between gap-4 px-2">
          <div class="min-w-0">
            <h2 class="text-[13px] font-medium text-foreground">
              {{ t('voice.speakTitle') }}
            </h2>
            <p class="text-xs text-muted-foreground">
              {{ t('voice.speakHint') }}
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            class="shrink-0"
            @click="openStatus.addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('common.add') }}
          </Button>
        </div>

        <div
          v-if="speechProviders.length > 0"
          class="grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <BackendCard
            v-for="provider in speechProviders"
            :key="provider.id"
            :name="provider.name ?? ''"
            :subtitle="provider.client_type ?? ''"
            :enabled="provider.enable !== false"
            @click="openSpeech(provider)"
          >
            <template #leading>
              <span class="flex size-10 items-center justify-center rounded-full bg-muted">
                <ProviderIcon
                  v-if="provider.icon"
                  :icon="provider.icon"
                  size="1.5em"
                />
                <span
                  v-else
                  class="text-xs font-medium text-muted-foreground"
                >
                  {{ getInitials(provider.name) }}
                </span>
              </span>
            </template>
          </BackendCard>
        </div>
        <p
          v-else
          class="px-2 text-xs text-muted-foreground"
        >
          {{ t('voice.empty') }}
        </p>
      </section>

      <!-- Listening (STT) -->
      <section class="space-y-2.5">
        <div class="flex items-center justify-between gap-4 px-2">
          <div class="min-w-0">
            <h2 class="text-[13px] font-medium text-foreground">
              {{ t('voice.listenTitle') }}
            </h2>
            <p class="text-xs text-muted-foreground">
              {{ t('voice.listenHint') }}
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            class="shrink-0"
            @click="openStatus.addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('common.add') }}
          </Button>
        </div>

        <div
          v-if="transcriptionProviders.length > 0"
          class="grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <BackendCard
            v-for="provider in transcriptionProviders"
            :key="provider.id"
            :name="provider.name ?? ''"
            :subtitle="provider.client_type ?? ''"
            :enabled="provider.enable !== false"
            @click="openTranscription(provider)"
          >
            <template #leading>
              <span class="flex size-10 items-center justify-center rounded-full bg-muted">
                <ProviderIcon
                  v-if="provider.icon"
                  :icon="provider.icon"
                  size="1.5em"
                />
                <span
                  v-else
                  class="text-xs font-medium text-muted-foreground"
                >
                  {{ getInitials(provider.name) }}
                </span>
              </span>
            </template>
          </BackendCard>
        </div>
        <p
          v-else
          class="px-2 text-xs text-muted-foreground"
        >
          {{ t('voice.empty') }}
        </p>
      </section>

      <AddProvider
        v-model:open="openStatus.addOpen"
        :providers="addProviderNames"
        hide-trigger
      />
    </section>

    <!-- Voice backend detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('voice.title')"
      @back="backToList()"
    >
      <SpeechSetting v-if="detailKind === 'speech' && curTts?.id" />
      <TranscriptionSetting v-else-if="detailKind === 'transcription' && curTranscription?.id" />
    </DetailPane>
  </SwapTransition>
</template>
