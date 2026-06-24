<template>
  <div class="space-y-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h4 class="text-xs font-medium">
          {{ $t('bots.channels.whatsappQr.title') }}
        </h4>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ $t('bots.channels.whatsappQr.description') }}
        </p>
      </div>
      <SegmentedControl
        v-if="loginState === 'idle'"
        v-model="loginMode"
        :items="loginModeItems"
        :aria-label="$t('bots.channels.whatsappQr.modeLabel')"
        class="shrink-0"
      />
    </div>

    <Alert>
      <TriangleAlert />
      <AlertTitle>{{ $t('bots.channels.whatsappQr.experimentalTitle') }}</AlertTitle>
      <AlertDescription>
        {{ $t('bots.channels.whatsappQr.experimentalWarning') }}
      </AlertDescription>
    </Alert>

    <div
      v-if="configured && loginState === 'idle'"
      class="rounded-[var(--radius-menu-shell)] border border-border p-4"
    >
      <div class="flex items-center justify-between gap-3">
        <div class="min-w-0">
          <p class="text-sm font-medium text-foreground">
            {{ statusLabel }}
          </p>
          <p
            v-if="accountLine"
            class="mt-1 truncate text-xs text-muted-foreground"
          >
            {{ accountLine }}
          </p>
        </div>
        <Spinner
          v-if="isLoadingStatus"
          class="size-4"
        />
      </div>
    </div>

    <div
      v-if="loginState === 'idle' && loginMode === 'qr'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <Button
        :disabled="isStarting"
        @click="startQrLogin"
      >
        <Spinner
          v-if="isStarting"
          class="mr-1.5"
        />
        <QrCode
          v-else
          class="mr-1.5 size-3.5"
        />
        {{ configured ? $t('bots.channels.whatsappQr.relink') : $t('bots.channels.whatsappQr.startScan') }}
      </Button>
    </div>

    <div
      v-else-if="loginState === 'idle' && loginMode === 'phone'"
      class="space-y-3 py-4"
    >
      <div class="space-y-1.5">
        <Label for="whatsapp-phone-number">
          {{ $t('bots.channels.whatsappQr.phoneLabel') }}
        </Label>
        <Input
          id="whatsapp-phone-number"
          v-model="phoneNumber"
          type="tel"
          autocomplete="tel"
          :disabled="isStarting"
          :placeholder="$t('bots.channels.whatsappQr.phonePlaceholder')"
          @keydown.enter.prevent="startPhoneLogin"
        />
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.channels.whatsappQr.phoneHint') }}
        </p>
      </div>
      <Button
        class="w-full sm:w-auto"
        :disabled="isStarting"
        @click="startPhoneLogin"
      >
        <Spinner
          v-if="isStarting"
          class="mr-1.5"
        />
        <Smartphone
          v-else
          class="mr-1.5 size-3.5"
        />
        {{ $t('bots.channels.whatsappQr.startPhone') }}
      </Button>
    </div>

    <div
      v-else-if="loginState === 'showing'"
      class="flex flex-col items-center gap-4 py-4"
    >
      <template v-if="activeLoginMode === 'qr'">
        <div class="relative rounded-[var(--radius-menu-shell)] border border-border bg-card p-3">
          <img
            v-if="qrImageDataUrl"
            :src="qrImageDataUrl"
            :alt="$t('bots.channels.whatsappQr.qrAlt')"
            class="size-52"
          >
          <div
            v-else
            class="flex size-52 items-center justify-center text-muted-foreground"
          >
            <Spinner />
          </div>

          <div
            v-if="pollStatus === 'expired'"
            class="absolute inset-0 flex flex-col items-center justify-center gap-2 rounded-[var(--radius-menu-shell)] bg-popover"
          >
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.channels.whatsappQr.expired') }}
            </p>
            <Button
              size="sm"
              variant="outline"
              :disabled="isStarting"
              @click="startQrLogin"
            >
              {{ $t('bots.channels.whatsappQr.refresh') }}
            </Button>
          </div>
        </div>
      </template>

      <template v-else>
        <div class="w-full max-w-xs rounded-[var(--radius-menu-shell)] border border-border bg-card p-4 text-center">
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.whatsappQr.pairingCodeTitle') }}
          </p>
          <p class="mt-2 font-mono text-title font-semibold text-foreground">
            {{ pairingCode || '--' }}
          </p>
        </div>
        <Button
          v-if="pollStatus === 'expired'"
          size="sm"
          variant="outline"
          :disabled="isStarting"
          @click="startPhoneLogin"
        >
          {{ $t('bots.channels.whatsappQr.refresh') }}
        </Button>
      </template>

      <p class="max-w-xs text-center text-xs text-muted-foreground">
        {{ statusText }}
      </p>

      <Button
        variant="ghost"
        size="sm"
        @click="() => cancel()"
      >
        {{ $t('common.cancel') }}
      </Button>
    </div>

    <div
      v-else-if="loginState === 'success'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <div class="flex size-12 items-center justify-center rounded-full bg-success-soft">
        <Check class="size-5 text-success-foreground" />
      </div>
      <p class="text-xs font-medium">
        {{ $t('bots.channels.whatsappQr.success') }}
      </p>
    </div>

    <div
      v-else-if="loginState === 'error'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <p class="text-xs text-destructive">
        {{ errorMessage }}
      </p>
      <Button
        variant="outline"
        size="sm"
        :disabled="isStarting"
        @click="retryLogin"
      >
        <Spinner
          v-if="isStarting"
          class="mr-1.5"
        />
        {{ $t('bots.channels.whatsappQr.retry') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Check, QrCode, Smartphone, TriangleAlert } from 'lucide-vue-next'
import { computed, onUnmounted, ref, watch } from 'vue'
import { Alert, AlertDescription, AlertTitle, Button, Input, Label, SegmentedControl, Spinner, toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import {
  getBotsByIdChannelWhatsappStatus,
  postBotsByIdChannelWhatsappLoginCancel,
  postBotsByIdChannelWhatsappPhonePoll,
  postBotsByIdChannelWhatsappPhoneStart,
  postBotsByIdChannelWhatsappQrPoll,
  postBotsByIdChannelWhatsappQrStart,
} from '@memohai/sdk'
import type { WhatsappStatusResponse } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
  configured?: boolean
}>()

const emit = defineEmits<{
  loginSuccess: []
  'update:busy': [busy: boolean]
}>()

const { t } = useI18n()

type LoginMode = 'qr' | 'phone'
type LoginState = 'idle' | 'showing' | 'success' | 'error'

const loginState = ref<LoginState>('idle')
const loginMode = ref<LoginMode>('qr')
const activeLoginMode = ref<LoginMode>('qr')
const loginId = ref('')
const phoneNumber = ref('')
const pairingCode = ref('')
const qrImageDataUrl = ref('')
const pollStatus = ref('')
const isStarting = ref(false)
const isLoadingStatus = ref(false)
const errorMessage = ref('')
const status = ref<WhatsappStatusResponse | null>(null)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let statusTimer: ReturnType<typeof setInterval> | null = null
let aborted = false
let flowSeq = 0
let statusSeq = 0

const loginModeItems = computed(() => [
  { value: 'qr' as const, label: t('bots.channels.whatsappQr.scanMode') },
  { value: 'phone' as const, label: t('bots.channels.whatsappQr.phoneMode') },
])
const pairingBusy = computed(() => isStarting.value || loginState.value === 'showing')

const statusLabel = computed(() => {
  const raw = status.value?.status || 'disconnected'
  switch (raw) {
    case 'connected':
      return t('bots.channels.whatsappQr.connected')
    case 'reconnecting':
      return t('bots.channels.whatsappQr.reconnecting')
    case 'logged_out':
      return t('bots.channels.whatsappQr.loggedOut')
    default:
      return t('bots.channels.whatsappQr.disconnected')
  }
})

const accountLine = computed(() => {
  const phone = status.value?.phone?.trim()
  const name = status.value?.push_name?.trim()
  const jid = status.value?.device_jid?.trim()
  return [name, phone || jid].filter(Boolean).join(' · ')
})

const statusText = computed(() => {
  switch (pollStatus.value) {
    case 'expired':
      return t('bots.channels.whatsappQr.expired')
    case 'success':
      return t('bots.channels.whatsappQr.success')
    case 'terminal':
      return errorMessage.value || t('bots.channels.whatsappQr.failed')
    case 'pair_code':
      return t('bots.channels.whatsappQr.waitingPhonePair')
    default:
      return activeLoginMode.value === 'phone'
        ? t('bots.channels.whatsappQr.waitingPhonePair')
        : t('bots.channels.whatsappQr.waitingScan')
  }
})

async function loadStatus() {
  const botId = props.botId
  const statusFlow = ++statusSeq
  if (!botId || !props.configured) {
    status.value = null
    return
  }
  isLoadingStatus.value = true
  try {
    const { data } = await getBotsByIdChannelWhatsappStatus({ path: { id: botId }, throwOnError: true })
    if (statusFlow === statusSeq && botId === props.botId) {
      status.value = data
    }
  } catch {
    if (statusFlow === statusSeq && botId === props.botId) {
      status.value = null
    }
  } finally {
    if (statusFlow === statusSeq && botId === props.botId) {
      isLoadingStatus.value = false
    }
  }
}

async function cancelPendingLogin(id = loginId.value, botId = props.botId) {
  const pendingID = id.trim()
  if (!botId || !pendingID) return
  try {
    await postBotsByIdChannelWhatsappLoginCancel({
      path: { id: botId },
      body: { login_id: pendingID },
      throwOnError: true,
    })
  } catch {
    // The login may already have expired or completed; cancel is best effort.
  }
}

async function startQrLogin() {
  if (isStarting.value) return
  const botID = props.botId
  const flow = resetStartState('qr', botID)
  try {
    const { data } = await postBotsByIdChannelWhatsappQrStart({ path: { id: botID }, throwOnError: true })
    if (!isCurrentFlow(flow)) {
      if (data.login_id) void cancelPendingLogin(data.login_id, botID)
      return
    }
    loginId.value = data.login_id || ''
    if (!loginId.value) {
      throw new Error('No login id returned')
    }
    pollStatus.value = data.status || ''
    if (data.qr_code) {
      await renderQR(data.qr_code, flow)
    }
    if (!isCurrentFlow(flow)) return
    loginState.value = 'showing'
    startPolling(flow)
  } catch (err) {
    if (!isCurrentFlow(flow)) return
    errorMessage.value = resolveApiErrorMessage(err, err instanceof Error ? err.message : String(err))
    loginState.value = 'error'
  } finally {
    if (isCurrentFlow(flow)) {
      isStarting.value = false
    }
  }
}

async function startPhoneLogin() {
  if (isStarting.value) return
  const botID = props.botId
  const phone = phoneNumber.value.trim()
  if (!phone) {
    toast.error(t('bots.channels.whatsappQr.phoneRequired'))
    return
  }
  const flow = resetStartState('phone', botID)
  try {
    const { data } = await postBotsByIdChannelWhatsappPhoneStart({
      path: { id: botID },
      body: { phone },
      throwOnError: true,
    })
    if (!isCurrentFlow(flow)) {
      if (data.login_id) void cancelPendingLogin(data.login_id, botID)
      return
    }
    loginId.value = data.login_id || ''
    if (!loginId.value) {
      throw new Error('No login id returned')
    }
    pollStatus.value = data.status || ''
    pairingCode.value = data.pairing_code || ''
    loginState.value = 'showing'
    startPolling(flow)
  } catch (err) {
    if (!isCurrentFlow(flow)) return
    errorMessage.value = resolveApiErrorMessage(err, err instanceof Error ? err.message : String(err))
    loginState.value = 'error'
  } finally {
    if (isCurrentFlow(flow)) {
      isStarting.value = false
    }
  }
}

function resetStartState(mode: LoginMode, botID = props.botId) {
  const pendingID = loginId.value
  if (pendingID) void cancelPendingLogin(pendingID, botID)
  flowSeq += 1
  const flow = flowSeq
  aborted = false
  activeLoginMode.value = mode
  isStarting.value = true
  loginId.value = ''
  errorMessage.value = ''
  pollStatus.value = ''
  pairingCode.value = ''
  qrImageDataUrl.value = ''
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  return flow
}

function isCurrentFlow(flow: number) {
  return !aborted && flow === flowSeq
}

function startPolling(flow: number) {
  if (!isCurrentFlow(flow)) return
  void pollOnce(flow)
}

async function pollOnce(flow: number) {
  if (!isCurrentFlow(flow) || loginState.value !== 'showing') return
  try {
    const { data } = activeLoginMode.value === 'phone'
      ? await postBotsByIdChannelWhatsappPhonePoll({
        path: { id: props.botId },
        body: { login_id: loginId.value },
        throwOnError: true,
      })
      : await postBotsByIdChannelWhatsappQrPoll({
        path: { id: props.botId },
        body: { login_id: loginId.value },
        throwOnError: true,
      })
    if (!isCurrentFlow(flow)) return
    pollStatus.value = data.status || ''
    if ('qr_code' in data && data.qr_code) {
      await renderQR(data.qr_code, flow)
    }
    if (!isCurrentFlow(flow)) return
    if ('pairing_code' in data && data.pairing_code) {
      pairingCode.value = data.pairing_code
    }
    switch (data.status) {
      case 'success':
        loginState.value = 'success'
        loginId.value = ''
        toast.success(t('bots.channels.whatsappQr.success'))
        emit('loginSuccess')
        await loadStatus()
        return
      case 'expired':
        return
      case 'terminal':
        errorMessage.value = data.message || t('bots.channels.whatsappQr.failed')
        loginState.value = 'error'
        return
      default:
        if (isCurrentFlow(flow)) {
          pollTimer = setTimeout(() => void pollOnce(flow), 1500)
        }
    }
  } catch (err) {
    if (isCurrentFlow(flow)) {
      if (isLoginNotFoundError(err)) {
        pollStatus.value = 'expired'
        return
      }
      errorMessage.value = resolveApiErrorMessage(err, t('bots.channels.whatsappQr.failed'))
      pollStatus.value = 'terminal'
      loginState.value = 'error'
    }
  }
}

async function renderQR(value: string, flow: number) {
  const dataUrl = await QRCode.toDataURL(value, { width: 208, margin: 1 })
  if (isCurrentFlow(flow)) {
    qrImageDataUrl.value = dataUrl
  }
}

function cancel(botIdForPending = props.botId) {
  const pendingID = loginId.value
  aborted = true
  flowSeq += 1
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  isStarting.value = false
  loginState.value = 'idle'
  loginId.value = ''
  pairingCode.value = ''
  qrImageDataUrl.value = ''
  pollStatus.value = ''
  if (pendingID) void cancelPendingLogin(pendingID, botIdForPending)
}

function retryLogin() {
  if (isStarting.value) return
  if (activeLoginMode.value === 'phone') {
    void startPhoneLogin()
    return
  }
  void startQrLogin()
}

function isLoginNotFoundError(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const payload = error as { message?: unknown, error?: unknown, detail?: unknown }
  return [payload.message, payload.error, payload.detail].some((value) => (
    typeof value === 'string' && value.toLowerCase().includes('whatsapp login not found')
  ))
}

function clearStatus() {
  statusSeq += 1
  status.value = null
  isLoadingStatus.value = false
}

function clearStatusRefresh() {
  if (statusTimer) {
    clearInterval(statusTimer)
    statusTimer = null
  }
}

function shouldRefreshStatus() {
  return props.configured && loginState.value === 'idle' && !isStarting.value
}

function syncStatusRefresh() {
  if (!shouldRefreshStatus()) {
    clearStatusRefresh()
    return
  }
  if (statusTimer) return
  statusTimer = setInterval(() => {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
    if (isLoadingStatus.value) return
    void loadStatus()
  }, 15000)
}

watch(() => props.botId, (_botId, oldBotId) => {
  cancel(oldBotId || props.botId)
  clearStatus()
  if (props.configured) void loadStatus()
  syncStatusRefresh()
})

watch(() => props.configured, () => {
  if (props.configured) {
    void loadStatus()
    syncStatusRefresh()
    return
  }
  clearStatus()
  syncStatusRefresh()
}, { immediate: true })

watch(loginMode, () => {
  errorMessage.value = ''
})

watch([loginState, isStarting], () => {
  syncStatusRefresh()
})

watch(pairingBusy, (busy) => {
  emit('update:busy', busy)
}, { immediate: true })

onUnmounted(() => {
  const pendingID = loginId.value
  const botID = props.botId
  aborted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  clearStatusRefresh()
  emit('update:busy', false)
  if (pendingID) void cancelPendingLogin(pendingID, botID)
})
</script>
