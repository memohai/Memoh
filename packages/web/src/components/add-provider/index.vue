<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('provider.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('provider.add')"
      :submit-disabled="(form.meta.value.valid === false) || isLoading"
      :loading="isLoading"
      @submit="createProvider"
    >
      <template #trigger>
        <Button
          class="w-full shadow-none! text-muted-foreground mb-4"
          variant="outline"
        >
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-1"
          /> {{ $t('provider.addBtn') }}
        </Button>
      </template>
      <template #body>
        <div class="flex-col gap-3 flex mt-4">
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  :for="componentField.id || 'provider-create-name'"
                >
                  {{ $t('common.name') }}
                </Label>
                <FormControl>
                  <Input
                    :id="componentField.id || 'provider-create-name'"
                    type="text"
                    :placeholder="$t('common.namePlaceholder')"
                    v-bind="componentField"
                    :aria-label="$t('common.name')"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="api_key"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  :for="componentField.id || 'provider-create-api-key'"
                >
                  {{ $t('provider.apiKey') }}
                </Label>
                <FormControl>
                  <Input
                    :id="componentField.id || 'provider-create-api-key'"
                    type="text"
                    :placeholder="$t('provider.apiKeyPlaceholder')"
                    v-bind="componentField"
                    :aria-label="$t('provider.apiKey')"
                  />
                </FormControl>
              </FormItem>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="base_url"
            >
              <FormItem>
                <Label
                  class="mb-2"
                  :for="componentField.id || 'provider-create-base-url'"
                >
                  {{ $t('provider.url') }}
                </Label>
                <FormControl>
                  <Input
                    :id="componentField.id || 'provider-create-base-url'"
                    type="text"
                    :placeholder="$t('provider.urlPlaceholder')"
                    v-bind="componentField"
                    :aria-label="$t('provider.url')"
                  />
                </FormControl>
              </FormItem>
            </FormField>
          </div>
      </template>
    </FormDialogShell>
  </section>
</template>
<script setup lang="ts">
import {
  Button,
  Input,
  FormField,
  FormControl,
  FormItem,
  Label,
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProviders } from '@memoh/sdk'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'

const open = defineModel<boolean>('open')

const queryCache = useQueryCache()
const { mutate: providerFetch, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postProviders({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  api_key: z.string().min(1),
  base_url: z.string().min(1),
  name: z.string().min(1),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}))

const form = useForm({
  validationSchema: providerSchema,
})

const createProvider = form.handleSubmit(async (value) => {
  try {
    await providerFetch(value)
    open.value = false
  } catch {
    return
  }
})
</script>
