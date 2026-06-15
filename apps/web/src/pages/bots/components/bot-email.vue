<template>
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <div class="mb-6 px-2">
      <h1 class="text-lg font-semibold text-foreground">
        {{ $t('bots.email.title') }}
      </h1>
      <p class="mt-1 text-xs text-muted-foreground">
        {{ $t('bots.email.subtitle') }}
      </p>
    </div>

    <div class="space-y-8">
      <SettingsSection :title="$t('bots.email.bindings')">
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.email.bindings') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ $t('bots.email.bindingsDescription') }}
            </p>
          </div>
          <Popover>
            <PopoverTrigger as-child>
              <Button
                size="sm"
                variant="outline"
                :disabled="!unboundProviders.length"
                class="shrink-0"
              >
                <Plus class="size-4" />
                {{ $t('bots.email.addBinding') }}
              </Button>
            </PopoverTrigger>
            <PopoverContent
              class="w-56 p-1"
              align="end"
            >
              <button
                v-for="p in unboundProviders"
                :key="p.id"
                type="button"
                class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-xs text-foreground transition-colors hover:bg-accent"
                :disabled="addingBinding"
                @click="handleAddBinding(p)"
              >
                <Spinner
                  v-if="addingProviderId === p.id"
                  class="size-3"
                />
                {{ p.name }}
                <span class="ml-auto text-xs text-muted-foreground">{{ p.provider }}</span>
              </button>
            </PopoverContent>
          </Popover>
        </div>

        <div
          v-if="bindingsLoading"
          class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 text-sm text-muted-foreground last:border-b-0"
        >
          <Spinner class="size-4" />
          <span>{{ $t('common.loading') }}</span>
        </div>

        <div
          v-else-if="!bindings?.length"
          class="p-4"
        >
          <Empty class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-12">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Mail class="size-4" />
              </EmptyMedia>
              <EmptyTitle>{{ $t('bots.email.noBindings') }}</EmptyTitle>
              <EmptyDescription>{{ $t('bots.email.noBindingsDescription') }}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        </div>

        <template v-else>
          <div
            v-for="binding in bindings"
            :key="binding.id"
            class="mx-4 border-b border-border py-4 last:border-b-0"
          >
            <div class="flex items-center justify-between gap-4">
              <div class="min-w-0">
                <p class="text-sm font-medium text-foreground">
                  {{ providerNameMap[binding.email_provider_id!] || binding.email_provider_id }}
                </p>
                <p
                  v-if="binding.email_address"
                  class="mt-0.5 text-xs text-muted-foreground"
                >
                  {{ binding.email_address }}
                </p>
              </div>
              <ConfirmPopover
                :message="$t('bots.email.unbindConfirm')"
                :loading="deletingId === binding.id"
                @confirm="handleDeleteBinding(binding.id!)"
              >
                <template #trigger>
                  <Button
                    variant="destructive"
                    size="sm"
                    class="shrink-0"
                  >
                    {{ $t('bots.email.unbind') }}
                  </Button>
                </template>
              </ConfirmPopover>
            </div>
            <div class="mt-4 flex flex-wrap gap-6 text-xs">
              <label class="flex cursor-pointer items-center gap-2 text-foreground">
                <Switch
                  :model-value="binding.can_read"
                  @update:model-value="(v) => handleTogglePerm(binding, 'can_read', !!v)"
                />
                <span>{{ $t('bots.email.canRead') }}</span>
              </label>
              <label class="flex cursor-pointer items-center gap-2 text-foreground">
                <Switch
                  :model-value="binding.can_write"
                  @update:model-value="(v) => handleTogglePerm(binding, 'can_write', !!v)"
                />
                <span>{{ $t('bots.email.canWrite') }}</span>
              </label>
            </div>
          </div>
        </template>
      </SettingsSection>

      <section class="space-y-2.5">
        <h2 class="px-2 text-[13px] font-medium text-muted-foreground">
          {{ $t('bots.email.outbox') }}
        </h2>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('bots.email.to') }}</TableHead>
              <TableHead>{{ $t('bots.email.subject') }}</TableHead>
              <TableHead>{{ $t('bots.email.status') }}</TableHead>
              <TableHead>{{ $t('bots.email.sentAt') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty
              v-if="outboxLoading"
              :colspan="4"
            >
              <div class="flex items-center gap-2">
                <Spinner class="size-4" />
                {{ $t('common.loading') }}
              </div>
            </TableEmpty>
            <TableEmpty
              v-else-if="!outboxItems?.length"
              :colspan="4"
            >
              {{ $t('bots.email.noEmails') }}
            </TableEmpty>
            <template v-else>
              <TableRow
                v-for="item in outboxItems"
                :key="item.id"
              >
                <TableCell class="text-foreground">
                  {{ Array.isArray(item.to) ? item.to.join(', ') : item.to }}
                </TableCell>
                <TableCell class="text-foreground">
                  {{ item.subject }}
                </TableCell>
                <TableCell>
                  <Badge :variant="item.status === 'failed' ? 'destructive' : 'secondary'">
                    {{ item.status }}
                  </Badge>
                </TableCell>
                <TableCell class="whitespace-nowrap text-muted-foreground">
                  {{ formatDate(item.sent_at) }}
                </TableCell>
              </TableRow>
            </template>
          </TableBody>
        </Table>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
  Button,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Spinner,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { Mail, Plus } from 'lucide-vue-next'
import { computed, ref } from 'vue'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  getEmailProviders,
  getBotsByBotIdEmailBindings,
  postBotsByBotIdEmailBindings,
  putBotsByBotIdEmailBindingsById,
  deleteBotsByBotIdEmailBindingsById,
  getBotsByBotIdEmailOutbox,
} from '@memohai/sdk'
import type { EmailProviderResponse, EmailBindingResponse, EmailOutboxItemResponse } from '@memohai/sdk'
import { formatDateTime } from '@/utils/date-time'
import SettingsSection from '@/components/settings/section.vue'

const props = defineProps<{ botId: string }>()
const { t } = useI18n()

const queryCache = useQueryCache()

const { data: providersData } = useQuery({
  key: () => ['email-providers'],
  query: async () => {
    const { data } = await getEmailProviders({ throwOnError: true })
    return data ?? []
  },
})

const { data: bindingsData, isLoading: bindingsLoading, refetch: refetchBindings } = useQuery({
  key: () => ['bot-email-bindings', props.botId],
  query: async () => {
    if (!props.botId) return [] as EmailBindingResponse[]
    const { data } = await getBotsByBotIdEmailBindings({ path: { bot_id: props.botId }, throwOnError: true })
    return data ?? []
  },
  enabled: () => !!props.botId,
})

const { data: outboxData, isLoading: outboxLoading } = useQuery({
  key: () => ['bot-email-outbox', props.botId],
  query: async () => {
    if (!props.botId) return [] as EmailOutboxItemResponse[]
    const { data } = await getBotsByBotIdEmailOutbox({
      path: { bot_id: props.botId },
      query: { limit: 50, offset: 0 },
      throwOnError: true,
    })
    return ((data as Record<string, unknown>)?.items as EmailOutboxItemResponse[]) ?? []
  },
  enabled: () => !!props.botId,
})

const providers = computed<EmailProviderResponse[]>(() => providersData.value ?? [])
const bindings = computed<EmailBindingResponse[]>(() => bindingsData.value ?? [])
const outboxItems = computed<EmailOutboxItemResponse[]>(() => outboxData.value ?? [])

const addingBinding = ref(false)
const addingProviderId = ref('')
const deletingId = ref('')

const providerNameMap = computed(() => {
  const map: Record<string, string> = {}
  for (const p of providers.value) {
    if (p.id && p.name) map[p.id] = p.name
  }
  return map
})

const unboundProviders = computed(() => {
  const boundIds = new Set(bindings.value.map((b) => b.email_provider_id))
  return providers.value.filter((p) => !boundIds.has(p.id))
})

function invalidateBindings() {
  queryCache.invalidateQueries({ key: ['bot-email-bindings', props.botId] })
}

async function handleAddBinding(provider: EmailProviderResponse) {
  addingBinding.value = true
  addingProviderId.value = provider.id!
  const emailAddr = (provider.config as Record<string, unknown>)?.username as string || provider.name || ''
  try {
    await postBotsByBotIdEmailBindings({
      path: { bot_id: props.botId },
      body: {
        email_provider_id: provider.id!,
        email_address: emailAddr,
        can_read: true,
        can_write: true,
        can_delete: false,
      },
      throwOnError: true,
    })
    invalidateBindings()
    await refetchBindings()
    toast.success(t('bots.email.bindSuccess'))
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  } finally {
    addingBinding.value = false
    addingProviderId.value = ''
  }
}

async function handleTogglePerm(binding: EmailBindingResponse, field: string, value: boolean) {
  try {
    await putBotsByBotIdEmailBindingsById({
      path: { bot_id: props.botId, id: binding.id! },
      body: { [field]: value },
      throwOnError: true,
    })
    invalidateBindings()
    await refetchBindings()
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  }
}

async function handleDeleteBinding(id: string) {
  deletingId.value = id
  try {
    await deleteBotsByBotIdEmailBindingsById({
      path: { bot_id: props.botId, id },
      throwOnError: true,
    })
    invalidateBindings()
    await refetchBindings()
    toast.success(t('bots.email.unbindSuccess'))
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  } finally {
    deletingId.value = ''
  }
}

function formatDate(value: string | undefined) {
  return formatDateTime(value, { fallback: '-' })
}
</script>
