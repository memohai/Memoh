import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { computed } from 'vue'
import { detectLocale } from '@/utils/detect-locale'

export type Locale = 'en' | 'zh'

function getInitialLocale(): Locale {
  const stored = localStorage.getItem('language')
  if (stored === 'en' || stored === 'zh') return stored
  return detectLocale()
}

const i18n = createI18n<typeof en | typeof zh, Locale>({
  locale: getInitialLocale(),
  legacy: false,
  fallbackLocale: 'en',
  messages: {
    en,
    zh
  }
})

export default i18n

const t = i18n.global.t

export const i18nRef = (arg:string) => {
  return computed(() => {
    return t(arg)
  })
}
