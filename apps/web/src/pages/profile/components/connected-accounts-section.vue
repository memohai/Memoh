<template>
  <div class="rounded-md border bg-background shadow-sm">
    <div class="p-4 md:p-6 space-y-4">
      <div class="flex items-start justify-between gap-3">
        <div class="space-y-1">
          <h3 class="text-sm font-medium">
            {{ $t('settings.connectedAccounts.title') }}
          </h3>
          <p class="text-xs text-muted-foreground max-w-md">
            {{ $t('settings.connectedAccounts.subtitle') }}
          </p>
        </div>
        <Button
          v-if="hasLinkableImBot"
          variant="outline"
          size="sm"
          :disabled="issuing"
          @click="onIssue"
        >
          <Spinner
            v-if="issuing"
            class="mr-1.5 size-3.5"
          />
          <Plus
            v-else
            class="mr-1.5 size-3.5"
          />
          {{ $t('settings.connectedAccounts.connect') }}
        </Button>
      </div>

      <!-- No IM bot: cannot receive /link, so guide instead of issuing a dead code -->
      <div
        v-if="!checkingImBot && !hasLinkableImBot"
        class="rounded-md border border-dashed border-border/70 bg-muted/20 p-3 space-y-1"
      >
        <p class="text-[11px] font-medium text-foreground">
          {{ $t('settings.connectedAccounts.noImBotTitle') }}
        </p>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('settings.connectedAccounts.noImBotHint') }}
        </p>
      </div>

      <!-- Active link code with live countdown -->
      <div
        v-else-if="activeCode"
        class="rounded-md border border-dashed border-border/70 bg-muted/20 p-3 space-y-2"
      >
        <div class="flex items-center justify-between gap-2">
          <p class="text-[11px] font-medium text-foreground">
            {{ $t('settings.connectedAccounts.codeTitle') }}
          </p>
          <span
            class="text-[10px] tabular-nums"
            :class="expired ? 'text-destructive' : 'text-muted-foreground'"
          >
            {{ expired ? $t('settings.connectedAccounts.expired') : $t('settings.connectedAccounts.expiresIn', { time: remainingLabel }) }}
          </span>
        </div>
        <p class="text-[11px] text-muted-foreground">
          {{ $t('settings.connectedAccounts.codeInstruction') }}
        </p>
        <div class="flex items-center gap-2">
          <code
            class="flex-1 rounded bg-background border px-2.5 py-1.5 font-mono text-xs select-all transition-colors"
            :class="expired ? 'border-border/40 text-muted-foreground/50 line-through' : 'border-border/50'"
          >
            /link {{ activeCode }}
          </code>
          <Button
            v-if="!expired"
            variant="ghost"
            size="sm"
            class="h-8 text-[11px] shrink-0"
            @click="copyCode"
          >
            <Check
              v-if="copied"
              class="mr-1 size-3.5"
            />
            <Copy
              v-else
              class="mr-1 size-3.5"
            />
            {{ copied ? $t('settings.connectedAccounts.copied') : $t('settings.connectedAccounts.copy') }}
          </Button>
          <Button
            v-else
            variant="ghost"
            size="sm"
            class="h-8 text-[11px] shrink-0"
            :disabled="issuing"
            @click="onIssue"
          >
            <Spinner
              v-if="issuing"
              class="mr-1 size-3.5"
            />
            <RefreshCw
              v-else
              class="mr-1 size-3.5"
            />
            {{ $t('settings.connectedAccounts.connect') }}
          </Button>
        </div>
      </div>

      <!-- Loading bound identities -->
      <div
        v-if="isLoading"
        class="flex justify-center py-6"
      >
        <Spinner class="size-5 text-muted-foreground/50" />
      </div>

      <!-- Empty -->
      <p
        v-else-if="bindings.length === 0"
        class="text-[11px] text-muted-foreground/70 italic"
      >
        {{ $t('settings.connectedAccounts.empty') }}
      </p>

      <!-- Bound identities -->
      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="binding in bindings"
          :key="binding.id"
          class="flex items-center gap-3 rounded-lg border border-border/60 bg-background/70 px-3 py-2.5"
        >
          <Avatar class="size-7 shrink-0 border border-border/40">
            <AvatarImage
              :src="binding.channel_identity_avatar_url || ''"
              :alt="bindingLabel(binding)"
            />
            <AvatarFallback class="text-[10px]">
              {{ bindingLabel(binding).slice(0, 2).toUpperCase() }}
            </AvatarFallback>
          </Avatar>
          <div class="min-w-0 flex-1">
            <div class="truncate text-xs font-medium text-foreground">
              {{ bindingLabel(binding) }}
            </div>
            <div
              v-if="binding.channel_type"
              class="flex items-center gap-1 truncate text-[10px] text-muted-foreground"
            >
              <ChannelIcon
                :channel="binding.channel_type"
                size="1em"
              />
              <span>{{ channelTypeDisplayName(t, binding.channel_type) }}</span>
            </div>
          </div>
          <ConfirmPopover
            :message="$t('settings.connectedAccounts.disconnectConfirm')"
            :confirm-text="$t('settings.connectedAccounts.disconnect')"
            @confirm="() => onDisconnect(binding)"
          >
            <template #trigger>
              <Button
                variant="ghost"
                size="icon"
                class="size-7 text-muted-foreground hover:text-destructive shadow-none"
                :disabled="isRowBusy(binding)"
              >
                <Trash2 class="size-3.5" />
              </Button>
            </template>
          </ConfirmPopover>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Plus, Trash2, Copy, Check, RefreshCw } from 'lucide-vue-next'
