// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'

const settings = reactive({ mermaidTheme: 'auto' as string })

vi.mock('@/store/settings', () => ({
  useSettingsStore: () => settings,
}))

interface CapturedProps {
  node: { code: string }
  loading?: boolean
  isDark?: boolean
}

let captured: CapturedProps | null = null

vi.mock('markstream-vue', () => ({
  MermaidBlockNode: {
    props: ['node', 'loading', 'isDark'],
    setup(props: CapturedProps) {
      captured = props
      return () => h('div')
    },
  },
}))

async function flushPromises() {
  await Promise.resolve()
  await nextTick()
}

async function mountWith(theme: string, code: string) {
  settings.mermaidTheme = theme
  const ThemedMermaidBlock = (await import('./index.vue')).default
  const app = createApp(ThemedMermaidBlock, {
    node: { type: 'code_block', language: 'mermaid', code, raw: code },
    isDark: false,
  })
  const root = document.createElement('div')
  app.mount(root)
  await flushPromises()
  return app
}

describe('ThemedMermaidBlock', () => {
  afterEach(() => {
    captured = null
  })

  it('injects the theme directive into the source the renderer reads (node.code)', async () => {
    const app = await mountWith('forest', 'graph TD\n  A-->B')
    expect(captured?.node.code).toBe('%%{init: {"theme":"forest"}}%%\ngraph TD\n  A-->B')
    app.unmount()
  })

  it('passes the source through untouched for the auto theme', async () => {
    const source = 'graph TD\n  A-->B'
    const app = await mountWith('auto', source)
    expect(captured?.node.code).toBe(source)
    app.unmount()
  })
})
