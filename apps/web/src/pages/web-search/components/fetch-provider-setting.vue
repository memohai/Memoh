<template>
  <SettingsShell width="narrow">
    <div class="space-y-6">
      <!-- Identity card mirrors the search provider detail, while Native keeps
           its managed/non-destructive behavior. -->
      <section class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
        <span class="flex size-9 shrink-0 items-center justify-center">
          <SearchProviderLogo
            :provider="curProvider?.provider || ''"
            size="md"
          />
        </span>
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-sm font-semibold">
            {{ curProvider?.name }}
          </h2>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <ConfirmPopover
            v-if="!isNative"
            :message="$t('webSearch.deleteFetchConfirm')"
            :loading="deleteLoading"
            variant="destructive"
            @confirm="deleteProvider"
          >
            <template #trigger>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                class="text-muted-foreground hover:text-destructive"
                :aria-label="$t('common.delete')"
              >
                <Trash2 class="size-4" />
              </Button>
            </template>
          </ConfirmPopover>
          <Switch
            :model-value="curProvider?.enable ?? false"
            :disabled="isNative || !curProvider?.id || enableLoading"
            :aria-label="$t('common.enable')"
            @update:model-value="handleToggleEnable"
          />
        </div>
      </section>

      <SettingsSection :title="$t('provider.configurationTitle')">
        <div
          v-if="isNative"
          class="px-4 py-3 text-xs text-muted-foreground"
        >
          {{ $t('webSearch.nativeManaged') }}
        </div>

        <form
          v-else
          @submit="editProvider"
        >
          <div>
            <FormField
              v-slot="{ componentField }"
              name="name"
            >
              <SettingsRow :label="$t('common.name')">
                <FormItem class="w-80">
                  <FormControl>
                    <Input
                      type="text"
                      :placeholder="$t('common.namePlaceholder')"
                      :aria-label="$t('common.name')"
                      v-bind="componentField"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              </SettingsRow>
            </FormField>

            <template v-if="form.values.provider === 'jina'">
              <JinaReaderSettings v-model="configProxy" />
            </template>
            <template v-else-if="form.values.provider === 'cloudflare_markdown'">
              <CloudflareMarkdownSettings v-model="configProxy" />
            </template>
            <div
              v-else-if="form.values.provider"
              class="px-4 py-3 text-xs text-muted-foreground"
            >
              {{ $t('webSearch.unsupportedProvider') }}
            </div>
          </div>

          <div class="mx-4 flex items-center justify-end border-t border-border py-3">
            <LoadingButton
              type="submit"
              size="sm"
              :loading="editLoading"
            >
              {{ $t('provider.saveChanges') }}
            </LoadingButton>
          </div>
        </form>
      </SettingsSection>
    </div>
  </SettingsShell>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
  Switch,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import JinaReaderSettings from './jina-reader-settings.vue'
import CloudflareMarkdownSettings from './cloudflare-markdown-settings.vue'
import { Trash2 } from 'lucide-vue-next'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import { computed, inject, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQueryCache } from '@pinia/colada'
import { putFetchProvidersById, deleteFetchProvidersById } from '@memohai/sdk'
import type { FetchprovidersGetResponse, FetchprovidersUpdateRequest } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'

const { t } = useI18n()
const curProvider = inject('curFetchProvider', ref<FetchprovidersGetResponse>())
const curProviderId = computed(() => curProvider.value?.id)
const isNative = computed(() => curProvider.value?.provider === 'native')
const enableLoading = ref(false)

const queryCache = useQueryCache()

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  provider: z.string().min(1),
}))

const form = useForm({
  validationSchema: providerSchema,
})

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

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value || isNative.value) return

  const prev = curProvider.value.enable ?? false
  curProvider.value = { ...curProvider.value, enable: value }

  enableLoading.value = true
  try {
    await putFetchProvidersById({
      path: { id: curProviderId.value },
      body: { enable: value },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['fetch-providers'] })
  } catch {
    curProvider.value = { ...curProvider.value, enable: prev }
    toast.error(t('common.saveFailed'))
  } finally {
    enableLoading.value = false
  }
}

const { mutate: submitUpdate, isLoading: editLoading } = useMutation({
  mutation: async (data: FetchprovidersUpdateRequest) => {
    if (!curProviderId.value) return
    const { data: result } = await putFetchProvidersById({
      path: { id: curProviderId.value },
      body: data,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['fetch-providers'] }),
})

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value || isNative.value) return
    await deleteFetchProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['fetch-providers'] }),
})

const editProvider = form.handleSubmit(async (values) => {
  submitUpdate({
    name: values.name,
    provider: values.provider as FetchprovidersUpdateRequest['provider'],
    config: configData.value,
  })
})
</script>
