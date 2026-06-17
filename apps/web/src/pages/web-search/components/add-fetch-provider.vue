<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('webSearch.addFetch')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('webSearch.addFetch')"
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
          <Plus class="mr-1 size-4" /> {{ $t('webSearch.addFetch') }}
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
                :for="'fetch-provider-create-name'"
              >
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="'fetch-provider-create-name'"
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
            name="provider"
          >
            <FormItem>
              <Label
                class="mb-2"
                :for="'fetch-provider-create-type'"
              >
                {{ $t('webSearch.fetchProvider') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    :id="'fetch-provider-create-type'"
                    class="w-full"
                    :aria-label="$t('webSearch.fetchProvider')"
                  >
                    <SelectValue :placeholder="$t('common.typePlaceholder')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem
                        v-for="type in PROVIDER_TYPES"
                        :key="type"
                        :value="type"
                      >
                        {{ $t(`webSearch.fetchProviderNames.${type}`, type) }}
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
} from '@memohai/ui'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { postFetchProviders } from '@memohai/sdk'
import type { FetchprovidersCreateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const PROVIDER_TYPES = ['jina', 'cloudflare_markdown'] as const

const open = defineModel<boolean>('open')
withDefaults(defineProps<{
  hideTrigger?: boolean
}>(), {
  hideTrigger: false,
})
const { t } = useI18n()
const { run } = useDialogMutation()

const queryCache = useQueryCache()
const { mutateAsync: createProviderMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postFetchProviders({ body: data as FetchprovidersCreateRequest, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['fetch-providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  provider: z.string().min(1),
}))

const form = useForm({
  validationSchema: providerSchema,
})

const handleCreate = form.handleSubmit(async (value) => {
  await run(
    () => createProviderMutation({ ...value, config: {} }),
    {
      fallbackMessage: t('common.saveFailed'),
      onSuccess: () => {
        open.value = false
      },
    },
  )
})
</script>
