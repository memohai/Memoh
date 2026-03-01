<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <div class="flex items-center gap-2">
        <SearchProviderLogo
          :provider="curProvider?.provider || ''"
          size="lg"
        />
        <h2 class="scroll-m-20 text-base font-semibold tracking-tight">
          {{ curProvider?.name }}
        </h2>
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit="editProvider">
      <div class="space-y-4">
        <section>
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label
                :for="componentField.id || 'search-provider-name'"
                class="scroll-m-20 font-semibold tracking-tight"
              >
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'search-provider-name'"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  :aria-label="$t('common.name')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </section>

        <section>
          <Label
            for="search-provider-type"
            class="scroll-m-20 font-semibold tracking-tight"
          >
            {{ $t('searchProvider.provider') }}
          </Label>
          <div class="mt-3 mb-4">
            <Select
              :model-value="form.values.provider"
              @update:model-value="(val) => form.setFieldValue('provider', val)"
            >
              <SelectTrigger
                id="search-provider-type"
                class="w-full"
                :aria-label="$t('searchProvider.provider')"
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
                    {{ $t(`searchProvider.providerNames.${type}`, type) }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </section>

        <Separator class="my-4" />

        <template v-if="form.values.provider === 'brave'">
          <BraveSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'bing'">
          <BingSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'google'">
          <GoogleSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'tavily'">
          <TavilySettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'sogou'">
          <SogouSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'serper'">
          <SerperSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'searxng'">
          <SearxngSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'jina'">
          <JinaSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'exa'">
          <ExaSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'bocha'">
          <BochaSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'duckduckgo'">
          <DuckduckgoSettings v-model="configProxy" />
        </template>
        <template v-else-if="form.values.provider === 'yandex'">
          <YandexSettings v-model="configProxy" />
        </template>
        <div
          v-else-if="form.values.provider"
          class="text-sm text-muted-foreground"
        >
          {{ $t('searchProvider.unsupportedProvider') }}
        </div>
      </div>

      <section class="flex justify-end mt-4 gap-4">
        <ConfirmPopover
          :message="$t('searchProvider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="deleteProvider"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
              :aria-label="$t('common.delete')"
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>

        <LoadingButton
          type="submit"
          :loading="editLoading"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </section>
    </form>
  </div>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Separator,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
  Label,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import BraveSettings from './brave-settings.vue'
import BingSettings from './bing-settings.vue'
import GoogleSettings from './google-settings.vue'
import TavilySettings from './tavily-settings.vue'
import SogouSettings from './sogou-settings.vue'
import SerperSettings from './serper-settings.vue'
import SearxngSettings from './searxng-settings.vue'
import JinaSettings from './jina-settings.vue'
import ExaSettings from './exa-settings.vue'
import BochaSettings from './bocha-settings.vue'
import DuckduckgoSettings from './duckduckgo-settings.vue'
import YandexSettings from './yandex-settings.vue'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import { computed, inject, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { putSearchProvidersById, deleteSearchProvidersById } from '@memoh/sdk'
import type { SearchprovidersGetResponse, SearchprovidersUpdateRequest } from '@memoh/sdk'

const PROVIDER_TYPES = ['brave', 'bing', 'google', 'tavily', 'sogou', 'serper', 'searxng', 'jina', 'exa', 'bocha', 'duckduckgo', 'yandex'] as const

const curProvider = inject('curSearchProvider', ref<SearchprovidersGetResponse>())
const curProviderId = computed(() => curProvider.value?.id)

const queryCache = useQueryCache()

// ---- form ----
const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  provider: z.string().min(1),
}))

const form = useForm({
  validationSchema: providerSchema,
})

// Store config separately since it varies by provider type
const configData = ref<Record<string, unknown>>({})

const configProxy = computed({
  get: () => configData.value,
  set: (val: Record<string, unknown>) => {
    configData.value = val
  },
})

watch(curProvider, (newVal) => {
  if (newVal) {
    form.setValues({
      name: newVal.name ?? '',
      provider: newVal.provider ?? '',
    })
    configData.value = { ...(newVal.config ?? {}) }
  }
}, { immediate: true })

// ---- mutations ----
const { mutate: submitUpdate, isLoading: editLoading } = useMutation({
  mutation: async (data: SearchprovidersUpdateRequest) => {
    if (!curProviderId.value) return
    const { data: result } = await putSearchProvidersById({
      path: { id: curProviderId.value },
      body: data,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['search-providers'] }),
})

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteSearchProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['search-providers'] }),
})

const editProvider = form.handleSubmit(async (values) => {
  submitUpdate({
    name: values.name,
    provider: values.provider as SearchprovidersUpdateRequest['provider'],
    config: configData.value,
  })
})
</script>
