<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('ttsProvider.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('ttsProvider.add')"
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
          /> {{ $t('ttsProvider.add') }}
        </Button>
      </template>
      <template #body>
        <div class="flex-col gap-3 flex mt-4">
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
          <FormField
            v-slot="{ componentField }"
            name="provider"
          >
            <FormItem>
              <Label :for="componentField.id || 'tts-provider-type'">
                {{ $t('ttsProvider.providerType') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    :id="componentField.id || 'tts-provider-type'"
                    class="w-full"
                  >
                    <SelectValue :placeholder="$t('common.typePlaceholder')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem
                        v-for="meta in providerMetas"
                        :key="meta.provider"
                        :value="meta.provider!"
                      >
                        {{ meta.display_name }}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
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
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
  Label,
} from '@memoh/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { postTtsProviders, getTtsProvidersMeta } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const { data: providerMetas } = useQuery({
  key: () => ['tts-providers-meta'],
  query: async () => {
    const { data } = await getTtsProvidersMeta({ throwOnError: true })
    return data
  },
})

const queryCache = useQueryCache()
const { mutateAsync: createMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postTtsProviders({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['tts-providers'] }),
})

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
  provider: z.string().min(1),
}))

const form = useForm({ validationSchema: schema })

const handleCreate = form.handleSubmit(async (value) => {
  await run(
    () => createMutation({ ...value, config: {} }),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => { open.value = false },
    },
  )
})
</script>
