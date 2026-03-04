import { ref } from 'vue'
import { getLanguageByFilename } from '@/components/file-manager/utils'

import type { HighlighterGeneric, BundledLanguage, BundledTheme } from 'shiki'

type Highlighter = HighlighterGeneric<BundledLanguage, BundledTheme>

let highlighterPromise: Promise<Highlighter> | null = null
const loadedLangs = new Set<string>(['plaintext'])

async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = import('shiki').then((m) =>
      m.createHighlighter({ themes: ['github-dark', 'github-light'], langs: [] }),
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

export function useShikiHighlighter() {
  const html = ref('')
  const loading = ref(false)

  async function highlight(code: string, filename: string) {
    loading.value = true
    try {
      const lang = getLanguageByFilename(filename)
      const hl = await getHighlighter()
      await ensureLang(hl, lang)
      html.value = hl.codeToHtml(code, {
        lang: loadedLangs.has(lang) ? lang : 'plaintext',
        themes: { light: 'github-light', dark: 'github-dark' },
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
      const effectiveLang = loadedLangs.has(lang) ? lang : 'plaintext'
      const themes = { light: 'github-light', dark: 'github-dark' }

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

  return { html, loading, highlight, highlightDiff }
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
