// @vitest-environment jsdom
/* eslint-disable vue/one-component-per-file */

import { createApp, defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

const TooltipPart = (name: string) => defineComponent({
  name,
  setup(_, { slots }) {
    return () => h('div', { [`data-${name}`]: '' }, slots.default?.())
  },
})

vi.mock('@felinic/ui', () => ({
  Tooltip: TooltipPart('tooltip'),
  TooltipContent: TooltipPart('tooltip-content'),
  TooltipProvider: TooltipPart('tooltip-provider'),
  TooltipTrigger: TooltipPart('tooltip-trigger'),
}))

describe('ModelDescriptionTooltip', () => {
  let app: ReturnType<typeof createApp> | undefined
  let root: HTMLDivElement | undefined

  afterEach(() => {
    app?.unmount()
    root?.remove()
    app = undefined
    root = undefined
  })

  async function mount(description?: string) {
    const ModelDescriptionTooltip = (await import('./index.vue')).default
    root = document.createElement('div')
    document.body.append(root)
    app = createApp({
      render: () => h(ModelDescriptionTooltip, { description }, {
        default: () => h('button', 'Model'),
      }),
    })
    app.mount(root)
    await nextTick()
    return root
  }

  it('renders tooltip content for a non-empty description', async () => {
    const el = await mount('  General purpose model.  ')
    expect(el.querySelector('[data-tooltip-content]')?.textContent).toBe('General purpose model.')
  })

  it('renders only the trigger slot for an empty description', async () => {
    const el = await mount('   ')
    expect(el.textContent).toBe('Model')
    expect(el.querySelector('[data-tooltip]')).toBeNull()
  })
})
