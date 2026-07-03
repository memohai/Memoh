<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('webSearch.addSearch')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('webSearch.addSearch')"
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
          <Plus class="mr-1 size-4" /> {{ $t('webSearch.addSearch') }}
        </Button>
      </template>
      <template #body>
        <div class="mt-4">
          <FormStack>
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <FieldStack
                :label="$t('common.name')"
                for="search-provider-create-name"
              >
                <FormControl>
                  <Input
                    id="search-provider-create-name"
                    type="text"
                    :placeholder="$t('common.namePlaceholder')"
                    v-bind="componentField"
                    :aria-label="$t('common.name')"
                  />
                </FormControl>
              </FieldStack>
            </FormField>
            <FormField
              v-slot="{ componentField }"
              name="provider"
            >
              <FieldStack
                :label="$t('webSearch.provider')"
                for="search-provider-create-type"
              >
                <FormControl>
                  <Select v-bind="componentField">
                    <SelectTrigger
                      id="search-provider-create-type"
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
                          <span class="flex items-center gap-2">
                            <SearchProviderLogo
                              :provider="type"
                              size="xs"
                            />
                            <span>{{ $t(`webSearch.providerNames.${type}`, type) }}</span>
                          </span>
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
              </FieldStack>
            </FormField>
          </FormStack>
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
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
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
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import FormStack from '@/components/settings/form-stack.vue'

const PROVIDER_TYPES = ['brave', 'bing', 'google', 'tavily', 'sogou', 'serper', 'searxng', 'jina', 'exa', 'bocha', 'duckduckgo', 'yandex'] as const

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
    const { data: result } = await postSearchProviders({ body: data as SearchprovidersCreateRequest, throwOnError: true })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['search-providers'] }),
})

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1, t('webSearch.nameRequired')),
  provider: z.string().min(1, t('webSearch.providerRequired')),
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
