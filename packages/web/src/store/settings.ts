import type { Locale } from '@/i18n'
import { defineStore } from 'pinia'
import { useColorMode, useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'

export interface Settings {
  language: Locale;
  theme: 'light' | 'dark';
}

export const useSettingsStore = defineStore('settings', () => {
  const colorMode = useColorMode()
  const i18n = useI18n()
  const language = useStorage<Locale>('language', 'zh')
  const theme = useStorage<'light' | 'dark'>('theme', 'light')

  // 立即同步持久化的设置到运行时状态
  colorMode.value = theme.value
  i18n.locale.value = language.value

  const setLanguage = (value: Locale) => {
    language.value = value
    i18n.locale.value = value
  }

  const setTheme = (value: 'light' | 'dark') => {
    theme.value = value
    colorMode.value = value
  }

  return {
    language,
    theme,
    setLanguage,
    setTheme,
  }
})
