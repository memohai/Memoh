<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <!-- Header Section -->
    <div class="space-y-0.5">
      <h4 class="text-xs font-medium text-foreground">
        {{ $t('bots.settings.blocks.multimedia') }}
      </h4>
      <p class="text-[11px] text-muted-foreground">
        {{ $t('bots.settings.blocks.multimediaDescription') }}
      </p>
    </div>
    
    <!-- Configuration inputs with compact spacing -->
    <div class="space-y-3 pt-1">
      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.ttsModel') }}</Label>
        <TtsModelSelect
          v-model="form.tts_model_id"
          :models="ttsModels"
          :providers="ttsProviders"
          :placeholder="$t('bots.settings.ttsModelPlaceholder')"
          show-icons
        />
      </div>

      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.transcriptionModel') }}</Label>
        <TtsModelSelect
          v-model="form.transcription_model_id"
          :models="transcriptionModels"
          :providers="ttsProviders"
          :placeholder="$t('bots.settings.transcriptionModelPlaceholder')"
          show-icons
        />
      </div>

      <div class="space-y-1.5">
        <div class="space-y-0.5">
          <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.imageModel') }}</Label>
          <p class="text-[10px] text-muted-foreground">
            {{ $t('bots.settings.imageModelDescription') }}
          </p>
        </div>
        <ModelSelect
          v-model="form.image_model_id"
          :models="imageCapableModels"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.imageModelPlaceholder')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Label } from '@memohai/ui'
import ModelSelect from './model-select.vue'
import TtsModelSelect from './tts-model-select.vue'
import type { 
  SettingsSettings, 
  AudioSpeechModelResponse, 
  AudioSpeechProviderResponse, 
  AudioTranscriptionModelResponse,
  ModelsGetResponse,
  ProvidersGetResponse
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
