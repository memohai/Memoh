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
        <div
          class="flex-col gap-3 flex mt-4"
        >
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label
                class="mb-2"
                for="provider-create-name"
              >
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-name"
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
                for="provider-create-api-key"
              >
                {{ $t('provider.apiKey') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-api-key"
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
                for="provider-create-base-url"
              >
                {{ $t('provider.url') }}
              </Label>
              <FormControl>
                <Input
                  id="provider-create-base-url"
                  type="text"
                  :placeholder="$t('provider.urlPlaceholder')"
                  v-bind="componentField"
                  :aria-label="$t('provider.url')"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <Separator />

          <FormField
            v-slot="{ value, handleChange }"
            name="auto_import"
          >
            <FormItem class="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm">
              <div class="space-y-0.5">
                <Label class="text-base">
                  {{ $t('provider.autoImport') }}
                </Label>
                <p class="text-[0.8rem] text-muted-foreground">
                  {{ $t('provider.autoImportHint') }}
                </p>
              </div>
              <FormControl>
                <Switch
                  :model-value="value"
                  @update:model-value="handleChange"
                />
              </FormControl>
            </FormItem>
          </FormField>

          <FormField
            v-if="form.values.auto_import"
            v-slot="{ value, handleChange }"
            name="client_type"
          >
            <FormItem>
              <Label class="mb-2">
                {{ $t('models.importClientType') }}
              </Label>
              <FormControl>
                <SearchableSelectPopover
                  :model-value="value"
                  :options="CLIENT_TYPE_LIST"
                  :placeholder="$t('models.clientTypePlaceholder')"
                  @update:model-value="handleChange"
                />
              </FormControl>
              <p class="text-[0.8rem] text-muted-foreground">
                {{ $t('models.importClientTypeHint') }}
              </p>
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
  Switch,
  Separator,
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm,Form,Field } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postProviders, postProvidersByIdImportModels } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { CLIENT_TYPE_LIST } from '@/constants/client-types'
import { toast } from 'vue-sonner'

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const queryCache = useQueryCache()
const { mutateAsync: createProviderMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const payload = {
      ...data,
      metadata: { additionalProp1: {} },
    }
    const { data: result } = await postProviders({ body: payload as any, throwOnError: true })
    if (data.auto_import && result?.id) {
      try {
        const { data: importResult } = await postProvidersByIdImportModels({
          path: { id: result.id },
          body: { client_type: data.client_type as string },
          throwOnError: true,
        })
        if (importResult) {
          toast.success(t('models.importSuccess', {
            created: importResult.created,
            skipped: importResult.skipped,
          }))
        }
      }
      catch (e) {
        console.error('Auto import failed:', e)
        toast.error(t('models.importFailed'))
      }
    }
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  api_key: z.string().min(1),
  base_url: z.string().min(1),
  name: z.string().min(1),
  auto_import: z.boolean().optional(),
  client_type: z.string().optional(),
}))

const form = useForm({
  validationSchema: providerSchema,
  initialValues: {
    auto_import: false,
    client_type: 'openai-completions',
  },
})

const createProvider = form.handleSubmit(async (value) => {
  await run(
    () => createProviderMutation(value),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {      
        open.value = false
        form.resetForm()
      },
    },
  )
})
</script>
