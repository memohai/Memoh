import { onScopeDispose, ref } from 'vue'
import {
  deleteBotsByBotIdContainerDisplaySessionsBySessionId,
  getBotsByBotIdContainerDisplay,
  postBotsByBotIdContainerDisplayWebrtcOffer,
} from '@memohai/sdk'
import i18n from '@/i18n'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  postBotsByBotIdContainerDisplayPrepareStream,
  type DisplayPrepareStreamEvent,
} from '@/composables/api/useDisplayPrepareStream'

export type DisplayStatus = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'unavailable'

export interface PrepareProgress {
  percent: number
  message: string
  step?: string
}

interface DisplayInfoPayload {
  enabled?: boolean
  available?: boolean
  running?: boolean
  encoder_available?: boolean
  desktop_available?: boolean
  browser_available?: boolean
  toolkit_available?: boolean
  prepare_supported?: boolean
  prepare_system?: string
  unavailable_reason?: string
}

interface DisplayOfferResponse {
  type: 'answer'
  sdp: string
  session_id?: string
}

const PREPARE_MAX_WAIT_ATTEMPTS = 12
const PREPARE_WAIT_INTERVAL_MS = 500

const connections = new Map<string, BotDisplayConnection>()

function delay(ms: number): Promise<void> {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

export class BotDisplayConnection {
  botId: string
  prepareProgress = ref<PrepareProgress | null>(null)
  unavailableReason = ref('')

  private ensureReadyPromise: Promise<boolean> | null = null
  private refs = 0

  constructor(botId: string) {
    this.botId = botId
  }

  addRef() { this.refs++ }
  removeRef() {
    this.refs--
    if (this.refs <= 0) {
      connections.delete(this.botId)
    }
  }

  /**
   * Ensure the remote display is ready to accept a WebRTC connection.
   * Only one caller per bot will run the actual prepare/check sequence;
   * concurrent callers wait for the same result.
   */
  async ensureReady(): Promise<boolean> {
    if (this.ensureReadyPromise) return this.ensureReadyPromise

    this.ensureReadyPromise = this.doEnsureReady().finally(() => {
      this.ensureReadyPromise = null
    })
    return this.ensureReadyPromise
  }

  private async doEnsureReady(): Promise<boolean> {
    this.unavailableReason.value = ''
    this.prepareProgress.value = null

    try {
      let info = await this.loadDisplayInfo()
      if (!info.enabled) {
        this.unavailableReason.value = this.t('chat.display.unavailable.disabled')
        return false
      }
      if (this.canPrepareDisplay(info)) {
        const prepared = await this.prepareDisplay()
        if (!prepared) return false
        info = await this.waitForDisplayReady()
      }
      if (!info.available || !info.running) {
        this.unavailableReason.value = this.formatUnavailableReason(info.unavailable_reason ?? '')
        return false
      }
      if (info.desktop_available === false) {
        this.unavailableReason.value = this.formatUnavailableReason('desktop unavailable')
        return false
      }
      if (info.browser_available === false) {
        this.unavailableReason.value = this.formatUnavailableReason('browser unavailable')
        return false
      }
      return true
    } catch (error) {
      this.unavailableReason.value = resolveApiErrorMessage(error, this.t('chat.display.status.unavailable'))
      return false
    } finally {
      if (this.unavailableReason.value) {
        this.prepareProgress.value = null
      }
    }
  }

  /**
   * Exchange offer/answer for the given peer and set the remote description.
   * The caller owns the peer and should attach event listeners before calling this.
   */
  async exchangeOffer(peer: RTCPeerConnection, existingSessionId?: string): Promise<DisplayOfferResponse> {
    const offer = await peer.createOffer()
    await peer.setLocalDescription(offer)
    await this.waitForIceGatheringComplete(peer)

    const local = peer.localDescription
    if (!local?.sdp) {
      throw new Error('local WebRTC offer is unavailable')
    }
    const { data } = await postBotsByBotIdContainerDisplayWebrtcOffer({
      path: { bot_id: this.botId },
      body: {
        type: local.type,
        sdp: local.sdp,
        session_id: existingSessionId || undefined,
        candidate_host: window.location.hostname,
      },
      throwOnError: true,
    })
    if (!data?.sdp) {
      throw new Error('display WebRTC answer is empty')
    }
    const answer: DisplayOfferResponse = { type: 'answer', sdp: data.sdp, session_id: data.session_id }
    await peer.setRemoteDescription(new RTCSessionDescription(answer))
    this.prepareProgress.value = null
    return answer
  }

  async closeSession(sessionID: string) {
    if (!sessionID) return
    void deleteBotsByBotIdContainerDisplaySessionsBySessionId({
      path: { bot_id: this.botId, session_id: sessionID },
    }).catch(() => {})
  }

  private async loadDisplayInfo(): Promise<DisplayInfoPayload> {
    const { data } = await getBotsByBotIdContainerDisplay({
      path: { bot_id: this.botId },
      throwOnError: true,
    })
    return data ?? {}
  }

  private isDisplayReady(info: DisplayInfoPayload): boolean {
    return info.enabled === true
      && info.available === true
      && info.running === true
      && info.desktop_available !== false
      && info.browser_available !== false
  }

  private canPrepareDisplay(info: DisplayInfoPayload): boolean {
    const reason = info.unavailable_reason ?? ''
    if (!info.enabled) return false
    if (reason === 'container not reachable' || reason === 'manager not configured') return false
    if (info.encoder_available === false && reason === 'gstreamer unavailable') return false
    return !info.available
      || !info.running
      || info.desktop_available === false
      || info.browser_available === false
  }

  private async waitForDisplayReady(): Promise<DisplayInfoPayload> {
    let last = await this.loadDisplayInfo()
    for (let attempt = 0; attempt < PREPARE_MAX_WAIT_ATTEMPTS && !this.isDisplayReady(last); attempt += 1) {
      await delay(PREPARE_WAIT_INTERVAL_MS)
      last = await this.loadDisplayInfo()
    }
    return last
  }

  private async prepareDisplay(): Promise<boolean> {
    this.prepareProgress.value = {
      percent: 5,
      message: this.t('chat.display.prepare.checking'),
      step: 'checking',
    }
    try {
      const { stream } = await postBotsByBotIdContainerDisplayPrepareStream({
        path: { bot_id: this.botId },
        throwOnError: true,
      })
      for await (const event of stream) {
        if (event.type === 'error') {
          throw new Error(event.message)
        }
        this.prepareProgress.value = {
          percent: event.percent ?? this.prepareProgress.value?.percent ?? 0,
          message: this.prepareEventMessage(event),
          step: event.step ?? this.prepareProgress.value?.step,
        }
        if (event.type === 'complete') {
          return true
        }
      }
      return true
    } catch (error) {
      this.status.value = 'unavailable'
      this.unavailableReason.value = resolveApiErrorMessage(error, this.t('chat.display.prepare.failed'))
      return false
    }
  }

  private prepareEventMessage(event: DisplayPrepareStreamEvent): string {
    switch (event.step) {
      case 'checking': return this.t('chat.display.prepare.checking')
      case 'toolkit': return this.t('chat.display.prepare.toolkit')
      case 'system': return this.t('chat.display.prepare.system')
      case 'installing': return this.t('chat.display.prepare.installing')
      case 'browser': return this.t('chat.display.prepare.browser')
      case 'starting': return this.t('chat.display.prepare.starting')
      case 'desktop': return this.t('chat.display.prepare.desktop')
      case 'styling': return this.t('chat.display.prepare.styling')
      case 'complete': return this.t('chat.display.prepare.complete')
      default: return event.message || this.t('chat.display.prepare.default')
    }
  }

  private async waitForIceGatheringComplete(pc: RTCPeerConnection): Promise<void> {
    if (pc.iceGatheringState === 'complete') return
    return new Promise((resolve) => {
      const timeout = window.setTimeout(done, 3000)
      function done() {
        window.clearTimeout(timeout)
        pc.removeEventListener('icegatheringstatechange', onChange)
        resolve()
      }
      function onChange() {
        if (pc.iceGatheringState === 'complete') done()
      }
      pc.addEventListener('icegatheringstatechange', onChange)
    })
  }

  private t(key: string) {
    return i18n.global.t(key)
  }

  private formatUnavailableReason(reason: string): string {
    const map: Record<string, string> = {
      'container not reachable': this.t('chat.display.unavailable.container'),
      'display bundle unavailable': this.t('chat.display.unavailable.bundle'),
      'display server not reachable': this.t('chat.display.unavailable.server'),
      'gstreamer unavailable': this.t('chat.display.unavailable.encoder'),
      'manager not configured': this.t('chat.display.unavailable.manager'),
      'browser unavailable': this.t('chat.display.unavailable.browser'),
      'desktop unavailable': this.t('chat.display.unavailable.desktop'),
      'toolkit unavailable': this.t('chat.display.unavailable.toolkit'),
    }
    return map[reason] || reason || this.t('chat.display.status.unavailable')
  }
}

export function useBotDisplayConnection(botId: string) {
  let conn = connections.get(botId)
  if (!conn) {
    conn = new BotDisplayConnection(botId)
    connections.set(botId, conn)
  }
  conn.addRef()
  onScopeDispose(() => conn?.removeRef())
  return conn
}
