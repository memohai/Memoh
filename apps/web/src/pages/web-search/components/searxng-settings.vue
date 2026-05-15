<template>
  <div class="grid gap-4 md:grid-cols-2">
    <div class="space-y-2 md:col-span-2">
      <Label for="searxng-base-url">{{ $t('common.baseUrl') }}</Label>
      <Input
        id="searxng-base-url"
        v-model="localConfig.base_url"
        :aria-label="$t('common.baseUrl')"
        placeholder="http://localhost:8080/search"
      />
    </div>
    <div class="space-y-2">
      <Label for="searxng-language">{{ $t('settings.language') }}</Label>
      <Input
        id="searxng-language"
        v-model="localConfig.language"
        :aria-label="$t('settings.language')"
        placeholder="all"
      />
    </div>
    <div class="space-y-2">
      <Label for="searxng-safesearch">{{ $t('common.safeSearch') }}</Label>
      <Input
        id="searxng-safesearch"
        v-model="localConfig.safesearch"
        :aria-label="$t('common.safeSearch')"
        placeholder="0, 1, or 2"
      />
    </div>
    <div class="space-y-2">
      <Label for="searxng-categories">{{ $t('common.categories') }}</Label>
      <Input
        id="searxng-categories"
        v-model="localConfig.categories"
        :aria-label="$t('common.categories')"
        placeholder="general"
      />
    </div>
    <div class="space-y-2">
      <Label for="searxng-timeout-seconds">{{ $t('common.timeoutSeconds') }}</Label>
      <Input
        id="searxng-timeout-seconds"
        v-model.number="localConfig.timeout_seconds"
        type="number"
        :min="1"
        :aria-label="$t('common.timeoutSeconds')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import { Input, Label } from '@memohai/ui'

const props = defineProps<{
  modelValue: Record<string, unknown>
}>()

const emit = defineEmits<{
  'update:modelValue': [value: Record<string, unknown>]
}>()

const localConfig = reactive({
  base_url: '',
  language: 'all',
  safesearch: '1',
  categories: 'general',
  timeout_seconds: 15,
})

watch(
  () => props.modelValue,
  (val) => {
    localConfig.base_url = String(val?.base_url ?? '')
    localConfig.language = String(val?.language ?? 'all')
    localConfig.safesearch = String(val?.safesearch ?? '1')
    localConfig.categories = String(val?.categories ?? 'general')
    const timeout = Number(val?.timeout_seconds ?? 15)
    localConfig.timeout_seconds = Number.isFinite(timeout) && timeout > 0 ? timeout : 15
  },
  { immediate: true, deep: true },
)

watch(localConfig, () => {
  emit('update:modelValue', {
    base_url: localConfig.base_url,
    language: localConfig.language,
    safesearch: localConfig.safesearch,
    categories: localConfig.categories,
    timeout_seconds: localConfig.timeout_seconds,
  })
}, { deep: true })
</script>
