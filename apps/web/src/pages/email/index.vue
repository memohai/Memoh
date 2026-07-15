<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Button,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@felinic/ui'
import { getEmailProviders } from '@memohai/sdk'
import type { EmailProviderResponse } from '@memohai/sdk'
import { Plus, Search } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import AddEmailProvider from './components/add-email-provider.vue'
import ProviderSetting from './components/provider-setting.vue'
import BackendCard from '@/components/settings/backend-card.vue'
import DetailPane from '@/components/settings/detail-pane.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import SwapTransition from '@/components/settings/swap-transition.vue'
import PageShell from '@/components/page-shell/index.vue'
import EmailProviderIcon from '@/components/email-provider-icon/index.vue'

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: providerData } = useQuery({
  key: () => ['email-providers'],
  query: async () => {
    const { data } = await getEmailProviders({ throwOnError: true })
    return data
  },
})

const curProvider = ref<EmailProviderResponse>()
provide('curEmailProvider', curProvider)

// 'emailProvider' query key (unique per settings page — see useViewSwap.ts):
// the open provider's ID lives in the URL, so a refresh restores the exact
// detail view, and re-clicking Email in the settings sidebar while a detail is
// open navigates back.
const { view, direction, queryValue, openDetail, backToList } = useViewSwap('emailProvider')
const searchQuery = ref('')
const openStatus = reactive({ addOpen: false })

const providers = computed<EmailProviderResponse[]>(() =>
  Array.isArray(providerData.value) ? providerData.value : [],
)

const showSearch = computed(() => providers.value.length > 0)

const filteredProviders = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  if (!keyword) return providers.value
  return providers.value.filter(p =>
    (p.name ?? '').toLowerCase().includes(keyword)
    || (p.provider ?? '').toLowerCase().includes(keyword),
  )
})

function openProvider(provider: EmailProviderResponse) {
  curProvider.value = provider
  openDetail(provider.id)
}

// Resolve the URL's provider ID against the loaded list: restores the open
// provider on refresh, follows refetched data, and falls back to the list if
// it was deleted while open. Only treat "not found" as deleted once data has
// actually arrived — the empty list during the initial fetch proves nothing.
watch([queryValue, providers], ([id, list]) => {
  if (!id) return
  const found = list.find(p => p.id === id)
  if (found) {
    curProvider.value = found
  }
  else if (providerData.value !== undefined) {
    backToList()
  }
}, { immediate: true })

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
        <Button @click="openStatus.addOpen = true">
          <Plus class="size-4" />
          {{ t('email.add') }}
        </Button>
      </template>

      <div
        v-if="providers.length > 0"
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

        <button
          type="button"
          class="group/add flex min-h-[4.5rem] items-center justify-center gap-2 rounded-[var(--radius-menu-shell)] border border-dashed border-border bg-background text-sm text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="openStatus.addOpen = true"
        >
          <Plus class="size-4" />
          {{ t('email.add') }}
        </button>
      </div>

      <Empty
        v-else
        class="rounded-[var(--radius-menu-shell)] border border-border py-16"
      >
        <EmptyHeader>
          <EmptyTitle>{{ t('email.emptyTitle') }}</EmptyTitle>
          <EmptyDescription>{{ t('email.emptyDescription') }}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openStatus.addOpen = true"
          >
            <Plus class="size-4" />
            {{ t('email.add') }}
          </Button>
        </EmptyContent>
      </Empty>

      <AddEmailProvider
        v-model:open="openStatus.addOpen"
        hide-trigger
      />
    </PageShell>

    <!-- Provider detail -->
    <DetailPane
      v-else
      width="narrow"
      :back-label="t('email.title')"
      @back="backToList()"
    >
      <ProviderSetting v-if="curProvider?.id" />
    </DetailPane>
  </SwapTransition>
</template>
