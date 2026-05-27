import type { Locale } from '@/i18n'
import { watch } from 'vue'
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
  const theme = useStorage<'light' | 'dark'>('theme',
    typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  const colorScheme = useStorage<ColorSchemeId>('color-scheme', 'memoh')

  const applyColorScheme = (value: ColorSchemeId) => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.colorScheme = value
  }

  if (!isColorSchemeId(colorScheme.value)) {
    colorScheme.value = 'memoh'
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

  const setLanguage = (value: Locale) => {
    language.value = value
  }

  const setTheme = (value: 'light' | 'dark') => {
    theme.value = value
  }

  const setColorScheme = (value: ColorSchemeId) => {
    colorScheme.value = value
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
