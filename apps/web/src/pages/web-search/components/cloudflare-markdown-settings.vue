<template>
  <div class="grid gap-4 md:grid-cols-2">
    <div class="space-y-2 md:col-span-2">
      <Label for="cloudflare-account-id">Account ID</Label>
      <Input
        id="cloudflare-account-id"
        v-model="localConfig.account_id"
        aria-label="Account ID"
      />
    </div>
    <div class="space-y-2 md:col-span-2">
      <Label for="cloudflare-api-token">API Token</Label>
      <Input
        id="cloudflare-api-token"
        v-model="localConfig.api_token"
        type="password"
        aria-label="API Token"
      />
    </div>
    <div class="space-y-2 md:col-span-2">
      <Label for="cloudflare-base-url">Base URL</Label>
      <Input
        id="cloudflare-base-url"
        v-model="localConfig.base_url"
        aria-label="Base URL"
      />
    </div>
    <div class="space-y-2">
      <Label for="cloudflare-timeout-seconds">Timeout (seconds)</Label>
      <Input
        id="cloudflare-timeout-seconds"
        v-model.number="localConfig.timeout_seconds"
        type="number"
        :min="1"
        aria-label="Timeout (seconds)"
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
  account_id: '',
  api_token: '',
  base_url: 'https://api.cloudflare.com/client/v4',
  timeout_seconds: 30,
})

watch(
  () => props.modelValue,
  (val) => {
    localConfig.account_id = String(val?.account_id ?? '')
    localConfig.api_token = String(val?.api_token ?? '')
    localConfig.base_url = String(val?.base_url ?? 'https://api.cloudflare.com/client/v4')
    const timeout = Number(val?.timeout_seconds ?? 30)
    localConfig.timeout_seconds = Number.isFinite(timeout) && timeout > 0 ? timeout : 30
  },
  { immediate: true, deep: true },
)

watch(localConfig, () => {
  emit('update:modelValue', {
    account_id: localConfig.account_id,
    api_token: localConfig.api_token,
    base_url: localConfig.base_url,
    timeout_seconds: localConfig.timeout_seconds,
  })
}, { deep: true })
</script>
