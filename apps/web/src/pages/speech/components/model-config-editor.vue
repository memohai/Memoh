<template>
  <div class="space-y-4">
    <div
      v-if="basicFields.length > 0"
      class="grid gap-4 md:grid-cols-2"
    >
      <section
        v-for="field in basicFields"
        :key="field.key"
        class="space-y-2"
        :class="isWideField(field) ? 'md:col-span-2' : ''"
      >
        <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `tts-field-${field.key}`">
          {{ field.title || field.key }}
        </Label>
        <p
          v-if="field.description"
          class="text-xs text-muted-foreground"
        >
          {{ field.description }}
        </p>

        <div
          v-if="field.type === 'secret'"
          class="relative"
        >
          <Input
            :id="`tts-field-${field.key}`"
            v-model="configData[field.key] as string"
            :type="visibleSecrets[field.key] ? 'text' : 'password'"
            :placeholder="field.example ? String(field.example) : ''"
          />
          <button
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
          >
            <component
              :is="visibleSecrets[field.key] ? EyeOff : Eye"
              class="size-3.5"
            />
          </button>
        </div>

        <Switch
          v-else-if="field.type === 'bool'"
          :model-value="!!configData[field.key]"
          @update:model-value="(val) => configData[field.key] = !!val"
        />

        <Input
          v-else-if="field.type === 'number'"
          :id="`tts-field-${field.key}`"
          v-model.number="configData[field.key] as number"
          type="number"
          :placeholder="field.example ? String(field.example) : ''"
        />

        <Select
          v-else-if="field.type === 'enum' && field.enum"
          :model-value="String(configData[field.key] ?? '')"
          @update:model-value="(val) => configData[field.key] = val"
        >
          <SelectTrigger>
            <SelectValue :placeholder="field.title || field.key" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="opt in field.enum"
              :key="opt"
              :value="opt"
            >
              {{ opt }}
            </SelectItem>
          </SelectContent>
        </Select>

        <Input
          v-else
          :id="`tts-field-${field.key}`"
          v-model="configData[field.key] as string"
          type="text"
          :placeholder="field.example ? String(field.example) : ''"
        />
      </section>
    </div>

    <div
      v-if="basicFields.length === 0 && advancedFields.length === 0"
      class="text-xs text-muted-foreground"
    >
      {{ mode === 'transcription' ? $t('transcription.noCapabilities') : $t('speech.noCapabilities') }}
    </div>

    <div
      v-if="advancedFields.length > 0"
      class="rounded-lg border border-border"
    >
      <button
        type="button"
        class="flex w-full items-center justify-between px-3 py-2 text-left text-xs font-medium"
        @click="showAdvanced = !showAdvanced"
      >
        <span>{{ mode === 'transcription' ? $t('transcription.advanced.title') : $t('speech.advanced.title') }}</span>
        <component
          :is="showAdvanced ? ChevronUp : ChevronDown"
          class="size-3 text-muted-foreground"
        />
      </button>
      <div
        v-if="showAdvanced"
        class="border-t border-border px-3 py-3 space-y-4"
      >
        <p class="text-xs text-muted-foreground">
          {{ mode === 'transcription' ? $t('transcription.advanced.description') : $t('speech.advanced.description') }}
        </p>
        <div class="grid gap-4 md:grid-cols-2">
          <section
            v-for="field in advancedFields"
            :key="field.key"
            class="space-y-2"
            :class="isWideField(field) ? 'md:col-span-2' : ''"
          >
            <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `tts-field-${field.key}`">
              {{ field.title || field.key }}
            </Label>
            <p
              v-if="field.description"
              class="text-xs text-muted-foreground"
            >
              {{ field.description }}
            </p>

            <div
              v-if="field.type === 'secret'"
              class="relative"
            >
              <Input
                :id="`tts-field-${field.key}`"
                v-model="configData[field.key] as string"
                :type="visibleSecrets[field.key] ? 'text' : 'password'"
                :placeholder="field.example ? String(field.example) : ''"
              />
              <button
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
              >
                <component
                  :is="visibleSecrets[field.key] ? EyeOff : Eye"
                  class="size-3.5"
                />
              </button>
            </div>

            <Switch
              v-else-if="field.type === 'bool'"
              :model-value="!!configData[field.key]"
              @update:model-value="(val) => configData[field.key] = !!val"
            />

            <Input
              v-else-if="field.type === 'number'"
              :id="`tts-field-${field.key}`"
              v-model.number="configData[field.key] as number"
              type="number"
              :placeholder="field.example ? String(field.example) : ''"
            />

            <Select
              v-else-if="field.type === 'enum' && field.enum"
              :model-value="String(configData[field.key] ?? '')"
              @update:model-value="(val) => configData[field.key] = val"
            >
              <SelectTrigger>
                <SelectValue :placeholder="field.title || field.key" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="opt in field.enum"
                  :key="opt"
                  :value="opt"
                >
                  {{ opt }}
                </SelectItem>
              </SelectContent>
            </Select>

            <Input
              v-else
              :id="`tts-field-${field.key}`"
              v-model="configData[field.key] as string"
              type="text"
              :placeholder="field.example ? String(field.example) : ''"
            />
          </section>
        </div>
      </div>
    </div>

    <Separator class="my-3" />

    <div class="space-y-3">
      <h4 class="text-xs font-medium">
        {{ mode === 'transcription' ? $t('transcription.test.title') : $t('speech.test.title') }}
      </h4>
      <div
        v-if="mode === 'synthesis'"
        class="relative"
      >
        <Textarea
          v-model="testText"
          :placeholder="$t('speech.test.placeholder')"
          :maxlength="maxTestTextLen"
          rows="2"
          class="resize-none"
        />
        <span class="absolute right-2 bottom-2 text-xs text-muted-foreground">
          {{ testText.length }}/{{ maxTestTextLen }}
        </span>
      </div>
      <div
        v-else
        class="space-y-2"
      >
        <Input
          type="file"
          accept="audio/*"
          class="file:-mt-1"
          @change="handleFileChange"
        />
        <p
          v-if="selectedFileName"
          class="text-xs text-muted-foreground"
        >
          {{ selectedFileName }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <LoadingButton
          type="button"
          variant="outline"
          size="sm"
          :loading="testLoading"
          :disabled="mode === 'synthesis' ? (!testText.trim() || testText.length > maxTestTextLen) : !selectedFile"
          @click="handleTest"
        >
          <Play
            v-if="mode === 'synthesis'"
            class="mr-1.5"
          />
          {{ mode === 'transcription' ? $t('transcription.test.run') : $t('speech.test.generate') }}
        </LoadingButton>
        <span
          v-if="testError"
          class="text-xs text-destructive"
        >
          {{ testError }}
        </span>
      </div>
      <div
        v-if="mode === 'synthesis' && audioUrl"
        class="rounded-md border border-border bg-muted/30 p-3"
      >
        <audio
          ref="audioEl"
          :src="audioUrl"
          controls
          class="w-full"
        />
      </div>
      <div
        v-if="mode === 'transcription' && transcriptionText"
        class="rounded-md border border-border bg-muted/30 p-3 space-y-2"
      >
        <p class="text-sm whitespace-pre-wrap wrap-break-word">
          {{ transcriptionText }}
        </p>
        <p
          v-if="transcriptionLanguage"
          class="text-xs text-muted-foreground"
        >
          {{ transcriptionLanguage }}
        </p>
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
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Switch,
  Textarea,
} from '@memohai/ui'
import { ChevronDown, ChevronUp, Eye, EyeOff, Play } from 'lucide-vue-next'
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import LoadingButton from '@/components/loading-button/index.vue'

