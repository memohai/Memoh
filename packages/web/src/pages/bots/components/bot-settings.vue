<template>
  <div class="max-w-2xl mx-auto space-y-6">
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

    <!-- Search Provider -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.searchProvider') }}</Label>
      <SearchProviderSelect
        v-model="form.search_provider_id"
        :providers="searchProviders"
        :placeholder="$t('bots.settings.searchProviderPlaceholder')"
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
        :aria-label="$t('bots.settings.maxContextLoadTime')"
      />
    </div>

    <!-- Max Context Tokens -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.maxContextTokens') }}</Label>
      <Input
        v-model.number="form.max_context_tokens"
        type="number"
        :min="0"
        placeholder="0"
        :aria-label="$t('bots.settings.maxContextTokens')"
      />
    </div>

    <!-- Language -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.language') }}</Label>
      <Input
        v-model="form.language"
        type="text"
        :aria-label="$t('bots.settings.language')"
      />
    </div>

    <!-- Allow Guest: only for public bot -->
    <template v-if="isPublicBot">
      <div class="flex items-center justify-between">
        <Label>{{ $t('bots.settings.allowGuest') }}</Label>
        <Switch
          :model-value="form.allow_guest"
          @update:model-value="(val) => form.allow_guest = !!val"
        />
      </div>
      <Separator />
    </template>

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

    <Separator />

    <!-- Danger Zone -->
    <div
      class="rounded-lg border border-destructive/50 bg-destructive/5 p-4 space-y-3"
    >
      <h3 class="text-sm font-semibold text-destructive">
        {{ $t('bots.settings.dangerZone') }}
      </h3>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.settings.deleteBotDescription') }}
      </p>
      <div class="flex items-center justify-end">
        <ConfirmPopover
          :message="$t('bots.deleteConfirm')"
          :loading="deleteLoading"
          :confirm-text="$t('common.delete')"
          @confirm="handleDeleteBot"
        >
          <template #trigger>
            <Button
              variant="destructive"
              :disabled="deleteLoading"
            >
              <Spinner
                v-if="deleteLoading"
                class="mr-1.5"
              />
              {{ $t('bots.settings.deleteBot') }}
            </Button>
          </template>
        </ConfirmPopover>
      </div>
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
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from './model-select.vue'
import SearchProviderSelect from './search-provider-select.vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings, deleteBotsById, getModels, getProviders, getSearchProviders } from '@memoh/sdk'
import type { SettingsSettings } from '@memoh/sdk'
import type { Ref } from 'vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
  botType?: string
}>()

const isPublicBot = computed(() => props.botType === 'public')

const { t } = useI18n()
const router = useRouter()

const botIdRef = computed(() => props.botId) as Ref<string>

// ---- Data ----
const queryCache = useQueryCache()

const { data: settings } = useQuery({
  key: () => ['bot-settings', botIdRef.value],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({ path: { bot_id: botIdRef.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botIdRef.value,
})

const { data: modelData } = useQuery({
  key: ['all-models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['all-providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: searchProviderData } = useQuery({
  key: ['all-search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({ throwOnError: true })
    return data
  },
})

const { mutateAsync: updateSettings, isLoading } = useMutation({
  mutation: async (body: Partial<SettingsSettings>) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] }),
})

const { mutateAsync: deleteBot, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    await deleteBotsById({ path: { id: botIdRef.value }, throwOnError: true })
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bots'] })
    queryCache.invalidateQueries({ key: ['bot'] })
  },
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])
const searchProviders = computed(() => searchProviderData.value ?? [])

// ---- Form ----
const form = reactive<SettingsSettings>({
  chat_model_id: '',
  memory_model_id: '',
  embedding_model_id: '',
  search_provider_id: '',
  max_context_load_time: 0,
  max_context_tokens: 0,
  language: '',
  allow_guest: false,
})

// 同步服务端数据到表单
watch(settings, (val) => {
  if (val) {
    form.chat_model_id = val.chat_model_id ?? ''
    form.memory_model_id = val.memory_model_id ?? ''
    form.embedding_model_id = val.embedding_model_id ?? ''
    form.search_provider_id = val.search_provider_id ?? ''
    form.max_context_load_time = val.max_context_load_time ?? 0
    form.max_context_tokens = val.max_context_tokens ?? 0
    form.language = val.language ?? ''
    form.allow_guest = val.allow_guest ?? false
  }
}, { immediate: true })

const hasChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  let changed =
    form.chat_model_id !== (s.chat_model_id ?? '')
    || form.memory_model_id !== (s.memory_model_id ?? '')
    || form.embedding_model_id !== (s.embedding_model_id ?? '')
    || form.search_provider_id !== (s.search_provider_id ?? '')
    || form.max_context_load_time !== (s.max_context_load_time ?? 0)
    || form.max_context_tokens !== (s.max_context_tokens ?? 0)
    || form.language !== (s.language ?? '')
  if (isPublicBot.value) {
    changed = changed || form.allow_guest !== (s.allow_guest ?? false)
  }
  return changed
})

async function handleSave() {
  try {
    await updateSettings({ ...form })
    toast.success(t('bots.settings.saveSuccess'))
  } catch {
    return
  }
}

async function handleDeleteBot() {
  try {
    await deleteBot()
    toast.success(t('bots.deleteSuccess'))
    await router.push({ name: 'bots' })
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.lifecycle.deleteFailed')))
  }
}
</script>
