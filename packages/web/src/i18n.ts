import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import { computed } from 'vue'

export type Locale = 'en' | 'zh'

const i18n = createI18n<typeof en | typeof zh, Locale>({
  locale: 'zh',
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
