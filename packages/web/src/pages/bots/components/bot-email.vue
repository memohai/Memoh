<template>
  <div class="max-w-4xl mx-auto space-y-5">
    <!-- Header -->
    <div class="flex items-start justify-between gap-3">
      <div class="space-y-1 min-w-0">
        <h3 class="text-lg font-semibold">
          {{ $t('bots.email.title') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.email.subtitle') }}
        </p>
      </div>
    </div>

    <!-- Bindings section -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <h4 class="text-sm font-medium">
          {{ $t('bots.email.bindings') }}
        </h4>
        <Popover>
          <PopoverTrigger as-child>
            <Button
              size="sm"
              :disabled="!unboundProviders.length"
            >
              <FontAwesomeIcon
                :icon="['fas', 'plus']"
                class="mr-1.5"
              />
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
              class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent"
              :disabled="addingBinding"
              @click="handleAddBinding(p)"
            >
              <Spinner
                v-if="addingProviderId === p.id"
                class="size-3"
              />
              {{ p.name }}
              <span class="text-xs text-muted-foreground ml-auto">{{ p.provider }}</span>
            </button>
          </PopoverContent>
        </Popover>
      </div>

      <div
        v-if="bindingsLoading"
        class="flex items-center gap-2 text-sm text-muted-foreground p-4"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <div
        v-else-if="!bindings?.length"
        class="rounded-md border p-4"
      >
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.email.noBindings') }}
        </p>
      </div>

      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="binding in bindings"
          :key="binding.id"
          class="rounded-md border p-4"
        >
          <div class="flex items-center justify-between">
            <div class="min-w-0">
              <p class="font-medium text-sm">
                {{ providerNameMap[binding.email_provider_id!] || binding.email_provider_id }}
              </p>
              <p
                v-if="binding.email_address"
                class="text-xs text-muted-foreground mt-0.5"
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
                >
                  {{ $t('bots.email.unbind') }}
                </Button>
              </template>
            </ConfirmPopover>
          </div>
          <Separator class="my-3" />
          <div class="flex gap-6 text-sm">
            <label class="flex items-center gap-2 cursor-pointer">
              <Switch
                :model-value="binding.can_read"
                @update:model-value="(v) => handleTogglePerm(binding, 'can_read', !!v)"
              />
              <span>{{ $t('bots.email.canRead') }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <Switch
                :model-value="binding.can_write"
                @update:model-value="(v) => handleTogglePerm(binding, 'can_write', !!v)"
              />
              <span>{{ $t('bots.email.canWrite') }}</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <Separator />

    <!-- Outbox (audit) -->
    <div class="space-y-3">
      <h4 class="text-sm font-medium">
        {{ $t('bots.email.outbox') }}
      </h4>
      <div
        v-if="outboxLoading"
        class="flex items-center gap-2 text-sm text-muted-foreground p-4"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>
      <div
        v-else-if="!outboxItems?.length"
        class="rounded-md border p-4"
      >
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.email.noEmails') }}
        </p>
      </div>
      <div
        v-else
        class="overflow-x-auto rounded-md border"
      >
        <table class="w-full text-sm">
          <thead class="bg-muted/50 text-left">
            <tr>
              <th class="px-3 py-2 font-medium">
                {{ $t('bots.email.to') }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t('bots.email.subject') }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t('bots.email.status') }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t('bots.email.sentAt') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in outboxItems"
              :key="item.id"
              class="border-t"
            >
              <td class="px-3 py-2 text-xs">
                {{ Array.isArray(item.to) ? item.to.join(', ') : item.to }}
              </td>
              <td class="px-3 py-2">
                {{ item.subject }}
              </td>
              <td class="px-3 py-2">
                <Badge :variant="item.status === 'failed' ? 'destructive' : 'secondary'">
                  {{ item.status }}
                </Badge>
              </td>
              <td class="px-3 py-2 text-xs text-muted-foreground whitespace-nowrap">
                {{ formatDate(item.sent_at) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Separator,
  Spinner,
  Switch,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import {
  getEmailProviders,
  getBotsByBotIdEmailBindings,
  postBotsByBotIdEmailBindings,
  putBotsByBotIdEmailBindingsById,
  deleteBotsByBotIdEmailBindingsById,
  getBotsByBotIdEmailOutbox,
} from '@memoh/sdk'
import type { EmailProviderResponse, EmailBindingResponse, EmailOutboxItemResponse } from '@memoh/sdk'
import { formatDateTime } from '@/utils/date-time'

const props = defineProps<{ botId: string }>()
const { t } = useI18n()

const providers = ref<EmailProviderResponse[]>([])
const bindings = ref<EmailBindingResponse[]>([])
const bindingsLoading = ref(false)
const addingBinding = ref(false)
const addingProviderId = ref('')
const deletingId = ref('')
const outboxItems = ref<EmailOutboxItemResponse[]>([])
const outboxLoading = ref(false)

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

async function loadProviders() {
  const { data } = await getEmailProviders({ throwOnError: true })
  providers.value = data ?? []
}

async function loadBindings() {
  bindingsLoading.value = true
  try {
    const { data } = await getBotsByBotIdEmailBindings({ path: { bot_id: props.botId }, throwOnError: true })
    bindings.value = data ?? []
  } finally {
    bindingsLoading.value = false
  }
}

async function loadOutbox() {
  outboxLoading.value = true
  try {
    const { data } = await getBotsByBotIdEmailOutbox({
      path: { bot_id: props.botId },
      query: { limit: 50, offset: 0 },
      throwOnError: true,
    })
    outboxItems.value = (data as any)?.items ?? []
  } finally {
    outboxLoading.value = false
  }
}

async function handleAddBinding(provider: EmailProviderResponse) {
  addingBinding.value = true
  addingProviderId.value = provider.id!
  const emailAddr = (provider.config as any)?.username || provider.name || ''
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
    await loadBindings()
    toast.success(t('bots.email.bindSuccess'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
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
    await loadBindings()
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  }
}

async function handleDeleteBinding(id: string) {
  deletingId.value = id
  try {
    await deleteBotsByBotIdEmailBindingsById({
      path: { bot_id: props.botId, id },
      throwOnError: true,
    })
    await loadBindings()
    toast.success(t('bots.email.unbindSuccess'))
  } catch (e: any) {
    toast.error(e?.message || t('common.saveFailed'))
  } finally {
    deletingId.value = ''
  }
}

function formatDate(value: string | undefined) {
  return formatDateTime(value, { fallback: '-' })
}

watch(() => props.botId, () => {
  void loadProviders()
  void loadBindings()
  void loadOutbox()
}, { immediate: true })
</script>
