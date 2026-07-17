// @vitest-environment jsdom
/* eslint-disable vue/one-component-per-file */

import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

const SlotComponent = (name: string) => defineComponent({
  name,
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const EmptyComponent = (name: string) => defineComponent({
  name,
  setup() {
    return () => h('span')
  },
})

vi.mock('@felinic/ui', () => ({
  Popover: SlotComponent('Popover'),
  PopoverAnchor: SlotComponent('PopoverAnchor'),
  PopoverContent: SlotComponent('PopoverContent'),
  ScrollArea: defineComponent({
    name: 'ScrollArea',
    setup(_, { slots }) {
      return () => h('div', { 'data-slot': 'scroll-area-viewport' }, slots.default?.())
    },
  }),
}))

vi.mock('lucide-vue-next', () => ({
  Check: EmptyComponent('Check'),
  ChevronRight: EmptyComponent('ChevronRight'),
  Lightbulb: EmptyComponent('Lightbulb'),
  X: EmptyComponent('X'),
}))

vi.mock('@tanstack/vue-virtual', () => ({
  useVirtualizer: () => ref({
    getTotalSize: () => 38,
    getVirtualItems: () => [{ index: 0, key: 'model-1', start: 0 }],
    measureElement: vi.fn(),
    scrollToOffset: vi.fn(),
  }),
}))

vi.mock('@/components/model-description-tooltip/index.vue', () => ({
  default: defineComponent({
    name: 'ModelDescriptionTooltip',
    props: {
      description: {
        type: String,
        default: undefined,
      },
      open: Boolean,
    },
    emits: ['update:open'],
    setup(props, { emit, slots }) {
      return () => h('div', {
        'data-model-tooltip': '',
        'data-open': String(props.open),
        onPointerenter: () => emit('update:open', true),
      }, slots.default?.())
    },
  }),
}))

describe('ChatModelPicker', () => {
  let app: ReturnType<typeof createApp> | undefined
  let root: HTMLDivElement | undefined

  afterEach(() => {
    app?.unmount()
    root?.remove()
    app = undefined
    root = undefined
  })

  async function mountPicker(overrides: Record<string, unknown> = {}) {
    const ChatModelPicker = (await import('./chat-model-picker.vue')).default
    root = document.createElement('div')
    document.body.append(root)
    app = createApp(ChatModelPicker, {
      models: [{
        id: 'model-1',
        model_id: 'model-1',
        name: 'Model 1',
        provider_id: '',
        type: 'chat',
        config: { description: 'Model description' },
      }],
      providers: [],
      modelType: 'chat',
      open: true,
      modelValue: 'model-1',
      reasoningEffort: 'disable',
      ...overrides,
    })
    app.config.globalProperties.$t = (key: string) => key
    app.mount(root)
    await nextTick()
    await nextTick()
    return root
  }

  it('dismisses an open model description when the list scrolls', async () => {
    const el = await mountPicker()
    const tooltip = el.querySelector<HTMLElement>('[data-model-tooltip]')
    const viewport = el.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]')

    expect(tooltip).not.toBeNull()
    expect(viewport).not.toBeNull()

    tooltip!.dispatchEvent(new Event('pointerenter'))
    await nextTick()
    expect(tooltip!.dataset.open).toBe('true')

    viewport!.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(tooltip!.dataset.open).toBe('false')
  })

  it('renders and emits agent-provided reasoning options without guessing fixed levels', async () => {
    const updateReasoning = vi.fn()
    const el = await mountPicker({
      reasoningEffort: 'ultra',
      reasoningOptions: [
        { value: 'balanced', label: 'Balanced by agent' },
        { value: 'ultra', label: 'Maximal deliberation', description: 'Agent-defined option' },
      ],
      'onUpdate:reasoningEffort': updateReasoning,
    })

    expect(el.textContent).toContain('Maximal deliberation')
    const option = Array.from(el.querySelectorAll('button')).find(button => button.textContent?.includes('Balanced by agent'))
    expect(option).toBeDefined()
    option!.click()
    await nextTick()
    expect(updateReasoning).toHaveBeenCalledWith('balanced')
  })

  it('bounds a long agent-provided reasoning list and dismisses its tooltip on scroll', async () => {
    const el = await mountPicker({
      reasoningOptions: Array.from({ length: 10 }, (_, index) => ({
        value: `effort-${index}`,
        label: `Effort ${index}`,
        description: `Description ${index}`,
      })),
    })
    const host = el.querySelector<HTMLElement>('.reasoning-effort-list')
    const tooltip = Array.from(el.querySelectorAll<HTMLElement>('[data-model-tooltip]'))
      .find(item => item.textContent?.includes('Effort 0'))
    const viewport = host?.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]')

    expect(host?.style.height).toBe('288px')
    expect(tooltip).toBeDefined()
    expect(viewport).not.toBeNull()

    tooltip!.dispatchEvent(new Event('pointerenter'))
    await nextTick()
    expect(tooltip!.dataset.open).toBe('true')

    viewport!.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(tooltip!.dataset.open).toBe('false')
  })
})
