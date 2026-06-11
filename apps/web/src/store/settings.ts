import type { Locale } from '@/i18n'
import { watch } from 'vue'
import { defineStore } from 'pinia'
import { useColorMode, useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { isColorSchemeId, type ColorSchemeId } from '@/constants/color-schemes'

export type ThemePreference = 'light' | 'dark' | 'system'

export interface Settings {
  language: Locale;
  theme: ThemePreference;
  colorScheme: ColorSchemeId;
}

export const useSettingsStore = defineStore('settings', () => {
  const colorMode = useColorMode({ emitAuto: true })
  const i18n = useI18n()
  const language = useStorage<Locale>('language', 'en')
  const theme = useStorage<ThemePreference>('theme', 'system')
  const colorScheme = useStorage<ColorSchemeId>('color-scheme', 'memoh')

  const applyColorScheme = (value: ColorSchemeId) => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.colorScheme = value
  }

  if (!isColorSchemeId(colorScheme.value)) {
    colorScheme.value = 'memoh'
  }

  watch(theme, (value) => {
    colorMode.value = value === 'system' ? 'auto' : value
  }, { immediate: true })

  watch(language, (value) => {
    i18n.locale.value = value
    // Reflect the active locale onto <html lang> so locale-scoped CSS can target
    // it — chiefly the CJK font-weight de-emphasis (see :lang(zh) in style.css):
    // Inter/MiSans render visually heavier for CJK at the same numeric weight, so
    // Chinese UI needs a lighter rung than English to read at the same density.
    if (typeof document !== 'undefined') {
      document.documentElement.lang = value
    }
  }, { immediate: true })

  watch(colorScheme, (value) => {
    if (!isColorSchemeId(value)) {
      colorScheme.value = 'memoh'
      return
    }
    applyColorScheme(value)
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

  const setTheme = (value: ThemePreference) => {
    // No view transition here: the segmented control already animates its own
    // thumb, and wrapping each toggle in startViewTransition freezes the page for
    // the transition's duration — which made rapid segment switching feel laggy
    // and swallowed hover. Theme flip is applied instantly instead.
    theme.value = value
    colorMode.value = value === 'system' ? 'auto' : value
  }

  const setColorScheme = (value: ColorSchemeId) => {
    withViewTransition(() => {
      colorScheme.value = value
      applyColorScheme(value)
    })
  }

  return {
    language,
    theme,
    colorScheme,
    setLanguage,
    setTheme,
    setColorScheme,
  }
})
