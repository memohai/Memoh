<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('email.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('email.add')"
      :submit-disabled="(form.meta.value.valid === false) || isLoading"
      :loading="isLoading"
      @submit="handleCreate"
    >
      <template #trigger>
        <span
          v-if="hideTrigger"
          class="hidden"
        />
        <Button
          v-else
          class="w-full shadow-none! text-muted-foreground h-9 px-3 rounded-md border-border bg-background hover:bg-accent"
          variant="outline"
        >
          <Plus
            class="mr-1 size-4"
          /> {{ $t('email.add') }}
        </Button>
      </template>
      <template #body>
        <FormStack class="mt-4">
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FieldStack
              :label="$t('common.name')"
              for="email-provider-name"
            >
              <FormControl>
                <Input
                  id="email-provider-name"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FieldStack>
          </FormField>
          <FormField
            v-slot="{ componentField }"
            name="provider"
          >
            <FieldStack
              :label="$t('email.providerType')"
              for="email-provider-type"
            >
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    id="email-provider-type"
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
                        <span class="flex items-center gap-2">
                          <EmailProviderIcon
                            :provider="meta.provider"
                            class="size-4 text-muted-foreground"
                          />
                          <span>{{ meta.display_name }}</span>
                        </span>
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormControl>
            </FieldStack>
          </FormField>
        </FormStack>
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
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from '@felinic/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { postEmailProviders, getEmailProvidersMeta } from '@memohai/sdk'
import type { EmailCreateProviderRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import FormStack from '@/components/settings/form-stack.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'
import EmailProviderIcon from '@/components/email-provider-icon/index.vue'

const open = defineModel<boolean>('open')
withDefaults(defineProps<{
  hideTrigger?: boolean
}>(), {
  hideTrigger: false,
})
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
    const { data: result } = await postEmailProviders({ body: data as EmailCreateProviderRequest, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['email-providers'] }),
})

const schema = toTypedSchema(z.object({
  name: z.string().min(1, t('email.nameRequired')),
  provider: z.string().min(1, t('email.providerTypeRequired')),
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
