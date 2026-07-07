// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive, ref, type Ref } from 'vue'

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

async function mountWith(theme: string, code: string, version?: Ref<number>) {
  settings.mermaidTheme = theme
  const node = { type: 'code_block', language: 'mermaid', code, raw: code }
  const ThemedMermaidBlock = (await import('./index.vue')).default
  const app = createApp(ThemedMermaidBlock, { node, isDark: false })
  if (version) app.provide('markstreamStreamVersion', version)
  app.mount(document.createElement('div'))
  await flushPromises()
  return { app, node }
}

describe('ThemedMermaidBlock', () => {
  afterEach(() => {
    captured = null
  })

  it('injects the theme directive into the source the renderer reads (node.code)', async () => {
    const { app } = await mountWith('forest', 'graph TD\n  A-->B')
    expect(captured?.node.code).toBe('%%{init: {"theme":"forest"}}%%\ngraph TD\n  A-->B')
    app.unmount()
  })

  it('passes the source through untouched for the auto theme', async () => {
    const source = 'graph TD\n  A-->B'
    const { app } = await mountWith('auto', source)
    expect(captured?.node.code).toBe(source)
    app.unmount()
  })

  it('re-applies the theme when markstream mutates node.code in place during streaming', async () => {
    const version = ref(0)
    const { app, node } = await mountWith('forest', 'graph TD', version)
    expect(captured?.node.code).toBe('%%{init: {"theme":"forest"}}%%\ngraph TD')

    node.code = 'graph TD\n  A-->B'
    version.value++
    await flushPromises()
    expect(captured?.node.code).toBe('%%{init: {"theme":"forest"}}%%\ngraph TD\n  A-->B')
    app.unmount()
  })
})
