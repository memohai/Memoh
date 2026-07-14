<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Button } from '@felinic/ui'
import { getSpeechProviders, getTranscriptionProviders, postSpeechProvidersByIdImportModels, postTranscriptionProvidersByIdImportModels } from '@memohai/sdk'
import type { AudioSpeechProviderResponse } from '@memohai/sdk'
import { Plus } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddProvider from '@/components/add-provider/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import PageShell from '@/components/page-shell/index.vue'
import SectionGroup from '@/components/section-group/index.vue'
import { useRoutedViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import SpeechSetting from '@/pages/speech/components/provider-setting.vue'
import TranscriptionSetting from '@/pages/transcription/provider-setting.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: speechData, isLoading: speechLoading } = useQuery({
  key: () => ['speech-providers'],
  query: async () => {
    const { data } = await getSpeechProviders({ throwOnError: true })
    return data
  },
})
const { data: transcriptionData, isLoading: transcriptionLoading } = useQuery({
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

type VoiceDetailKind = 'speech' | 'transcription'
type VoiceDetail = { kind: VoiceDetailKind, provider: AudioSpeechProviderResponse }
const detailKind = ref<VoiceDetailKind>('speech')
const openStatus = reactive({ addSpeechOpen: false, addTranscriptionOpen: false })

async function importSpeechModels(providerId: string) {
  const { data } = await postSpeechProvidersByIdImportModels({
    path: { id: providerId },
    throwOnError: true,
  })
  queryCache.invalidateQueries({ key: ['speech-providers'] })
  queryCache.invalidateQueries({ key: ['speech-models'] })
  queryCache.invalidateQueries({ key: ['speech-provider-models', providerId] })
  return data
}

async function importTranscriptionModels(providerId: string) {
  const { data } = await postTranscriptionProvidersByIdImportModels({
    path: { id: providerId },
    throwOnError: true,
  })
  queryCache.invalidateQueries({ key: ['transcription-providers'] })
  queryCache.invalidateQueries({ key: ['transcription-models'] })
  queryCache.invalidateQueries({ key: ['transcription-provider-models', providerId] })
  return data
}

function sortByEnabled<T extends { enable?: boolean }>(list: T[]) {
  return [...list].sort((a, b) => Number(b.enable !== false) - Number(a.enable !== false))
}

const speechProviders = computed<AudioSpeechProviderResponse[]>(() =>
  Array.isArray(speechData.value) ? sortByEnabled(speechData.value) : [],
)
const transcriptionProviders = computed<AudioSpeechProviderResponse[]>(() =>
  Array.isArray(transcriptionData.value) ? sortByEnabled(transcriptionData.value) : [],
)

const { view, direction, openDetail, backToList: closeProvider } = useRoutedViewSwap<VoiceDetail>({
  key: 'provider',
  items: () => [
    ...speechProviders.value.map(provider => ({ kind: 'speech' as const, provider })),
    ...transcriptionProviders.value.map(provider => ({ kind: 'transcription' as const, provider })),
  ],
  selected: () => {
    const provider = detailKind.value === 'speech' ? curTts.value : curTranscription.value
    return provider ? { kind: detailKind.value, provider } : undefined
  },
  select: (detail) => {
    detailKind.value = detail?.kind ?? 'speech'
    curTts.value = detail?.kind === 'speech' ? detail.provider : undefined
    curTranscription.value = detail?.kind === 'transcription' ? detail.provider : undefined
  },
  getRouteValue: detail => `${detail.kind}:${detail.provider.id}`,
  isLoading: routeValue => routeValue.startsWith('speech:')
    ? speechLoading.value
    : routeValue.startsWith('transcription:') && transcriptionLoading.value,
  isReady: routeValue => routeValue.startsWith('speech:')
    ? speechData.value !== undefined
    : routeValue.startsWith('transcription:')
      ? transcriptionData.value !== undefined
      : true,
})

const addProviderNames = computed(() => [
  ...speechProviders.value.map((p) => ({ name: p.name })),
  ...transcriptionProviders.value.map((p) => ({ name: p.name })),
])

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function openSpeech(provider: AudioSpeechProviderResponse) {
  openDetail({ kind: 'speech', provider })
}

function openTranscription(provider: AudioSpeechProviderResponse) {
  openDetail({ kind: 'transcription', provider })
}

// Each section adds its own kind of provider, so refresh just that list when
// the matching add dialog closes.
watch(() => openStatus.addSpeechOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['speech-providers'] })
  }
})
watch(() => openStatus.addTranscriptionOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['transcription-providers'] })
  }
})

</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Two capability sections -->
    <PageShell
      v-if="view === 'list'"
      :title="t('voice.title')"
    >
      <div class="space-y-8">
        <!-- Speaking (TTS) -->
        <SectionGroup
          :title="t('voice.speakTitle')"
          :description="t('voice.speakHint')"
        >
          <template #actions>
            <Button
              variant="secondary"
              size="sm"
              @click="openStatus.addSpeechOpen = true"
            >
              <Plus class="size-4" />
              {{ t('common.add') }}
            </Button>
          </template>

          <div
            v-if="speechProviders.length > 0"
            class="grid grid-cols-1 gap-3 sm:grid-cols-2"
          >
            <BackendCard
              v-for="provider in speechProviders"
              :key="provider.id"
              :name="provider.name ?? ''"
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
        </SectionGroup>

        <!-- Listening (STT) -->
        <SectionGroup
          :title="t('voice.listenTitle')"
          :description="t('voice.listenHint')"
        >
          <template #actions>
            <Button
              variant="secondary"
              size="sm"
              @click="openStatus.addTranscriptionOpen = true"
            >
              <Plus class="size-4" />
              {{ t('common.add') }}
            </Button>
          </template>

          <div
            v-if="transcriptionProviders.length > 0"
            class="grid grid-cols-1 gap-3 sm:grid-cols-2"
          >
            <BackendCard
              v-for="provider in transcriptionProviders"
              :key="provider.id"
              :name="provider.name ?? ''"
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
        </SectionGroup>
      </div>

      <AddProvider
        v-model:open="openStatus.addSpeechOpen"
        hide-trigger
        preset-domain="speech"
        :providers="addProviderNames"
        :import-models="importSpeechModels"
      />
      <AddProvider
        v-model:open="openStatus.addTranscriptionOpen"
        hide-trigger
        preset-domain="transcription"
        :providers="addProviderNames"
        :import-models="importTranscriptionModels"
      />
    </PageShell>

    <!-- Voice backend detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('voice.title')"
      @back="closeProvider"
    >
      <SpeechSetting v-if="detailKind === 'speech' && curTts?.id" />
      <TranscriptionSetting v-else-if="detailKind === 'transcription' && curTranscription?.id" />
    </DetailPane>
  </SwapTransition>
</template>
