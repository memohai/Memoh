import { ref } from 'vue'
import { getLanguageByFilename } from '@/components/file-manager/utils'
import { useSettingsStore } from '@/store/settings'

import type { HighlighterGeneric, BundledLanguage, BundledTheme } from 'shiki'

type Highlighter = HighlighterGeneric<BundledLanguage, BundledTheme>

let highlighterPromise: Promise<Highlighter> | null = null
const loadedLangs = new Set<string>(['plaintext'])
const loadedThemes = new Set<string>()

async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = import('shiki').then((m) =>
      m.createHighlighter({ themes: [], langs: [] }),
    )
  }
  return highlighterPromise
}

async function ensureLang(hl: Highlighter, lang: string) {
  if (loadedLangs.has(lang)) return
  try {
    await hl.loadLanguage(lang as BundledLanguage)
    loadedLangs.add(lang)
  } catch {
    loadedLangs.add(lang)
  }
}

async function ensureTheme(hl: Highlighter, theme: BundledTheme) {
  if (loadedThemes.has(theme)) return
  try {
    await hl.loadTheme(theme)
    loadedThemes.add(theme)
  } catch {
    loadedThemes.add(theme)
  }
}

export function useShikiHighlighter() {
  const settings = useSettingsStore()
  const html = ref('')
  const loading = ref(false)

  const activeThemes = () => ({
    light: settings.shikiThemeLight as BundledTheme,
    dark: settings.shikiThemeDark as BundledTheme,
  })

  async function ensurePairedThemes(hl: Highlighter) {
    const themes = activeThemes()
    await Promise.all([ensureTheme(hl, themes.light), ensureTheme(hl, themes.dark)])
    return themes
  }

  async function highlight(code: string, filename: string) {
    loading.value = true
    try {
      const lang = getLanguageByFilename(filename)
      const hl = await getHighlighter()
      await ensureLang(hl, lang)
      const themes = await ensurePairedThemes(hl)
      html.value = hl.codeToHtml(code, {
        lang: loadedLangs.has(lang) ? lang : 'plaintext',
        themes,
      })
    } catch {
      html.value = `<pre>${escapeHtml(code)}</pre>`
    } finally {
      loading.value = false
    }
  }

  // Highlight by an explicit language id (markdown code fences carry the
  // language directly, e.g. ```bash), rather than deriving it from a filename.
  async function highlightLang(code: string, lang: string) {
    loading.value = true
    try {
      const normalized = (lang || 'plaintext').toLowerCase()
      const hl = await getHighlighter()
      await ensureLang(hl, normalized)
      const themes = await ensurePairedThemes(hl)
      html.value = hl.codeToHtml(code, {
        lang: loadedLangs.has(normalized) ? normalized : 'plaintext',
        themes,
      })
    } catch {
      html.value = `<pre>${escapeHtml(code)}</pre>`
    } finally {
      loading.value = false
    }
  }

  async function highlightDiff(oldText: string, newText: string, filename: string) {
    loading.value = true
    try {
      const lang = getLanguageByFilename(filename)
      const hl = await getHighlighter()
      await ensureLang(hl, lang)
      const themes = await ensurePairedThemes(hl)
      const effectiveLang = loadedLangs.has(lang) ? lang : 'plaintext'

      const oldHtml = oldText
        ? hl.codeToHtml(oldText, { lang: effectiveLang, themes })
        : ''
      const newHtml = newText
        ? hl.codeToHtml(newText, { lang: effectiveLang, themes })
        : ''

      html.value =
        (oldHtml ? `<div class="diff-block diff-remove">${oldHtml}</div>` : '') +
        (newHtml ? `<div class="diff-block diff-add">${newHtml}</div>` : '')
    } catch {
      html.value = `<pre>${escapeHtml(`- ${oldText}\n+ ${newText}`)}</pre>`
    } finally {
      loading.value = false
    }
  }

  async function highlightLanguage(code: string, lang: string, options: {
    theme?: BundledTheme
    // Explicit dual-theme override. Pass when the call site needs to dodge the
    // `.dark .shiki span` !important rule that ships in the design system: set
    // both halves to the same theme and shiki emits `--shiki-dark` equal to the
    // light value, so the override resolves back to the picked colors.
    themes?: { light: BundledTheme, dark: BundledTheme }
    transparentPre?: boolean
  } = {}) {
    loading.value = true
    try {
      const hl = await getHighlighter()
      await ensureLang(hl, lang)
      const effectiveLang = loadedLangs.has(lang) ? lang : 'plaintext'
      const transformers = options.transparentPre ? [transparentPreTransformer] : undefined
      if (options.theme) {
        await ensureTheme(hl, options.theme)
        html.value = hl.codeToHtml(code, {
          lang: effectiveLang,
          theme: options.theme,
          transformers,
        })
      } else if (options.themes) {
        await Promise.all([
          ensureTheme(hl, options.themes.light),
          ensureTheme(hl, options.themes.dark),
        ])
        html.value = hl.codeToHtml(code, {
          lang: effectiveLang,
          themes: options.themes,
          transformers,
        })
      } else {
        const themes = await ensurePairedThemes(hl)
        html.value = hl.codeToHtml(code, {
          lang: effectiveLang,
          themes,
          transformers,
        })
      }
    } catch {
      html.value = `<pre>${escapeHtml(code)}</pre>`
    } finally {
      loading.value = false
    }
  }

  return { html, loading, highlight, highlightLang, highlightDiff, highlightLanguage }
}

const transparentPreTransformer = {
  pre(node: { properties?: Record<string, unknown> }) {
    if (node.properties) {
      delete node.properties.class
      delete node.properties.className
      delete node.properties.style
    }
  },
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function resolveLanguage(filename: string): string {
  return getLanguageByFilename(filename)
}

export function extractFilename(path: string): string {
  return path.split('/').pop() ?? path
}
