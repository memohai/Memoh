<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div>
        <h4 class="text-xs font-medium">
          {{ $t('bots.channels.weixinQr.title') }}
        </h4>
        <p class="text-xs text-muted-foreground mt-1">
          {{ $t('bots.channels.weixinQr.description') }}
        </p>
      </div>
    </div>

    <!-- QR code display -->
    <div
      v-if="qrState === 'idle'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <Button
        :disabled="isStarting"
        @click="startLogin"
      >
        <Spinner
          v-if="isStarting"
          class="mr-1.5"
        />
        <QrCode
          v-else
          class="mr-1.5 size-3.5"
        />
        {{ $t('bots.channels.weixinQr.startScan') }}
      </Button>
    </div>

    <div
      v-else-if="qrState === 'showing'"
      class="flex flex-col items-center gap-4 py-4"
    >
      <div class="relative rounded-lg border bg-white p-3">
        <img
          v-if="qrImageDataUrl"
          :src="qrImageDataUrl"
          alt="WeChat QR Code"
          class="size-52"
        >
        <div
          v-else
          class="size-52 flex items-center justify-center text-muted-foreground"
        >
          <Spinner />
        </div>

        <!-- Overlay for scanned state -->
        <div
          v-if="pollStatus === 'scanned'"
          class="absolute inset-0 flex items-center justify-center rounded-lg bg-background/80"
        >
          <div class="text-center">
            <Smartphone
              class="size-8 text-primary mb-2"
            />
            <p class="text-xs font-medium text-foreground">
              {{ $t('bots.channels.weixinQr.scanned') }}
            </p>
          </div>
        </div>

        <!-- Overlay for expired state -->
        <div
          v-if="pollStatus === 'expired'"
          class="absolute inset-0 flex flex-col items-center justify-center rounded-lg bg-background/80 gap-2"
        >
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.weixinQr.expired') }}
          </p>
          <Button
            size="sm"
            variant="outline"
            @click="startLogin"
          >
            {{ $t('bots.channels.weixinQr.refresh') }}
          </Button>
        </div>
      </div>

      <p class="text-xs text-muted-foreground text-center max-w-xs">
        {{ statusText }}
      </p>

      <Button
        variant="ghost"
        size="sm"
        @click="cancel"
      >
        {{ $t('common.cancel') }}
      </Button>
    </div>

    <div
      v-else-if="qrState === 'success'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <div class="flex size-12 items-center justify-center rounded-full bg-success-soft">
        <Check
          class="size-5 text-success-foreground"
        />
      </div>
      <p class="text-xs font-medium">
        {{ $t('bots.channels.weixinQr.success') }}
      </p>
    </div>

    <div
      v-else-if="qrState === 'error'"
      class="flex flex-col items-center gap-3 py-4"
    >
      <p class="text-xs text-destructive">
        {{ errorMessage }}
      </p>
      <Button
        variant="outline"
        size="sm"
        @click="startLogin"
      >
        {{ $t('bots.channels.weixinQr.retry') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { QrCode, Smartphone, Check } from 'lucide-vue-next'
import { ref, computed, onUnmounted } from 'vue'
import { Button, Spinner } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import QRCode from 'qrcode'
import { client } from '@memohai/sdk/client'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const emit = defineEmits<{
  loginSuccess: []
}>()

const { t } = useI18n()

type QRState = 'idle' | 'showing' | 'success' | 'error'

interface WeixinQrStartResponse {
  qr_code_url?: string
  qr_code?: string
  message?: string
}

interface WeixinQrPollResponse {
  status?: string
  message?: string
}

const qrState = ref<QRState>('idle')
const qrCode = ref('')
const qrImageDataUrl = ref('')
const pollStatus = ref('')
const isStarting = ref(false)
const errorMessage = ref('')
let pollTimer: ReturnType<typeof setTimeout> | null = null
let aborted = false

const statusText = computed(() => {
  switch (pollStatus.value) {
    case 'wait':
      return t('bots.channels.weixinQr.waitingScan')
    case 'scanned':
      return t('bots.channels.weixinQr.scanned')
    case 'expired':
      return t('bots.channels.weixinQr.expired')
    default:
      return t('bots.channels.weixinQr.waitingScan')
  }
})

async function startLogin() {
  aborted = false
  isStarting.value = true
  errorMessage.value = ''
  pollStatus.value = ''
  qrImageDataUrl.value = ''

  try {
    const { data } = await client.post<{ 200: WeixinQrStartResponse }, unknown, true>({
      url: '/bots/{bot_id}/channel/weixin/qr/start',
      path: { bot_id: props.botId },
      body: {},
      throwOnError: true,
    })
    const qrContent = data.qr_code_url || data.qr_code || ''
    if (!qrContent) {
      throw new Error('No QR code data returned')
    }

    qrCode.value = data.qr_code || ''
    qrImageDataUrl.value = await QRCode.toDataURL(qrContent, { width: 208, margin: 1 })
    qrState.value = 'showing'

    startPolling()
  } catch (err) {
    errorMessage.value = resolveApiErrorMessage(err, err instanceof Error ? err.message : String(err))
    qrState.value = 'error'
  } finally {
    isStarting.value = false
  }
}

function startPolling() {
  if (aborted) return
  pollOnce()
}

async function pollOnce() {
  if (aborted || qrState.value !== 'showing') return

  try {
    const { data } = await client.post<{ 200: WeixinQrPollResponse }, unknown, true>({
      url: '/bots/{bot_id}/channel/weixin/qr/poll',
      path: { bot_id: props.botId },
      body: {
        qr_code: qrCode.value,
      },
      throwOnError: true,
    })
    pollStatus.value = data.status ?? ''

    switch (data.status) {
      case 'confirmed':
        qrState.value = 'success'
        toast.success(t('bots.channels.weixinQr.success'))
        emit('loginSuccess')
        return
      case 'expired':
        return
      case 'wait':
      case 'scanned':
        if (!aborted) {
          pollTimer = setTimeout(pollOnce, 1500)
        }
        return
      default:
        if (!aborted) {
          pollTimer = setTimeout(pollOnce, 2000)
        }
    }
  } catch {
    if (!aborted) {
      pollTimer = setTimeout(pollOnce, 3000)
    }
  }
}

function cancel() {
  aborted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  qrState.value = 'idle'
  qrCode.value = ''
  qrImageDataUrl.value = ''
  pollStatus.value = ''
}

onUnmounted(() => {
  aborted = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
})
</script>
