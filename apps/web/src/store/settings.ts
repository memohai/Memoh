import type { Locale } from '@/i18n'
import { defineStore } from 'pinia'
import { useColorMode, useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { isColorSchemeId, type ColorSchemeId } from '@/constants/color-schemes'

export interface Settings {
  language: Locale;
  theme: 'light' | 'dark';
  colorScheme: ColorSchemeId;
}

export const useSettingsStore = defineStore('settings', () => {
  const colorMode = useColorMode()
  const i18n = useI18n()
  const language = useStorage<Locale>('language', 'en')
  const theme = useStorage<'light' | 'dark'>('theme', 'light')
  const colorScheme = useStorage<ColorSchemeId>('color-scheme', 'memoh')

  const applyColorScheme = (value: ColorSchemeId) => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.colorScheme = value
  }

  colorMode.value = theme.value
  i18n.locale.value = language.value
  if (!isColorSchemeId(colorScheme.value)) {
    colorScheme.value = 'memoh'
  }
  applyColorScheme(colorScheme.value)

  const setLanguage = (value: Locale) => {
    language.value = value
    i18n.locale.value = value
  }

  const setTheme = (value: 'light' | 'dark') => {
    theme.value = value
    colorMode.value = value
  }

  const setColorScheme = (value: ColorSchemeId) => {
    colorScheme.value = value
    applyColorScheme(value)
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
