import type { Locale } from '@/i18n'

export function detectLocale(): Locale {
  if (typeof navigator === 'undefined') return 'en'
  const lang = navigator.language || ''
  if (lang.toLowerCase().startsWith('zh')) return 'zh'
  return 'en'
}