interface SpeechFieldSchema {
  key: string
  type: string
  title?: string
  description?: string
  required?: boolean
  advanced?: boolean
  enum?: string[]
  example?: unknown
  order?: number
}

interface SpeechConfigSchema {
  fields?: SpeechFieldSchema[]
}

const props = defineProps<{
  modelId: string
  modelName: string
  config: Record<string, unknown>
  schema: SpeechConfigSchema | null
  mode?: 'synthesis' | 'transcription'
  onTest: (payload: string | File, config: Record<string, unknown>) => Promise<Blob | { text?: string, language?: string }>
}>()

const emit = defineEmits<{
  save: [config: Record<string, unknown>]
}>()

const { t } = useI18n()
const configData = reactive<Record<string, unknown>>({})
const visibleSecrets = reactive<Record<string, boolean>>({})
const saving = ref(false)
const showAdvanced = ref(false)
const testText = ref('')
const selectedFile = ref<File | null>(null)
const selectedFileName = ref('')
const testLoading = ref(false)
const testError = ref('')
const audioUrl = ref('')
const transcriptionText = ref('')
const transcriptionLanguage = ref('')
const audioEl = ref<HTMLAudioElement>()
const maxTestTextLen = 500
const mode = computed(() => props.mode ?? 'synthesis')

const orderedFields = computed(() => {
  const fields = props.schema?.fields ?? []
  return [...fields].sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
})

const basicFields = computed(() => orderedFields.value.filter(field => !field.advanced))
const advancedFields = computed(() => orderedFields.value.filter(field => field.advanced))

function isWideField(field: SpeechFieldSchema) {
  if (field.type === 'secret') return true
  const key = field.key.toLowerCase()
  if (key.includes('url') || key.includes('endpoint') || key.includes('key') || key.includes('token') || key.includes('path') || key.includes('uri')) return true
  if ((field.description ?? '').length > 80) return true
  return false
}

watch(() => props.config, (cfg) => {
  Object.keys(configData).forEach((key) => delete configData[key])
  Object.assign(configData, { ...(cfg ?? {}) })
  showAdvanced.value = advancedFields.value.some(field => {
    const value = cfg?.[field.key]
    return value !== '' && value != null
  })
}, { immediate: true, deep: true })

function buildConfig(): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(configData)) {
    if (value === '' || value == null) continue
    result[key] = value
  }
  return result
}

function revokeAudio() {
  if (audioUrl.value) {
    URL.revokeObjectURL(audioUrl.value)
    audioUrl.value = ''
  }
}

function resetTranscription() {
  transcriptionText.value = ''
  transcriptionLanguage.value = ''
}

onBeforeUnmount(revokeAudio)

async function handleSaveConfig() {
  saving.value = true
  try {
    emit('save', buildConfig())
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  if (mode.value === 'synthesis' && !testText.value.trim()) return
  if (mode.value === 'transcription' && !selectedFile.value) return
  testLoading.value = true
  testError.value = ''
  revokeAudio()
  resetTranscription()

  try {
    const result = await props.onTest(mode.value === 'synthesis' ? testText.value : selectedFile.value as File, buildConfig())

    if (mode.value === 'synthesis') {
      const blob = result as Blob
      audioUrl.value = URL.createObjectURL(blob)
      await new Promise<void>((resolve) => setTimeout(resolve, 50))
      audioEl.value?.play()
    } else {
      const payload = result as { text?: string, language?: string }
      transcriptionText.value = payload.text ?? ''
      transcriptionLanguage.value = payload.language ?? ''
    }
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : t(mode.value === 'transcription' ? 'transcription.test.failed' : 'speech.test.failed')
    testError.value = msg
    toast.error(msg)
  } finally {
    testLoading.value = false
  }
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] ?? null
  selectedFile.value = file
  selectedFileName.value = file?.name ?? ''
}
</script>
