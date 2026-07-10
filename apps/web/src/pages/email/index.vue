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

// 'detail' query key: see useViewSwap.ts — makes re-clicking Email in the
// settings sidebar while a provider's detail is open actually navigate back.
const { view, direction, openDetail, backToList } = useViewSwap('detail')
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
  openDetail()
}

// Keep the open provider synced with refreshed data; if it was deleted while
// open, fall back to the list.
watch(providers, (list) => {
  const currentId = curProvider.value?.id
  if (!currentId) return
  const stillExists = list.find(p => p.id === currentId)
  if (stillExists) {
    curProvider.value = stillExists
  }
  else if (view.value === 'detail') {
    backToList()
  }
})

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
