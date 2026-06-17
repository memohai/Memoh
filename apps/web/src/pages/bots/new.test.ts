// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { Slots } from 'vue'

const mocks = vi.hoisted(() => ({
  getBotsNameAvailability: vi.fn(),
  loadCapabilities: vi.fn(),
  routerBack: vi.fn(),
  routerPush: vi.fn(),
  startBotCreate: vi.fn(),
  localWorkspaceEnabled: false,
  getDesktopServerStatus: vi.fn(),
  defaultWorkspacePath: vi.fn(),
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

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({
    back: mocks.routerBack,
    push: mocks.routerPush,
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: translate,
  }),
}))

vi.mock('vue-sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

vi.mock('@vueuse/core', () => ({
  useDebounceFn: (fn: (...args: unknown[]) => unknown) => fn,
}))

vi.mock('@pinia/colada', async () => {
  const { ref } = await import('vue')
  return {
    useQuery: () => ({ data: ref([]) }),
    useQueryCache: () => ({ invalidateQueries: vi.fn() }),
  }
})

vi.mock('@memohai/sdk', () => ({
  getBotsNameAvailability: (...args: unknown[]) => mocks.getBotsNameAvailability(...args),
  getMemoryProviders: vi.fn(async () => ({ data: [] })),
  getModels: vi.fn(async () => ({ data: [] })),
  getProviders: vi.fn(async () => ({ data: [] })),
}))

vi.mock('@memohai/sdk/colada', () => ({
  getBotsQueryKey: () => ['bots'],
}))

vi.mock('@/store/capabilities', () => ({
  useCapabilitiesStore: () => ({
    load: mocks.loadCapabilities,
    get localWorkspaceEnabled() {
      return mocks.localWorkspaceEnabled
    },
  }),
}))

vi.mock('@/store/bot-create-progress', () => ({
  useBotCreateProgressStore: () => ({
    bot: null,
    setupError: null,
    start: mocks.startBotCreate,
    status: 'idle',
    reset: vi.fn(),
  }),
}))

vi.mock('@/composables/useAvatarInitials', () => ({
  useAvatarInitials: () => 'P',
}))

vi.mock('@memohai/ui', async () => {
  const { h } = await import('vue')
  const Passthrough = (_props: Record<string, unknown>, { slots }: { slots: Slots }) => h('div', slots.default?.())
  const Button = (props: Record<string, unknown>, { attrs, slots }: { attrs: Record<string, unknown>, slots: Slots }) =>
    h('button', { ...attrs, disabled: props.disabled, type: props.type ?? 'button' }, slots.default?.())
  const Input = Object.assign((
    props: { modelValue?: string },
    { attrs, emit }: { attrs: Record<string, unknown>, emit: (event: 'update:modelValue', value: string) => void },
  ) =>
    h('input', {
      ...attrs,
      value: props.modelValue ?? '',
      onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
    }), {
    emits: ['update:modelValue'],
  })
  return {
    Avatar: Passthrough,
    AvatarFallback: Passthrough,
    AvatarImage: Passthrough,
    Button,
    Input,
    Label: Passthrough,
    Select: Passthrough,
    SelectContent: Passthrough,
    SelectItem: Passthrough,
    SelectTrigger: Passthrough,
    SelectValue: Passthrough,
    Separator: Passthrough,
    Spinner: Passthrough,
    Tabs: Passthrough,
    TabsList: Passthrough,
    TabsTrigger: Passthrough,
    Tooltip: Passthrough,
    TooltipContent: Passthrough,
    TooltipTrigger: Passthrough,
  }
})

vi.mock('lucide-vue-next', async () => {
  const { h } = await import('vue')
  const Icon = () => h('span')
  return {
    Check: Icon,
    CircleHelp: Icon,
    LoaderCircle: Icon,
    SquarePen: Icon,
    X: Icon,
  }
})

vi.mock('@/components/timezone-select/index.vue', () => ({ default: (_props: Record<string, unknown>) => h('select') }))
vi.mock('./components/avatar-edit-dialog.vue', () => ({ default: () => h('div') }))
vi.mock('./components/bot-import-panel.vue', () => ({ default: () => h('div') }))
vi.mock('./components/memory-provider-select.vue', () => ({ default: () => h('select') }))
vi.mock('./components/model-select.vue', () => ({ default: () => h('select') }))

describe('bot create page', () => {
  beforeEach(() => {
    mocks.getBotsNameAvailability.mockReset()
    mocks.loadCapabilities.mockReset()
    mocks.routerBack.mockReset()
    mocks.routerPush.mockReset()
    mocks.startBotCreate.mockReset()
    mocks.getDesktopServerStatus.mockReset()
    mocks.defaultWorkspacePath.mockReset()
    mocks.getBotsNameAvailability.mockResolvedValue({ data: { available: true } })
    mocks.localWorkspaceEnabled = false
    delete (window as unknown as { api?: unknown }).api
    document.body.innerHTML = ''
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('clears submit loading after handing container creation to the progress route', async () => {
    const Page = (await import('./new.vue')).default
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(Page)
    app.config.globalProperties.$t = translate
    app.mount(root)
    await flushPromises()

    const [displayInput] = Array.from(root.querySelectorAll('input'))
    displayInput!.value = 'Prog'
    displayInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    const form = root.querySelector('form')!
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(mocks.startBotCreate).toHaveBeenCalledTimes(1)
    expect(mocks.routerPush).toHaveBeenCalledWith({ name: 'bot-create-progress' })
    expect(form.getAttribute('aria-busy')).toBe('false')

    app.unmount()
    root.remove()
  })

  it('hides local workspace creation in remote desktop even when the server supports it', async () => {
    mocks.localWorkspaceEnabled = true
    mocks.getDesktopServerStatus.mockResolvedValue({
      mode: 'remote',
      baseUrl: 'https://memoh.example.com',
      ready: true,
      managed: false,
    })
    ;(window as unknown as { api: unknown }).api = {
      desktop: {
        getServerStatus: mocks.getDesktopServerStatus,
        defaultWorkspacePath: mocks.defaultWorkspacePath,
      },
    }

    const Page = (await import('./new.vue')).default
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(Page)
    app.config.globalProperties.$t = translate
    app.mount(root)
    await flushPromises()

    expect(root.textContent).not.toContain('bots.workspaceBackend')

    const [displayInput, nameInput] = Array.from(root.querySelectorAll('input'))
    displayInput!.value = 'Remote Bot'
    displayInput!.dispatchEvent(new Event('input', { bubbles: true }))
    nameInput!.value = 'remote-bot'
    nameInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    await flushPromises()

    const submitButton = root.querySelector('button[type="submit"]') as HTMLButtonElement
    expect(submitButton.disabled).toBe(false)

    root.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => {
      expect(mocks.startBotCreate).toHaveBeenCalledTimes(1)
    })

    expect(mocks.startBotCreate.mock.calls[0]?.[0]).toMatchObject({
      display_name: 'Remote Bot',
      metadata: undefined,
    })
    expect(mocks.routerPush).toHaveBeenCalledWith({ name: 'bot-create-progress' })

    app.unmount()
    root.remove()
  })

  it('shows local workspace creation in local desktop and resolves the default path', async () => {
    mocks.localWorkspaceEnabled = true
    mocks.getDesktopServerStatus.mockResolvedValue({
      mode: 'local',
      baseUrl: 'http://127.0.0.1:18731',
      ready: true,
      managed: true,
    })
    mocks.defaultWorkspacePath.mockResolvedValue('/Users/test/.memoh/workspaces/local-bot')
    ;(window as unknown as { api: unknown }).api = {
      desktop: {
        getServerStatus: mocks.getDesktopServerStatus,
        defaultWorkspacePath: mocks.defaultWorkspacePath,
      },
    }

    const Page = (await import('./new.vue')).default
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(Page)
    app.config.globalProperties.$t = translate
    app.mount(root)
    await flushPromises()

    expect(root.textContent).toContain('bots.workspaceBackend')

    const [displayInput] = Array.from(root.querySelectorAll('input'))
    displayInput!.value = 'Local Bot'
    displayInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    expect(mocks.defaultWorkspacePath).toHaveBeenCalledWith('Local Bot')
    expect(Array.from(root.querySelectorAll('input')).some(input =>
      input.value === '/Users/test/.memoh/workspaces/local-bot',
    )).toBe(true)

    app.unmount()
    root.remove()
  })

  it('keeps browser web local workspace behavior based on server capabilities', async () => {
    mocks.localWorkspaceEnabled = true

    const Page = (await import('./new.vue')).default
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(Page)
    app.config.globalProperties.$t = translate
    app.mount(root)
    await flushPromises()

    expect(root.textContent).toContain('bots.workspaceBackend')

    app.unmount()
    root.remove()
  })
})
