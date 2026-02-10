<template>
  <div class="max-w-2xl space-y-6">
    <!-- Chat Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.chatModel') }}</Label>
      <ModelSelect
        v-model="form.chat_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.chatModel')"
      />
    </div>

    <!-- Memory Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.memoryModel') }}</Label>
      <ModelSelect
        v-model="form.memory_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.memoryModel')"
      />
    </div>

    <!-- Embedding Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.embeddingModel') }}</Label>
      <ModelSelect
        v-model="form.embedding_model_id"
        :models="models"
        :providers="providers"
        model-type="embedding"
        :placeholder="$t('bots.settings.embeddingModel')"
      />
    </div>

    <Separator />

    <!-- Max Context Load Time -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.maxContextLoadTime') }}</Label>
      <Input
        v-model.number="form.max_context_load_time"
        type="number"
        :min="0"
      />
    </div>

    <!-- Language -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.language') }}</Label>
      <Input
        v-model="form.language"
        type="text"
      />
    </div>

    <!-- Allow Guest -->
    <div class="flex items-center justify-between">
      <Label>{{ $t('bots.settings.allowGuest') }}</Label>
      <Switch
        :model-value="form.allow_guest"
        @update:model-value="(val) => form.allow_guest = !!val"
      />
    </div>

    <Separator />

    <!-- Save -->
    <div class="flex justify-end">
      <Button
        :disabled="!hasChanges || isLoading"
        @click="handleSave"
      >
        <Spinner v-if="isLoading" />
        {{ $t('bots.settings.save') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Input,
  Switch,
  Button,
  Separator,
  Spinner,
} from '@memoh/ui'
import { reactive, computed, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ModelSelect from './model-select.vue'
import { useBotSettings, useUpdateBotSettings, type BotSettings } from '@/composables/api/useBotSettings'
import { useAllModels } from '@/composables/api/useModels'
import { useAllProviders } from '@/composables/api/useProviders'
import type { Ref } from 'vue'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const botIdRef = computed(() => props.botId) as Ref<string>

// ---- Data ----
const { data: settings } = useBotSettings(botIdRef)
const { data: modelData } = useAllModels()
const { data: providerData } = useAllProviders()
const { mutateAsync: updateSettings, isLoading } = useUpdateBotSettings(botIdRef)

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])

// ---- Form ----
const form = reactive<BotSettings>({
  chat_model_id: '',
  memory_model_id: '',
  embedding_model_id: '',
  max_context_load_time: 0,
  language: '',
  allow_guest: false,
})

// 同步服务端数据到表单
watch(settings, (val) => {
  if (val) {
    form.chat_model_id = val.chat_model_id ?? ''
    form.memory_model_id = val.memory_model_id ?? ''
    form.embedding_model_id = val.embedding_model_id ?? ''
    form.max_context_load_time = val.max_context_load_time ?? 0
    form.language = val.language ?? ''
    form.allow_guest = val.allow_guest ?? false
  }
}, { immediate: true })

const hasChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  return (
    form.chat_model_id !== (s.chat_model_id ?? '')
    || form.memory_model_id !== (s.memory_model_id ?? '')
    || form.embedding_model_id !== (s.embedding_model_id ?? '')
    || form.max_context_load_time !== (s.max_context_load_time ?? 0)
    || form.language !== (s.language ?? '')
    || form.allow_guest !== (s.allow_guest ?? false)
  )
})

async function handleSave() {
  try {
    await updateSettings({ ...form })
    toast.success(t('bots.settings.saveSuccess'))
  } catch {
    return
  }
}
</script>
