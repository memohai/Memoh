<template>
  <div class="space-y-4">
    <template v-if="caps">
      <!-- Language -->
      <div class="space-y-2">
        <Label for="tts-lang">{{ $t('ttsProvider.fields.language') }}</Label>
        <Select
          :model-value="configData.voice_lang ?? ''"
          @update:model-value="onLangChange"
        >
          <SelectTrigger
            id="tts-lang"
            class="w-full"
          >
            <SelectValue :placeholder="$t('ttsProvider.fields.languagePlaceholder')" />
          </SelectTrigger>
          <SelectContent class="max-h-60">
            <SelectItem
              v-for="lang in availableLanguages"
              :key="lang"
              :value="lang"
            >
              {{ lang }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <!-- Voice -->
      <div class="space-y-2">
        <Label for="tts-voice">{{ $t('ttsProvider.fields.voice') }}</Label>
        <Select
          :model-value="configData.voice_id ?? ''"
          @update:model-value="(val) => configData.voice_id = val"
        >
          <SelectTrigger
            id="tts-voice"
            class="w-full"
          >
            <SelectValue :placeholder="$t('ttsProvider.fields.voicePlaceholder')" />
          </SelectTrigger>
          <SelectContent class="max-h-60">
            <SelectItem
              v-for="voice in filteredVoices"
              :key="voice.id"
              :value="voice.id!"
            >
              {{ voice.name }} ({{ voice.id }})
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <!-- Format -->
      <div
        v-if="caps.formats && caps.formats.length > 0"
        class="space-y-2"
      >
        <Label for="tts-format">{{ $t('ttsProvider.fields.format') }}</Label>
        <Select
          :model-value="configData.format ?? ''"
          @update:model-value="(val) => configData.format = val"
        >
          <SelectTrigger
            id="tts-format"
            class="w-full"
          >
            <SelectValue :placeholder="$t('ttsProvider.fields.formatPlaceholder')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="fmt in caps.formats"
              :key="fmt"
              :value="fmt"
            >
              {{ fmt }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <!-- Speed -->
      <div
        v-if="caps.speed"
        class="space-y-2"
      >
        <Label>{{ $t('ttsProvider.fields.speed') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('ttsProvider.fields.speedDescription', { default: caps.speed.default ?? 1 }) }}
        </p>
        <div v-if="caps.speed.options && caps.speed.options.length > 0">
          <Select
            :model-value="String(configData.speed ?? caps.speed.default ?? 1)"
            @update:model-value="(val) => configData.speed = Number(val)"
          >
            <SelectTrigger class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="opt in caps.speed.options"
                :key="opt"
                :value="String(opt)"
              >
                {{ opt }}x
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div
          v-else
          class="flex items-center gap-3"
        >
          <Slider
            :model-value="[Number(configData.speed ?? caps.speed.default ?? 1)]"
            :min="caps.speed.min"
            :max="caps.speed.max"
            :step="0.1"
            class="flex-1"
            @update:model-value="(val) => configData.speed = val[0]"
          />
          <span class="text-sm text-muted-foreground w-12 text-right">
            {{ Number(configData.speed ?? caps.speed.default ?? 1).toFixed(1) }}x
          </span>
        </div>
      </div>

      <!-- Pitch -->
      <div
        v-if="caps.pitch"
        class="space-y-2"
      >
        <Label>{{ $t('ttsProvider.fields.pitch') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('ttsProvider.fields.pitchDescription', { default: caps.pitch.default ?? 0 }) }}
        </p>
        <div
          v-if="caps.pitch.options && caps.pitch.options.length > 0"
        >
          <Select
            :model-value="String(configData.pitch ?? caps.pitch.default ?? 0)"
            @update:model-value="(val) => configData.pitch = Number(val)"
          >
            <SelectTrigger class="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="opt in caps.pitch.options"
                :key="opt"
                :value="String(opt)"
              >
                {{ opt }} Hz
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div
          v-else
          class="flex items-center gap-3"
        >
          <Slider
            :model-value="[Number(configData.pitch ?? caps.pitch.default ?? 0)]"
            :min="caps.pitch.min"
            :max="caps.pitch.max"
            :step="1"
            class="flex-1"
            @update:model-value="(val) => configData.pitch = val[0]"
          />
          <span class="text-sm text-muted-foreground w-16 text-right">
            {{ Number(configData.pitch ?? caps.pitch.default ?? 0).toFixed(0) }} Hz
          </span>
        </div>
      </div>
    </template>

    <div
      v-else
      class="text-sm text-muted-foreground"
    >
      {{ $t('ttsProvider.noCapabilities') }}
    </div>

    <Separator class="my-3" />

    <!-- Test Synthesis -->
    <div class="space-y-3">
      <h4 class="text-sm font-medium">
        {{ $t('ttsProvider.test.title') }}
      </h4>
      <div class="relative">
        <Textarea
          v-model="testText"
          :placeholder="$t('ttsProvider.test.placeholder')"
          :maxlength="maxTestTextLen"
          rows="2"
          class="resize-none"
        />
        <span class="absolute right-2 bottom-2 text-xs text-muted-foreground">
          {{ testText.length }}/{{ maxTestTextLen }}
        </span>
      </div>
      <div class="flex items-center gap-3">
        <LoadingButton
          type="button"
          variant="outline"
          size="sm"
          :loading="testLoading"
          :disabled="!testText.trim() || testText.length > maxTestTextLen"
          @click="handleTest"
        >
          <FontAwesomeIcon
            :icon="['fas', 'play']"
            class="mr-1.5"
          />
          {{ $t('ttsProvider.test.generate') }}
        </LoadingButton>
        <span
          v-if="testError"
          class="text-xs text-destructive"
        >
          {{ testError }}
        </span>
      </div>
      <div
        v-if="audioUrl"
        class="rounded-md border border-border bg-muted/30 p-3"
      >
        <audio
          ref="audioEl"
          :src="audioUrl"
          controls
          class="w-full"
        />
      </div>
    </div>

    <Separator class="my-3" />

    <div class="flex justify-end">
      <LoadingButton
        type="button"
        size="sm"
        :loading="saving"
        @click="handleSaveConfig"
      >
        {{ $t('provider.saveChanges') }}
      </LoadingButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Slider,
  Textarea,
  Separator,
} from '@memoh/ui'
import LoadingButton from '@/components/loading-button/index.vue'
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  modelId: string
  modelName: string
  config: Record<string, any>
  capabilities: any
}>()

