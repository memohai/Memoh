import type { Locale } from '@/i18n'
import { watch } from 'vue'
import { defineStore } from 'pinia'
import { useColorMode, useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { isColorSchemeId, type ColorSchemeId } from '@/constants/color-schemes'
import { isFontId, getFontById, type FontId } from '@/constants/fonts'

export interface Settings {
  language: Locale;
  theme: 'light' | 'dark';
  colorScheme: ColorSchemeId;
  fontFamily: FontId;
}

const loadedFontHrefs = new Set<string>()

export function loadFontStylesheet(href: string) {
  if (typeof document === 'undefined') return
  if (loadedFontHrefs.has(href)) return
  loadedFontHrefs.add(href)
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = href
  document.head.appendChild(link)
}

function applyFontFamily(id: FontId) {
  if (typeof document === 'undefined') return
  const font = getFontById(id)
  if (!font) return
  if (font.href) loadFontStylesheet(font.href)
  document.documentElement.style.setProperty('--font-sans', font.family)
}

export const useSettingsStore = defineStore('settings', () => {
  const colorMode = useColorMode()
  const i18n = useI18n()
  const language = useStorage<Locale>('language', 'en')
  const theme = useStorage<'light' | 'dark'>('theme',
    typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  const colorScheme = useStorage<ColorSchemeId>('color-scheme', 'memoh')
  const fontFamily = useStorage<FontId>('font-family', 'system')

  const applyColorScheme = (value: ColorSchemeId) => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.colorScheme = value
  }

  if (!isColorSchemeId(colorScheme.value)) {
    colorScheme.value = 'memoh'
  }

  if (!isFontId(fontFamily.value)) {
    fontFamily.value = 'system'
  }

  watch(theme, (value) => {
    colorMode.value = value
  }, { immediate: true })

  watch(language, (value) => {
    i18n.locale.value = value
  }, { immediate: true })

  watch(colorScheme, (value) => {
    if (!isColorSchemeId(value)) {
      colorScheme.value = 'memoh'
      return
    }
    applyColorScheme(value)
  }, { immediate: true })

  watch(fontFamily, (value) => {
    if (!isFontId(value)) {
      fontFamily.value = 'system'
      return
    }
    applyFontFamily(value)
  }, { immediate: true })

  const setLanguage = (value: Locale) => {
    language.value = value
  }

  const withViewTransition = (fn: () => void) => {
    if (typeof document !== 'undefined' && 'startViewTransition' in document) {
      (document as Document & { startViewTransition: (cb: () => void) => unknown }).startViewTransition(fn)
    } else {
      fn()
    }
  }

  const setTheme = (value: 'light' | 'dark') => {
    withViewTransition(() => {
      document.documentElement.classList.toggle('dark', value === 'dark')
      theme.value = value
    })
  }

  const setColorScheme = (value: ColorSchemeId) => {
    withViewTransition(() => {
      colorScheme.value = value
      applyColorScheme(value)
    })
  }

  const setFontFamily = (value: FontId) => {
    fontFamily.value = value
    applyFontFamily(value)
  }

  return {
    language,
    theme,
    colorScheme,
    fontFamily,
    setLanguage,
    setTheme,
    setColorScheme,
    setFontFamily,
  }
})
