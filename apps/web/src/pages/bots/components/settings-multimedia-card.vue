<!-- eslint-disable vue/no-mutating-props -->
<template>
  <SettingsSection :title="$t('bots.settings.blocks.multimedia')">
    <SettingsRow :label="$t('bots.settings.ttsModel')">
      <div class="w-56">
        <TtsModelSelect
          v-model="form.tts_model_id"
          :models="ttsModels"
          :providers="ttsProviders"
          :placeholder="$t('bots.settings.ttsModelPlaceholder')"
          show-icons
        />
      </div>
    </SettingsRow>

    <SettingsRow :label="$t('bots.settings.transcriptionModel')">
      <div class="w-56">
        <TtsModelSelect
          v-model="form.transcription_model_id"
          :models="transcriptionModels"
          :providers="ttsProviders"
          :placeholder="$t('bots.settings.transcriptionModelPlaceholder')"
          show-icons
        />
      </div>
    </SettingsRow>

    <SettingsRow
      :label="$t('bots.settings.imageModel')"
      :description="$t('bots.settings.imageModelDescription')"
    >
      <div class="w-56">
        <ModelSelect
          v-model="form.image_model_id"
          :models="imageCapableModels"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.imageModelPlaceholder')"
        />
      </div>
    </SettingsRow>
  </SettingsSection>
</template>

<script setup lang="ts">
import ModelSelect from './model-select.vue'
import TtsModelSelect from './tts-model-select.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import type {
  SettingsSettings,
  AudioSpeechModelResponse,
  AudioSpeechProviderResponse,
  AudioTranscriptionModelResponse,
  ModelsGetResponse,
  ProvidersGetResponse,
} from '@memohai/sdk'

defineProps<{
  form: SettingsSettings
  ttsModels: AudioSpeechModelResponse[]
  ttsProviders: AudioSpeechProviderResponse[]
  transcriptionModels: AudioTranscriptionModelResponse[]
  imageCapableModels: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
}>()
</script>
