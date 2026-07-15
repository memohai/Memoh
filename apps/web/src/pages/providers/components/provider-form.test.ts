// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { Slots } from 'vue'

const mocks = vi.hoisted(() => ({
  getOAuthStatus: vi.fn(),
  pollOAuth: vi.fn(),
  syncCatalog: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

function translate(key: string) {
  return key
}

async function flushPromises() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: translate }),
}))

vi.mock('@memohai/sdk', () => ({
  deleteProvidersByIdOauthToken: vi.fn(),
  getProvidersByIdOauthAuthorize: vi.fn(),
  getProvidersByIdOauthStatus: mocks.getOAuthStatus,
  postProvidersByIdOauthPoll: mocks.pollOAuth,
  postProvidersByIdTest: vi.fn(),
}))

vi.mock('@/composables/useProviderModelCatalog', () => ({
  useProviderModelCatalog: () => ({ syncProviderModelCatalog: mocks.syncCatalog }),
}))

vi.mock('lucide-vue-next', () => ({
  AlertCircle: () => h('span'),
  KeyRound: () => h('span'),
  RefreshCw: () => h('span'),
}))

vi.mock('@felinic/ui', async () => {
  const { h } = await import('vue')
  const Passthrough = (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.())
  const FormField = (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.({
    componentField: {},
    errorMessage: '',
  }))
  return {
    AutoHeight: Passthrough,
    Button: Passthrough,
    FormControl: Passthrough,
    FormField,
    FormItem: Passthrough,
    FormMessage: Passthrough,
    HoverCard: Passthrough,
    HoverCardContent: Passthrough,
    HoverCardTrigger: Passthrough,
    Input: Passthrough,
    LabelSwap: Passthrough,
    Select: Passthrough,
    SelectContent: Passthrough,
    SelectItem: Passthrough,
    SelectTrigger: Passthrough,
    SelectValue: Passthrough,
    Spinner: Passthrough,
    toast: {
      success: mocks.toastSuccess,
      error: mocks.toastError,
    },
  }
})

vi.mock('@/components/confirm-popover/index.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/device-code-panel/index.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/loading-button/index.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/settings/row.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/settings/section.vue', () => ({ default: { template: '<div><slot /></div>' } }))

describe('provider OAuth model sync', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    mocks.getOAuthStatus.mockResolvedValue({
      data: {
        configured: true,
        mode: 'device',
        has_token: false,
        expired: false,
        device: {
          pending: true,
          user_code: 'ABCD-EFGH',
          verification_uri: 'https://github.com/login/device',
          interval_seconds: 1,
        },
      },
    })
    mocks.pollOAuth.mockResolvedValue({
      data: {
        configured: true,
        mode: 'device',
        has_token: true,
        expired: false,
      },
    })
    mocks.syncCatalog.mockResolvedValue({ created: 2, updated: 1 })
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('syncs the managed catalog when device authorization completes', async () => {
    const ProviderForm = (await import('./provider-form.vue')).default
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(ProviderForm, {
      provider: {
        id: 'provider-id',
        name: 'GitHub Copilot',
        client_type: 'github-copilot',
        enable: true,
        config: {},
      },
      editLoading: false,
    })
    app.config.globalProperties.$t = translate
    app.mount(root)
    await flushPromises()

    expect(mocks.getOAuthStatus).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(mocks.pollOAuth).toHaveBeenCalledOnce()
    expect(mocks.syncCatalog).toHaveBeenCalledOnce()
    expect(mocks.syncCatalog).toHaveBeenCalledWith('provider-id')
    expect(mocks.toastSuccess).toHaveBeenCalledWith('provider.oauth.authorizeSuccess')

    app.unmount()
    root.remove()
  })
})
