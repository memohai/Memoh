<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <div class="flex items-center gap-2">
        <FontAwesomeIcon
          :icon="['fas', 'volume-high']"
          class="size-5"
        />
        <div>
          <h2 class="text-base font-semibold">
            {{ curProvider?.name }}
          </h2>
          <p class="text-xs text-muted-foreground">
            {{ currentMeta?.display_name ?? curProvider?.provider }}
          </p>
        </div>
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit="handleSave">
      <div class="space-y-5">
        <section>
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label :for="componentField.id || 'tts-provider-name'">
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'tts-provider-name'"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </section>

        <Separator class="my-4" />

        <section v-if="caps">
          <h3 class="text-sm font-medium mb-4">
            {{ $t('ttsProvider.voiceSettings') }}
          </h3>

          <div class="space-y-4">
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
          </div>
        </section>

        <Separator class="my-4" />

        <!-- Test Synthesis -->
        <section>
          <h3 class="text-sm font-medium mb-4">
            {{ $t('ttsProvider.test.title') }}
          </h3>
          <div class="space-y-3">
            <div class="relative">
              <Textarea
                v-model="testText"
                :placeholder="$t('ttsProvider.test.placeholder')"
                :maxlength="maxTestTextLen"
                rows="3"
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
                @ended="onAudioEnded"
              />
            </div>
          </div>
        </section>
      </div>

      <section class="flex justify-end mt-6 gap-4">
        <ConfirmPopover
          :message="$t('ttsProvider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="handleDelete"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>
        <LoadingButton
          type="submit"
          :loading="editLoading"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </section>
    </form>
  </div>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Separator,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Slider,
  Textarea,
  Label,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import { computed, inject, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { putTtsProvidersById, deleteTtsProvidersById, getTtsProvidersMeta } from '@memoh/sdk'
import type { TtsProviderResponse, TtsProviderMetaResponse } from '@memoh/sdk'

const { t } = useI18n()
const curProvider = inject('curTtsProvider', ref<TtsProviderResponse>())
const curProviderId = computed(() => curProvider.value?.id)

const { data: metaList } = useQuery({
  key: () => ['tts-providers-meta'],
  query: async () => {
    const { data } = await getTtsProvidersMeta({ throwOnError: true })
    return data
  },
})

const currentMeta = computed<TtsProviderMetaResponse | null>(() => {
  if (!metaList.value || !curProvider.value?.provider) return null
  return (metaList.value as TtsProviderMetaResponse[]).find((m) => m.provider === curProvider.value?.provider) ?? null
})

const caps = computed(() => currentMeta.value?.capabilities)

const availableLanguages = computed(() => {
  if (!caps.value?.voices) return []
  const langs = new Set(caps.value.voices.map((v) => v.lang ?? '').filter(Boolean))
  return [...langs].sort()
})

const filteredVoices = computed(() => {
  if (!caps.value?.voices) return []
  const lang = configData.voice_lang
  if (!lang) return caps.value.voices
  return caps.value.voices.filter((v) => v.lang === lang)
})

function onLangChange(lang: string) {
  configData.voice_lang = lang
  const voices = caps.value?.voices?.filter((v) => v.lang === lang)
  if (voices && voices.length > 0 && !voices.some((v) => v.id === configData.voice_id)) {
    configData.voice_id = voices[0].id ?? ''
  }
}

const queryCache = useQueryCache()

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
}))

const form = useForm({ validationSchema: schema })

const configData = reactive<Record<string, any>>({})

let loadedProviderId = ''
watch(() => curProvider.value?.id, (id) => {
  if (!id || id === loadedProviderId) return
  loadedProviderId = id
  const p = curProvider.value
  if (p) {
    form.setValues({ name: p.name ?? '' })
    const cfg = p.config ?? {}
    Object.keys(configData).forEach((k) => delete configData[k])
    if (cfg.voice && typeof cfg.voice === 'object') {
      configData.voice_id = (cfg.voice as any).id ?? ''
      configData.voice_lang = (cfg.voice as any).lang ?? ''
    }
    if (cfg.format) configData.format = cfg.format
    if (cfg.speed != null) configData.speed = cfg.speed
    if (cfg.pitch != null) configData.pitch = cfg.pitch
    if (cfg.sample_rate != null) configData.sample_rate = cfg.sample_rate
  }
}, { immediate: true })

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

const { mutateAsync: submitUpdate, isLoading: editLoading } = useMutation({
  mutation: async (data: { name: string; config: Record<string, unknown> }) => {
    if (!curProviderId.value) return
    const { data: result } = await putTtsProvidersById({
      path: { id: curProviderId.value },
      body: { name: data.name, config: data.config },
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['tts-providers'] }),
})

const { mutateAsync: doDelete, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteTtsProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['tts-providers'] }),
})

const handleSave = form.handleSubmit(async (values) => {
  try {
    await submitUpdate({ name: values.name, config: buildConfig() })
    toast.success(t('provider.saveChanges'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  }
})

async function handleDelete() {
  try {
    await doDelete()
    toast.success(t('common.deleteSuccess'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  }
}

// --- Test Synthesis ---
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

function onAudioEnded() {
  // no-op, keep player visible for replay
}

async function handleTest() {
  if (!curProviderId.value || !testText.value.trim()) return
  testLoading.value = true
  testError.value = ''
  revokeAudio()

  try {
    const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'
    const token = localStorage.getItem('token')
    const resp = await fetch(`${apiBase}/tts-providers/${curProviderId.value}/test`, {
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
      try {
        msg = JSON.parse(errBody)?.message ?? errBody
      } catch {
        msg = errBody
      }
      throw new Error(msg)
    }
    const blob = await resp.blob()
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
