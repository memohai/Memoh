<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('emailProvider.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('emailProvider.add')"
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
          /> {{ $t('emailProvider.add') }}
        </Button>
      </template>
      <template #body>
        <div class="flex-col gap-3 flex mt-4">
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label :for="componentField.id || 'email-provider-name'">
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'email-provider-name'"
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
              <Label :for="componentField.id || 'email-provider-type'">
                {{ $t('emailProvider.providerType') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    :id="componentField.id || 'email-provider-type'"
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
import { postEmailProviders, getEmailProvidersMeta } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const { data: providerMetas } = useQuery({
  key: () => ['email-providers-meta'],
  query: async () => {
    const { data } = await getEmailProvidersMeta({ throwOnError: true })
    return data
  },
})

const queryCache = useQueryCache()
const { mutateAsync: createMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postEmailProviders({ body: data as any, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['email-providers'] }),
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
