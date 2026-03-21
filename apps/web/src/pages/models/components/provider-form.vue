<template>
  <form @submit="editProvider">
    <div class="space-y-4">
      <section class="space-y-2">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('common.name') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('common.namePlaceholder')"
                :aria-label="$t('common.name')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section class="space-y-2">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.apiKey') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormControl>
              <Input
                type="password"
                :placeholder="props.provider?.api_key || $t('provider.apiKeyPlaceholder')"
                :aria-label="$t('provider.apiKey')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section class="space-y-2">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.url') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('provider.urlPlaceholder')"
                :aria-label="$t('provider.url')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
    </div>

    <section class="flex justify-between items-center mt-4">
      <LoadingButton
        type="button"
        variant="outline"
        :loading="testLoading"
        :disabled="!props.provider?.id"
        @click="runTest"
      >
        <FontAwesomeIcon
          v-if="!testLoading"
          :icon="['fas', 'rotate']"
        />
        {{ $t('provider.testConnection') }}
      </LoadingButton>

      <div class="flex gap-4">
        <ConfirmPopover
          :message="$t('provider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="$emit('delete')"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
              :aria-label="$t('common.delete')"
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>

        <LoadingButton
          type="submit"
          :loading="editLoading"
          :disabled="!hasChanges || !form.meta.value.valid"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </div>
    </section>

    <section
      v-if="testResult"
      class="mt-4 rounded-lg border p-4 space-y-3 text-sm"
    >
      <div class="flex items-center gap-2">
        <StatusDot :status="testResult.reachable ? 'success' : 'error'" />
        <span class="font-medium">
          {{ testResult.reachable ? $t('provider.reachable') : $t('provider.unreachable') }}
        </span>
        <span
          v-if="testResult.latency_ms"
          class="text-muted-foreground"
        >
          {{ testResult.latency_ms }}ms
        </span>
      </div>

      <div
        v-if="testResult.message"
        class="text-muted-foreground text-xs"
      >
        {{ testResult.message }}
      </div>

      <div
        v-if="testError"
        class="text-destructive text-xs"
      >
        {{ testError }}
      </div>
    </section>
  </form>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import StatusDot from '@/components/status-dot/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import { computed, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { postProvidersByIdTest } from '@memoh/sdk'
import type { ProvidersGetResponse, ProvidersTestResponse } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  provider: Partial<ProvidersGetResponse> | undefined
  editLoading: boolean
  deleteLoading: boolean
}>()

const emit = defineEmits<{
  submit: [values: Record<string, unknown>]
  delete: []
}>()

const testLoading = ref(false)
const testResult = ref<ProvidersTestResponse | null>(null)
const testError = ref('')

async function runTest() {
  if (!props.provider?.id) return
  testLoading.value = true
  testResult.value = null
  testError.value = ''
  try {
    const { data } = await postProvidersByIdTest({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    testResult.value = data ?? null
  } catch (err: unknown) {
    testError.value = err instanceof Error ? err.message : t('provider.testFailed')
  } finally {
    testLoading.value = false
  }
}

watch(() => props.provider?.id, () => {
  testResult.value = null
  testError.value = ''
})

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  base_url: z.string().min(1),
  api_key: z.string().optional(),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}))

const form = useForm({
  validationSchema: providerSchema,
})

watch(() => props.provider, (newVal) => {
  if (newVal) {
    form.setValues({
      name: newVal.name,
      base_url: newVal.base_url,
      api_key: '',
    })
  }
}, { immediate: true })

const hasChanges = computed(() => {
  const raw = props.provider
  const baseChanged = JSON.stringify({
    name: form.values.name,
    base_url: form.values.base_url,
    metadata: form.values.metadata,
  }) !== JSON.stringify({
    name: raw?.name,
    base_url: raw?.base_url,
    metadata: { additionalProp1: {} },
  })

  const apiKeyChanged = Boolean(form.values.api_key && form.values.api_key.trim() !== '')
  return baseChanged || apiKeyChanged
})

const editProvider = form.handleSubmit(async (value) => {
  const payload: Record<string, unknown> = {
    name: value.name,
    base_url: value.base_url,
    metadata: value.metadata,
  }
  if (value.api_key && value.api_key.trim() !== '') {
    payload.api_key = value.api_key
  }
  emit('submit', payload)
})
</script>
