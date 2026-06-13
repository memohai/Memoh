import type { Locale } from '@/i18n'
import { computed, watch } from 'vue'
import { defineStore } from 'pinia'
import { useColorMode, useStorage } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { isColorSchemeId, type ColorSchemeId } from '@/constants/color-schemes'
import {
  applyTypographyVariables,
  cssFontFamilyDeclaration,
  DEFAULT_CODE_FONT_FAMILY,
  DEFAULT_CODE_FONT_SIZE_PX,
  DEFAULT_UI_FONT_FAMILY,
  DEFAULT_UI_FONT_SIZE_PX,
  normalizeCodeFontSizePx,
  normalizeStoredFontFamilyInput,
  normalizeUiFontSizePx,
} from './typography'

export type ThemePreference = 'light' | 'dark' | 'system'

export interface Settings {
  language: Locale;
  theme: ThemePreference;
  colorScheme: ColorSchemeId;
  uiFontFamily: string;
  codeFontFamily: string;
  uiFontSizePx: number;
  codeFontSizePx: number;
}

export const useSettingsStore = defineStore('settings', () => {
  const colorMode = useColorMode({ emitAuto: true })
  const i18n = useI18n()
  const defaultUiFontFamily = computed(() => DEFAULT_UI_FONT_FAMILY)
  const defaultCodeFontFamily = computed(() => DEFAULT_CODE_FONT_FAMILY)
  const language = useStorage<Locale>('language', 'en')
  const theme = useStorage<ThemePreference>('theme', 'system')
  const colorScheme = useStorage<ColorSchemeId>('color-scheme', 'memoh')
  const uiFontFamily = useStorage<string>('ui-font-family', '')
  const codeFontFamily = useStorage<string>('code-font-family', '')
  const uiFontSizePx = useStorage<number>('ui-font-size-px', DEFAULT_UI_FONT_SIZE_PX)
  const codeFontSizePx = useStorage<number>('code-font-size-px', DEFAULT_CODE_FONT_SIZE_PX)
  const uiFontStack = computed(() => cssFontFamilyDeclaration(uiFontFamily.value, DEFAULT_UI_FONT_FAMILY))
  const codeFontStack = computed(() => cssFontFamilyDeclaration(codeFontFamily.value, DEFAULT_CODE_FONT_FAMILY))

  const applyColorScheme = (value: ColorSchemeId) => {
    if (typeof document === 'undefined') return
    document.documentElement.dataset.colorScheme = value
  }

  const normalizeTypographySettings = () => {
    uiFontFamily.value = normalizeStoredFontFamilyInput(uiFontFamily.value, DEFAULT_UI_FONT_FAMILY)
    codeFontFamily.value = normalizeStoredFontFamilyInput(codeFontFamily.value, DEFAULT_CODE_FONT_FAMILY)
    const normalizedUiSize = normalizeUiFontSizePx(uiFontSizePx.value)
    const normalizedCodeSize = normalizeCodeFontSizePx(codeFontSizePx.value)
    if (uiFontSizePx.value !== normalizedUiSize) uiFontSizePx.value = normalizedUiSize
    if (codeFontSizePx.value !== normalizedCodeSize) codeFontSizePx.value = normalizedCodeSize
  }

  const applyTypography = () => {
    applyTypographyVariables({
      uiFontFamily: uiFontFamily.value,
      codeFontFamily: codeFontFamily.value,
      uiFontSizePx: uiFontSizePx.value,
      codeFontSizePx: codeFontSizePx.value,
    })
  }

  if (!isColorSchemeId(colorScheme.value)) {
    colorScheme.value = 'memoh'
  }
  normalizeTypographySettings()

  watch(theme, (value) => {
    colorMode.value = value === 'system' ? 'auto' : value
  }, { immediate: true })

  watch(language, (value) => {
    i18n.locale.value = value
    // Reflect the active locale onto <html lang> so locale-scoped CSS can target
    // it — chiefly the CJK font-weight de-emphasis (see :lang(zh) in style.css):
    // CJK glyphs render visually heavier than Latin at the same numeric weight, so
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

  watch([uiFontFamily, codeFontFamily, uiFontSizePx, codeFontSizePx], () => {
    normalizeTypographySettings()
    applyTypography()
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

  const setUiFontFamily = (value: string) => {
    uiFontFamily.value = normalizeStoredFontFamilyInput(value, DEFAULT_UI_FONT_FAMILY)
  }

  const setCodeFontFamily = (value: string) => {
    codeFontFamily.value = normalizeStoredFontFamilyInput(value, DEFAULT_CODE_FONT_FAMILY)
  }

  const setUiFontSizePx = (value: string | number) => {
    uiFontSizePx.value = normalizeUiFontSizePx(value)
  }

  const setCodeFontSizePx = (value: string | number) => {
    codeFontSizePx.value = normalizeCodeFontSizePx(value)
  }

  return {
    language,
    theme,
    colorScheme,
    uiFontFamily,
    codeFontFamily,
    uiFontSizePx,
    codeFontSizePx,
    defaultUiFontFamily,
    defaultCodeFontFamily,
    uiFontStack,
    codeFontStack,
    setLanguage,
    setTheme,
    setColorScheme,
    setUiFontFamily,
    setCodeFontFamily,
    setUiFontSizePx,
    setCodeFontSizePx,
  }
})
