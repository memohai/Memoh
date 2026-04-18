<template>
  <div class="space-y-4">
    <template v-if="orderedFields.length > 0">
      <section
        v-for="field in orderedFields"
        :key="field.key"
        class="space-y-2"
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
    </template>

    <div
      v-else
      class="text-xs text-muted-foreground"
    >
      {{ $t('speech.noCapabilities') }}
    </div>

    <Separator class="my-3" />

    <div class="space-y-3">
      <h4 class="text-xs font-medium">
        {{ $t('speech.test.title') }}
      </h4>
      <div class="relative">
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
      <div class="flex items-center gap-3">
        <LoadingButton
          type="button"
          variant="outline"
          size="sm"
          :loading="testLoading"
          :disabled="!testText.trim() || testText.length > maxTestTextLen"
          @click="handleTest"
        >
          <Play class="mr-1.5" />
          {{ $t('speech.test.generate') }}
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
import { Eye, EyeOff, Play } from 'lucide-vue-next'
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
  onTest: (text: string, config: Record<string, unknown>) => Promise<Blob>
}>()

const emit = defineEmits<{
  save: [config: Record<string, unknown>]
}>()

const { t } = useI18n()
const configData = reactive<Record<string, unknown>>({})
const visibleSecrets = reactive<Record<string, boolean>>({})
const saving = ref(false)
const testText = ref('')
const testLoading = ref(false)
const testError = ref('')
const audioUrl = ref('')
const audioEl = ref<HTMLAudioElement>()
const maxTestTextLen = 500

const orderedFields = computed(() => {
  const fields = props.schema?.fields ?? []
  return [...fields].sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
})

watch(() => props.config, (cfg) => {
  Object.keys(configData).forEach((key) => delete configData[key])
  Object.assign(configData, { ...(cfg ?? {}) })
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
  if (!testText.value.trim()) return
  testLoading.value = true
  testError.value = ''
  revokeAudio()

  try {
    const blob = await props.onTest(testText.value, buildConfig())

    audioUrl.value = URL.createObjectURL(blob)
    await new Promise<void>((resolve) => setTimeout(resolve, 50))
    audioEl.value?.play()
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : t('speech.test.failed')
    testError.value = msg
    toast.error(msg)
  } finally {
    testLoading.value = false
  }
}
</script>