import { toast } from '@memohai/ui'
import { Button, Spinner, Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { channelTypeDisplayName } from '@/utils/channel-type-label'
import {
  getBots,
  getChannels,
  getBotsByIdChannelByPlatform,
  getUsersMeChannelIdentities,
  postUsersMeChannelLinks,
  deleteUsersMeChannelIdentitiesByChannelIdentityId,
} from '@memohai/sdk'
import type { ChannelaccessBinding } from '@memohai/sdk'

const { t } = useI18n()
const queryCache = useQueryCache()

const issuing = ref(false)
const activeCode = ref('')
const expiresAtMs = ref(0)
const nowMs = ref(Date.now())
const copied = ref(false)
const busyIds = ref<Set<string>>(new Set())

let ticker: ReturnType<typeof setInterval> | undefined
let pollTick = 0

function stopTicker() {
  if (ticker) {
    clearInterval(ticker)
    ticker = undefined
  }
}

onBeforeUnmount(stopTicker)

const expired = computed(() => !!activeCode.value && expiresAtMs.value > 0 && nowMs.value >= expiresAtMs.value)
const remainingLabel = computed(() => {
  const ms = Math.max(0, expiresAtMs.value - nowMs.value)
  const totalSec = Math.ceil(ms / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${m}:${String(s).padStart(2, '0')}`
})

const { data: bindingsData, isLoading, refetch: refetchBindings } = useQuery({
  key: () => ['my-channel-identities'],
  query: async () => {
    const { data } = await getUsersMeChannelIdentities({ throwOnError: true })
    return data
  },
})

const bindings = computed<ChannelaccessBinding[]>(() => bindingsData.value?.items ?? [])

// Whether the user has any bot connected to an IM channel that could receive /link.
const { data: hasImBotData, isLoading: checkingImBot } = useQuery({
  key: () => ['user-has-im-bot'],
  query: async (): Promise<boolean> => {
    const [botsRes, channelsRes] = await Promise.all([
      getBots({ throwOnError: true }),
      getChannels({ throwOnError: true }),
    ])
    const imPlatforms = (channelsRes.data ?? [])
      .filter(m => !m.configless && m.type)
      .map(m => m.type as string)
    const botList = botsRes.data?.items ?? []
    if (imPlatforms.length === 0 || botList.length === 0) return false
    for (const bot of botList) {
      const id = bot.id?.trim()
      if (!id) continue
      const hits = await Promise.all(imPlatforms.map(async (platform) => {
        try {
          const { data } = await getBotsByIdChannelByPlatform({ path: { id, platform }, throwOnError: true })
          return !data?.disabled
        }
        catch {
          return false
        }
      }))
      if (hits.some(Boolean)) return true
    }
    return false
  },
})

const hasLinkableImBot = computed(() => hasImBotData.value === true)

function bindingLabel(binding: ChannelaccessBinding): string {
  return binding.channel_identity_display_name
    || binding.channel_subject_id
    || binding.channel_identity_id
    || ''
}

function isRowBusy(binding: ChannelaccessBinding): boolean {
  return !!binding.channel_identity_id && busyIds.value.has(binding.channel_identity_id)
}

function invalidate() {
  return queryCache.invalidateQueries({ key: ['my-channel-identities'] })
}

function startTicker() {
  stopTicker()
  nowMs.value = Date.now()
  pollTick = 0
  ticker = setInterval(() => {
    nowMs.value = Date.now()
    // While the code is live, the user is likely completing /link in IM, so poll
    // the bindings list (every ~3s) to surface the new account without a manual
    // refresh. Stop ticking once the code expires.
    if (expiresAtMs.value > 0 && nowMs.value >= expiresAtMs.value) {
      stopTicker()
      return
    }
    pollTick += 1
    if (pollTick % 3 === 0) {
      void refetchBindings()
    }
  }, 1000)
}

async function onIssue() {
  if (issuing.value) return
  issuing.value = true
  try {
    const { data } = await postUsersMeChannelLinks({ body: {}, throwOnError: true })
    activeCode.value = data?.token ?? ''
    expiresAtMs.value = data?.expires_at ? new Date(data.expires_at).getTime() : 0
    copied.value = false
    startTicker()
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('settings.connectedAccounts.issueFailed')))
  }
  finally {
    issuing.value = false
  }
}

async function copyCode() {
  if (!activeCode.value || expired.value) return
  try {
    await navigator.clipboard.writeText(`/link ${activeCode.value}`)
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  }
  catch {
    // Clipboard may be unavailable; the code is selectable as a fallback.
  }
}

async function onDisconnect(binding: ChannelaccessBinding) {
  const identityId = binding.channel_identity_id
  if (!identityId) return
  busyIds.value.add(identityId)
  try {
    await deleteUsersMeChannelIdentitiesByChannelIdentityId({
      path: { channel_identity_id: identityId },
      throwOnError: true,
    })
    await invalidate()
    toast.success(t('settings.connectedAccounts.disconnected'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('settings.connectedAccounts.disconnectFailed')))
  }
  finally {
    busyIds.value.delete(identityId)
  }
}
</script>
