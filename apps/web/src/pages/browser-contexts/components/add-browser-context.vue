<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('browserContext.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('browserContext.add')"
      :submit-disabled="(form.meta.value.valid === false) || isLoading"
      :loading="isLoading"
      @submit="handleCreate"
    >
      <template #trigger>
        <Button
          class="w-full shadow-none! text-muted-foreground mb-4"
          variant="outline"
        >
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-1"
          /> {{ $t('browserContext.add') }}
        </Button>
      </template>
      <template #body>
        <div class="flex-col gap-3 flex mt-4">
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label :for="componentField.id || 'browser-context-name'">
                {{ $t('browserContext.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'browser-context-name'"
                  type="text"
                  :placeholder="$t('browserContext.namePlaceholder')"
                  v-bind="componentField"
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
import { postBrowserContexts } from '@memoh/sdk'
import type { BrowsercontextsCreateRequest } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const queryCache = useQueryCache()
const { mutateAsync: createMutation, isLoading } = useMutation({
  mutation: async (data: { name: string }) => {
    const { data: result } = await postBrowserContexts({
      body: { name: data.name } as BrowsercontextsCreateRequest,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['browser-contexts'] }),
})

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
}))

const form = useForm({ validationSchema: schema })

const handleCreate = form.handleSubmit(async (value) => {
  await run(
    () => createMutation(value),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => { open.value = false },
    },
  )
})
</script>
