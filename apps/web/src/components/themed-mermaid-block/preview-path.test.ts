// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'

// Real settings store stub: only mermaidTheme matters for injection.
const settings = reactive({ mermaidTheme: 'auto' as string, isDark: false })
vi.mock('@/store/settings', () => ({ useSettingsStore: () => settings }))

// Use the REAL markstream-vue (MarkdownRender, setCustomComponents, enableMermaid),
// but replace MermaidBlockNode with a spy so we can read the node.code that the
// renderer actually receives after ThemedMermaidBlock injection.
let captured: { node: { code: string } } | null = null
vi.mock('markstream-vue', async () => {
  const actual = await vi.importActual<typeof import('markstream-vue')>('markstream-vue')
  return {
    ...actual,
    MermaidBlockNode: {
      props: ['node', 'loading', 'isDark'],
      setup(props: { node: { code: string } }) {
        captured = props
        return () => h('div', props.node?.code ?? '')
      },
    },
  }
})

async function flush() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

const PREVIEW_CONTENT = `\`\`\`mermaid
pie
  title Theme palette
  "Chat" : 38
  "Memory" : 27
  "Tools" : 20
  "Skills" : 15
\`\`\``

describe('appearance preview render path (real MarkdownRender + custom-id)', () => {
  afterEach(() => {
    captured = null
  })

  const MERMAID_PREVIEW_PROPS = {
    showHeader: false,
    showModeToggle: false,
    showCopyButton: false,
    showExportButton: false,
    showFullscreenButton: false,
    showCollapseButton: false,
    showZoomControls: false,
    enableMermaidInteractions: false,
  } as const

  async function renderPreview(mermaidProps?: Record<string, unknown>) {
    const markstream = await import('markstream-vue')
    const ThemedMermaidBlock = (await import('./index.vue')).default
    markstream.enableMermaid()
    markstream.setCustomComponents({ mermaid: ThemedMermaidBlock })

    settings.mermaidTheme = 'forest'
    const MarkdownRender = markstream.default
    const app = createApp(MarkdownRender, {
      content: PREVIEW_CONTENT,
      isDark: false,
      typewriter: false,
      fade: false,
      customId: 'appearance-mermaid-preview',
      ...(mermaidProps ? { mermaidProps } : {}),
    })
    app.mount(document.createElement('div'))
    await flush()
    return app
  }

  it('routes through ThemedMermaidBlock and injects the picked theme (no mermaidProps)', async () => {
    const app = await renderPreview()
    expect(captured).not.toBeNull()
    expect(captured?.node.code.startsWith('%%{init: {"theme":"forest"}}%%')).toBe(true)
    app.unmount()
  })

  it('still injects the theme when the appearance preview mermaidProps are passed', async () => {
    const app = await renderPreview(MERMAID_PREVIEW_PROPS)
    expect(captured).not.toBeNull()
    expect(captured?.node.code.startsWith('%%{init: {"theme":"forest"}}%%')).toBe(true)
    app.unmount()
  })

  // The preview must reflect a theme switch WITHOUT a forced :key remount — same
  // reactive path that already works in chat. Proves removing the obsolete
  // mermaidPreviewKey workaround is safe: changing the store theme re-injects.
  it('re-injects the new theme reactively when the store theme changes (no remount)', async () => {
    const app = await renderPreview(MERMAID_PREVIEW_PROPS)
    expect(captured?.node.code.startsWith('%%{init: {"theme":"forest"}}%%')).toBe(true)

    settings.mermaidTheme = 'neutral'
    await flush()
    expect(captured?.node.code.startsWith('%%{init: {"theme":"neutral"}}%%')).toBe(true)
    app.unmount()
  })
})