const emit = defineEmits<{
  save: [config: Record<string, unknown>]
  test: [text: string, config: Record<string, unknown>]
}>()

const { t } = useI18n()

const caps = computed(() => props.capabilities)

const configData = reactive<Record<string, any>>({})

watch(() => props.config, (cfg) => {
  Object.keys(configData).forEach((k) => delete configData[k])
  if (cfg.voice && typeof cfg.voice === 'object') {
    configData.voice_id = (cfg.voice as any).id ?? ''
    configData.voice_lang = (cfg.voice as any).lang ?? ''
  }
  if (cfg.format) configData.format = cfg.format
  if (cfg.speed != null) configData.speed = cfg.speed
  if (cfg.pitch != null) configData.pitch = cfg.pitch
  if (cfg.sample_rate != null) configData.sample_rate = cfg.sample_rate
}, { immediate: true })

const availableLanguages = computed(() => {
  if (!caps.value?.voices) return []
  const langs = new Set(caps.value.voices.map((v: any) => v.lang ?? '').filter(Boolean))
  return [...langs].sort()
})

const filteredVoices = computed(() => {
  if (!caps.value?.voices) return []
  const lang = configData.voice_lang
  if (!lang) return caps.value.voices
  return caps.value.voices.filter((v: any) => v.lang === lang)
})

function onLangChange(lang: string) {
  configData.voice_lang = lang
  const voices = caps.value?.voices?.filter((v: any) => v.lang === lang)
  if (voices && voices.length > 0 && !voices.some((v: any) => v.id === configData.voice_id)) {
    configData.voice_id = voices[0].id ?? ''
  }
}

function buildConfig(): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  if (configData.voice_id || configData.voice_lang) {
    result.voice = { id: configData.voice_id ?? '', lang: configData.voice_lang ?? '' }
  }
  if (configData.format) result.format = configData.format
  if (configData.speed != null) result.speed = Number(configData.speed)
  if (configData.pitch != null) result.pitch = Number(configData.pitch)
  if (configData.sample_rate != null) result.sample_rate = Number(configData.sample_rate)
  return result
}

const saving = ref(false)
async function handleSaveConfig() {
  saving.value = true
  try {
    emit('save', buildConfig())
  } finally {
    saving.value = false
  }
}

// Test synthesis
const maxTestTextLen = 500
const testText = ref('')
const testLoading = ref(false)
const testError = ref('')
const audioUrl = ref('')
const audioEl = ref<HTMLAudioElement>()

function revokeAudio() {
  if (audioUrl.value) {
    URL.revokeObjectURL(audioUrl.value)
    audioUrl.value = ''
  }
}

onBeforeUnmount(revokeAudio)

async function handleTest() {
  if (!testText.value.trim()) return
  testLoading.value = true
  testError.value = ''
  revokeAudio()

  try {
    const blob = await new Promise<Blob>((resolve, reject) => {
      const handler = async () => {
        try {
          const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'
          const token = localStorage.getItem('token')
          const resp = await fetch(`${apiBase}/tts-models/${props.modelId}/test`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              ...(token ? { Authorization: `Bearer ${token}` } : {}),
            },
            body: JSON.stringify({ text: testText.value, config: buildConfig() }),
          })
          if (!resp.ok) {
            const errBody = await resp.text()
            let msg: string
            try { msg = JSON.parse(errBody)?.message ?? errBody } catch { msg = errBody }
            reject(new Error(msg))
            return
          }
          resolve(await resp.blob())
        } catch (e) {
          reject(e)
        }
      }
      handler()
    })

    audioUrl.value = URL.createObjectURL(blob)
    await new Promise<void>((resolve) => setTimeout(resolve, 50))
    audioEl.value?.play()
  } catch (e: any) {
    testError.value = e?.message || t('ttsProvider.test.failed')
    toast.error(e?.message || t('ttsProvider.test.failed'))
  } finally {
    testLoading.value = false
  }
}
</script>
