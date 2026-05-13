<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('webSearch.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('webSearch.add')"
      :submit-disabled="(form.meta.value.valid === false) || isLoading"
      :loading="isLoading"
      @submit="handleCreate"
    >
      <template #trigger>
        <Button
          class="w-full shadow-none! text-muted-foreground h-9 px-3 rounded-md border-border bg-background hover:bg-accent"
          variant="outline"
        >
          <Plus class="mr-1 size-4" /> {{ $t('webSearch.add') }}
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
                :for="'search-provider-create-name'"
              >
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="'search-provider-create-name'"
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
                :for="'search-provider-create-type'"
              >
                {{ $t('webSearch.provider') }}
              </Label>
              <FormControl>
                <Select v-bind="componentField">
                  <SelectTrigger
                    :id="'search-provider-create-type'"
                    class="w-full"
                    :aria-label="$t('webSearch.provider')"
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
                        {{ $t(`webSearch.providerNames.${type}`, type) }}
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
import { postSearchProviders } from '@memohai/sdk'
import type { SearchprovidersCreateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import FormDialogShell from '@/components/form-dialog-shell/index.vue'
import { useDialogMutation } from '@/composables/useDialogMutation'

const PROVIDER_TYPES = ['brave', 'bing', 'google', 'tavily', 'sogou', 'serper', 'searxng', 'jina', 'exa', 'bocha', 'duckduckgo', 'yandex'] as const

const open = defineModel<boolean>('open')
const { t } = useI18n()
const { run } = useDialogMutation()

const queryCache = useQueryCache()
const { mutateAsync: createProviderMutation, isLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    const { data: result } = await postSearchProviders({ body: data as SearchprovidersCreateRequest, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['search-providers'] }),
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
