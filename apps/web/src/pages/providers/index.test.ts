// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { Component, Ref, Slots } from 'vue'

const mocks = vi.hoisted(() => ({
  providerData: undefined as unknown as Ref<Array<Record<string, unknown>> | undefined>,
  providersLoading: undefined as unknown as Ref<boolean>,
  modelData: undefined as unknown as Ref<Array<Record<string, unknown>>>,
  route: { query: {} as Record<string, string> },
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('@pinia/colada', async () => {
  const { ref } = await import('vue')
  mocks.providerData = ref()
  mocks.providersLoading = ref(false)
  mocks.modelData = ref([])
  return {
    useQuery: ({ key }: { key: () => string[] }) => key()[0] === 'providers'
      ? { data: mocks.providerData, isLoading: mocks.providersLoading }
      : { data: mocks.modelData, isLoading: ref(false) },
  }
})

vi.mock('@memohai/sdk', () => ({
  getModels: vi.fn(),
  getProviders: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('lucide-vue-next', () => ({
  Boxes: () => h('span'),
  Box: () => h('span'),
  ChevronRight: () => h('span'),
  Plus: () => h('span'),
  Search: () => h('span'),
}))

vi.mock('@felinic/ui', () => {
  const Passthrough = (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.())
  return {
    Button: Passthrough,
    Empty: Passthrough,
    EmptyContent: Passthrough,
    EmptyDescription: Passthrough,
    EmptyHeader: Passthrough,
    EmptyMedia: Passthrough,
    EmptyTitle: Passthrough,
    InputGroup: Passthrough,
    InputGroupAddon: Passthrough,
    InputGroupInput: Passthrough,
  }
})

vi.mock('@/components/add-provider/index.vue', () => ({ default: () => h('div') }))
vi.mock('@/components/provider-icon/index.vue', () => ({ default: () => h('span') }))
vi.mock('@/components/settings/backend-card.vue', () => ({
  default: {
    props: ['name'],
    emits: ['click'],
    setup(props: { name: string }, { emit }: { emit: (event: 'click') => void }) {
      return () => h('button', {
        'data-provider': props.name,
        'onClick': () => emit('click'),
      }, props.name)
    },
  },
}))
vi.mock('@/components/settings/detail-pane.vue', () => ({
  default: (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.()),
}))
vi.mock('@/components/settings/swap-transition.vue', () => ({
  default: (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.()),
}))
vi.mock('@/components/page-shell/index.vue', () => ({
  default: (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', [slots.actions?.(), slots.default?.()]),
}))
vi.mock('./model-setting.vue', () => ({
  default: {
    props: ['provider'],
    setup(props: { provider?: { id?: string, name?: string } }) {
      return () => h('div', { 'data-testid': 'provider-detail' }, `${props.provider?.id}:${props.provider?.name}`)
    },
  },
}))

let ProviderPage: Component

async function mountPage() {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp(ProviderPage)
  app.mount(root)
  await nextTick()
  return { app, root }
}

describe('provider route state', () => {
  beforeEach(async () => {
    ProviderPage = (await import('./index.vue')).default
    mocks.providerData.value = [
      { id: 'provider-one', name: 'One', enable: true },
      { id: 'provider-two', name: 'Two', enable: true },
    ]
    mocks.providersLoading.value = false
    mocks.modelData.value = []
    mocks.route.query = {}
    mocks.replace.mockReset()
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('waits for a stale provider query to refresh before resolving the URL', async () => {
    mocks.route.query = { provider: 'provider-two' }
    mocks.providerData.value = [
      { id: 'provider-one', name: 'One', enable: true },
    ]
    mocks.providersLoading.value = true

    const { app, root } = await mountPage()

    expect(mocks.replace).not.toHaveBeenCalled()
    expect(root.querySelector('[data-testid="provider-detail"]')).toBeNull()

    mocks.providerData.value = [
      { id: 'provider-one', name: 'One', enable: true },
      { id: 'provider-two', name: 'Two', enable: true },
    ]
    mocks.providersLoading.value = false
    await nextTick()

    expect(root.querySelector('[data-testid="provider-detail"]')?.textContent).toBe('provider-two:Two')
    app.unmount()
  })

  it('writes the selected provider ID instead of a shared detail flag', async () => {
    const { app, root } = await mountPage()

    ;(root.querySelector('[data-provider="Two"]') as HTMLButtonElement).click()
    await nextTick()

    expect(mocks.replace).toHaveBeenCalledWith({ query: { provider: 'provider-two' } })
    expect(root.querySelector('[data-testid="provider-detail"]')?.textContent).toBe('provider-two:Two')
    app.unmount()
  })

  it('returns to the list when the URL references a missing provider', async () => {
    mocks.route.query = { provider: 'missing', tab: 'kept' }

    const { app, root } = await mountPage()

    expect(mocks.replace).toHaveBeenCalledWith({ query: { tab: 'kept' } })
    expect(root.querySelector('[data-testid="provider-detail"]')).toBeNull()
    app.unmount()
  })
})
