<template>
  <FormDialogShell
    v-model:open="open"
    :title="$t('ttsProvider.addModel')"
    :cancel-text="$t('common.cancel')"
    :submit-text="$t('ttsProvider.addModel')"
    :submit-disabled="(form.meta.value.valid === false) || isLoading"
    :loading="isLoading"
    @submit="handleCreate"
  >
    <template #trigger>
      <Button variant="default">
        {{ $t('ttsProvider.addModel') }}
      </Button>
    </template>
    <template #body>
      <div class="flex-col gap-3 flex mt-4">
        <FormField
          v-slot="{ componentField }"
          name="model_id"
        >
          <FormItem>
            <Label :for="componentField.id || 'tts-model-id'">
              {{ $t('ttsProvider.modelId') }}
            </Label>
            <FormControl>
              <Input
                :id="componentField.id || 'tts-model-id'"
                type="text"
                :placeholder="$t('ttsProvider.modelIdPlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <Label :for="componentField.id || 'tts-model-name'">
              {{ $t('common.name') }}
            </Label>
            <FormControl>
              <Input
                :id="componentField.id || 'tts-model-name'"
                type="text"
                :placeholder="$t('common.namePlaceholder')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </div>
    </template>
  </FormDialogShell>
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
import { postTtsModels } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const props = defineProps<{
  providerId: string
}>()

const emit = defineEmits<{
  created: []
}>()

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const queryCache = useQueryCache()
const { mutateAsync: createMutation, isLoading } = useMutation({
  mutation: async (data: { model_id: string; name: string }) => {
    const { data: result } = await postTtsModels({
      body: {
        model_id: data.model_id,
        name: data.name,
        tts_provider_id: props.providerId,
      },
      throwOnError: true,
    })
    return result
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['tts-provider-models'] })
    queryCache.invalidateQueries({ key: ['tts-models'] })
  },
})

const schema = toTypedSchema(z.object({
  model_id: z.string().min(1),
  name: z.string(),
}))

const form = useForm({
  validationSchema: schema,
  initialValues: { model_id: '', name: '' },
})

const handleCreate = form.handleSubmit(async (value) => {
  await run(
    () => createMutation({ model_id: value.model_id, name: value.name ?? '' }),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
        emit('created')
      },
    },
  )
})
</script>
