<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@felinic/ui'
import { getEmailProviders, getEmailProvidersMeta } from '@memohai/sdk'
import type { EmailProviderMeta, EmailProviderResponse } from '@memohai/sdk'
import { Plus, Search } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddEmailProvider from './components/add-email-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useRoutedViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import PageShell from '@/components/page-shell/index.vue'
import EmailProviderIcon from '@/components/email-provider-icon/index.vue'

const { t } = useI18n()
const queryCache = useQueryCache()
const EMAIL_PROVIDER_TYPES = ['generic', 'gmail', 'mailgun'] as const

const { data: providerData, isLoading: providersLoading } = useQuery({
  key: () => ['email-providers'],
  query: async () => {
    const { data } = await getEmailProviders({ throwOnError: true })
    return data
  },
})

const { data: providerMetaData } = useQuery({
  key: () => ['email-providers-meta'],
  query: async () => {
    const { data } = await getEmailProvidersMeta({ throwOnError: true })
    return data
  },
})

const curProvider = ref<EmailProviderResponse>()
provide('curEmailProvider', curProvider)

const searchQuery = ref('')
const openStatus = reactive({ addOpen: false })
const initialProvider = ref('')

const providers = computed<EmailProviderResponse[]>(() =>
  Array.isArray(providerData.value) ? providerData.value : [],
)
const providerMetas = computed<EmailProviderMeta[]>(() =>
  Array.isArray(providerMetaData.value) ? providerMetaData.value : [],
)
const availableTemplates = computed(() => EMAIL_PROVIDER_TYPES
  .filter(provider => !providers.value.some(instance => instance.provider === provider))
  .map(provider => ({
    provider,
    meta: providerMetas.value.find(item => item.provider === provider),
  })))

// Page-owned query key (unique under settings KeepAlive — see useViewSwap.ts).
const {
  view,
  direction,
  isDetailLoading,
  openDetail: openProvider,
  backToList: closeProvider,
} = useRoutedViewSwap({
  key: 'emailProvider',
  items: () => providers.value,
  selected: () => curProvider.value,
  select: provider => curProvider.value = provider,
  getRouteValue: provider => provider.id ?? '',
  isLoading: () => providersLoading.value,
  isReady: () => providerData.value !== undefined,
})

const showSearch = computed(() => providers.value.length + availableTemplates.value.length > 0)

const filteredProviders = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter(p =>
    (p.name ?? '').toLowerCase().includes(keyword)
    || (p.provider ?? '').toLowerCase().includes(keyword),
  )
})

const filteredTemplates = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return availableTemplates.value
  return availableTemplates.value.filter(template =>
    (template.meta?.display_name ?? '').toLowerCase().includes(keyword)
    || template.provider.toLowerCase().includes(keyword),
  )
})

function openAdd(provider: string) {
  initialProvider.value = provider
  openStatus.addOpen = true
}

// A provider may have been created in the add dialog — refresh on close.
watch(() => openStatus.addOpen, (isOpen, wasOpen) => {
  if (wasOpen && !isOpen) {
    queryCache.invalidateQueries({ key: ['email-providers'] })
  }
})
</script>

<template>
  <SwapTransition :direction="direction">
    <!-- Provider list -->
    <PageShell
      v-if="view === 'list'"
      :title="t('email.title')"
    >
      <template #actions>
        <div
          v-if="showSearch"
          class="w-44 sm:w-56"
        >
          <InputGroup class="w-full">
            <InputGroupAddon align="inline-start">
              <Search class="size-3.5 text-muted-foreground" />
            </InputGroupAddon>
            <InputGroupInput
              v-model="searchQuery"
              :placeholder="t('email.searchPlaceholder')"
            />
          </InputGroup>
        </div>
      </template>

      <div
        v-if="providers.length + availableTemplates.length > 0"
        class="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <BackendCard
          v-for="provider in filteredProviders"
          :key="provider.id"
          :name="provider.name ?? ''"
          @click="openProvider(provider)"
        >
          <template #leading>
            <span class="flex size-10 items-center justify-center rounded-full bg-muted">
              <EmailProviderIcon
                :provider="provider.provider"
                class="size-5 text-muted-foreground"
              />
            </span>
          </template>
        </BackendCard>

        <BackendCard
          v-for="template in filteredTemplates"
          :key="`template:${template.provider}`"
          :name="template.meta?.display_name ?? template.provider"
          :subtitle="t('provider.templateNotConfigured')"
          @click="openAdd(template.provider)"
        >
          <template #leading>
            <span class="flex size-10 items-center justify-center rounded-full bg-muted">
              <EmailProviderIcon
                :provider="template.provider"
                class="size-5 text-muted-foreground"
              />
            </span>
          </template>
          <template #trailing>
            <Plus class="size-4 shrink-0 text-muted-foreground" />
          </template>
        </BackendCard>
      </div>

      <AddEmailProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
        :initial-provider="initialProvider"
      />
    </PageShell>

    <!-- Provider detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('email.title')"
      :loading="isDetailLoading || !curProvider?.id"
      @back="closeProvider"
    >
      <ProviderSetting v-if="curProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
