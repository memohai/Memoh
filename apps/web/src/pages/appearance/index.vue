<template>
  <section class="mx-auto max-w-3xl px-6 pt-10 pb-12">
    <h1 class="mb-6 px-2 text-lg font-semibold">
      {{ t('settings.appearance.title') }}
    </h1>

    <div class="space-y-8">
      <SettingsSection :title="t('settings.appearance.interface')">
        <SettingsRow :label="t('settings.language')">
          <Select
            :model-value="language"
            @update:model-value="(value) => value && setLanguage(value as Locale)"
          >
            <SelectTrigger size="sm">
              <SelectValue :placeholder="t('settings.languagePlaceholder')" />
            </SelectTrigger>
            <SelectContent
              align="end"
              :align-offset="0"
            >
              <SelectItem value="en">
                {{ t('settings.langEn') }}
              </SelectItem>
              <SelectItem value="zh">
                {{ t('settings.langZh') }}
              </SelectItem>
              <SelectItem value="ja">
                {{ t('settings.langJa') }}
              </SelectItem>
            </SelectContent>
          </Select>
        </SettingsRow>

        <SettingsRow :label="t('settings.theme')">
          <SegmentedControl
            :model-value="theme"
            :items="themeItems"
            :aria-label="t('settings.theme')"
            @update:model-value="(value) => setTheme(value as ThemePreference)"
          >
            <template #item="{ item }">
              <component
                :is="themeIcons[item.value]"
                class="size-4"
              />
            </template>
          </SegmentedControl>
        </SettingsRow>

        <SettingsRow :label="t('settings.appearance.colorScheme')">
          <Select
            :model-value="colorScheme"
            @update:model-value="(value) => value && setColorScheme(value as ColorSchemeId)"
          >
            <SelectTrigger
              size="sm"
              class="min-w-36"
            >
              <span class="flex min-w-0 items-center gap-1.5">
                <span
                  class="flex size-5 shrink-0 items-center justify-center rounded-sm border border-border text-[10px] font-semibold"
                  :style="{ backgroundColor: previewSwatches(currentColorScheme)[0] }"
                >
                  <span :style="{ color: previewSwatches(currentColorScheme)[4] }">Aa</span>
                </span>
                <span class="truncate">
                  {{ t(currentColorScheme.labelKey) }}
                </span>
              </span>
            </SelectTrigger>
            <SelectContent
              align="end"
              :align-offset="0"
            >
              <SelectItem
                v-for="scheme in colorSchemes"
                :key="scheme.id"
                :value="scheme.id"
              >
                <span class="flex min-w-0 items-center gap-1.5">
                  <span
                    class="flex size-6 shrink-0 items-center justify-center rounded-sm border border-border text-[11px] font-semibold"
                    :style="{ backgroundColor: previewSwatches(scheme)[0] }"
                  >
                    <span :style="{ color: previewSwatches(scheme)[4] }">Aa</span>
                  </span>
                  <span class="truncate">
                    {{ t(scheme.labelKey) }}
                  </span>
                </span>
              </SelectItem>
            </SelectContent>
          </Select>
        </SettingsRow>
      </SettingsSection>

      <SettingsSection :title="t('settings.appearance.typography')">
        <SettingsRow
          :label="t('settings.appearance.uiFontSize')"
          :description="t('settings.appearance.uiFontSizeDescription')"
        >
          <Input
            id="ui-font-size"
            type="number"
            min="12"
            max="20"
            step="1"
            :model-value="uiFontSizeDraft"
            :placeholder="String(DEFAULT_UI_FONT_SIZE_PX)"
            class="h-8 w-20 tabular-nums"
            @update:model-value="(value) => updateUiFontSizeDraft(value)"
            @change="commitUiFontSizeDraft"
            @blur="commitUiFontSizeDraft"
            @keydown.enter="commitUiFontSizeDraft"
          />
        </SettingsRow>

        <SettingsRow
          :label="t('settings.appearance.codeFontSize')"
          :description="t('settings.appearance.codeFontSizeDescription')"
        >
          <Input
            id="code-font-size"
            type="number"
            min="11"
            max="20"
            step="1"
            :model-value="codeFontSizeDraft"
            :placeholder="String(DEFAULT_CODE_FONT_SIZE_PX)"
            class="h-8 w-20 tabular-nums"
            @update:model-value="(value) => updateCodeFontSizeDraft(value)"
            @change="commitCodeFontSizeDraft"
            @blur="commitCodeFontSizeDraft"
            @keydown.enter="commitCodeFontSizeDraft"
          />
        </SettingsRow>

        <SettingsRow
          :label="t('settings.appearance.uiFontFamily')"
          :description="t('settings.appearance.uiFontFamilyDescription')"
        >
          <Input
            id="ui-font-family"
            :model-value="uiFontFamilyDraft"
            :placeholder="defaultUiFontFamily"
            class="h-8 w-48 font-mono text-xs"
            @update:model-value="(value) => updateUiFontFamilyDraft(value)"
            @change="commitUiFontFamilyDraft"
            @blur="commitUiFontFamilyDraft"
            @keydown.enter="commitUiFontFamilyDraft"
          />
        </SettingsRow>

        <div class="mx-4 border-b border-border py-3 last:border-b-0">
          <div class="flex min-h-[2.25rem] items-center justify-between gap-4">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ t('settings.appearance.codeFontFamily') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ t('settings.appearance.codeFontFamilyDescription') }}
              </p>
            </div>
            <div class="shrink-0">
              <Input
                id="code-font-family"
                :model-value="codeFontFamilyDraft"
                :placeholder="defaultCodeFontFamily"
                class="h-8 w-48 font-mono text-xs"
                @update:model-value="(value) => updateCodeFontFamilyDraft(value)"
                @change="commitCodeFontFamilyDraft"
                @blur="commitCodeFontFamilyDraft"
                @keydown.enter="commitCodeFontFamilyDraft"
              />
            </div>
          </div>
          <div
            class="typography-code-preview pointer-events-none mt-3 border-l-4 border-l-warning-border pl-3"
            :style="codeFontPreviewStyle"
            inert
          >
            <!-- eslint-disable vue/no-v-html -->
            <div
              class="overflow-x-auto"
              v-html="codeFontPreviewHtml"
            />
            <!-- eslint-enable vue/no-v-html -->
          </div>
        </div>
      </SettingsSection>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

import {
  Input,
  SegmentedControl,
  type SegmentedItem,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memohai/ui'
import { useDark } from '@vueuse/core'
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useShikiHighlighter } from '@/composables/useShikiHighlighter'
import type { Locale } from '@/i18n'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import { colorSchemes, type ColorSchemeId, type ColorSchemeOption } from '@/constants/color-schemes'
import { useSettingsStore, type ThemePreference } from '@/store/settings'
import { cssFontFamilyDeclaration, DEFAULT_CODE_FONT_FAMILY, DEFAULT_CODE_FONT_SIZE_PX, DEFAULT_UI_FONT_SIZE_PX, normalizeCodeFontSizePx } from '@/store/settings/typography'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme, uiFontFamily, codeFontFamily, uiFontSizePx, codeFontSizePx, defaultUiFontFamily, defaultCodeFontFamily } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme, setUiFontFamily, setCodeFontFamily, setUiFontSizePx, setCodeFontSizePx } = settingsStore
const isDark = useDark()

const currentColorScheme = computed(() =>
  colorSchemes.find(scheme => scheme.id === colorScheme.value) ?? colorSchemes[0],
)

function previewSwatches(scheme: ColorSchemeOption) {
  return isDark.value ? scheme.darkSwatches : scheme.swatches
}

const themeItems: SegmentedItem<ThemePreference>[] = [
  { value: 'system' },
  { value: 'light' },
  { value: 'dark' },
]
const themeIcons: Record<ThemePreference, Component> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
}

const uiFontSizeDraft = ref(String(uiFontSizePx.value))
const codeFontSizeDraft = ref(String(codeFontSizePx.value))
const uiFontFamilyDraft = ref(uiFontFamily.value)
const codeFontFamilyDraft = ref(codeFontFamily.value)
const codeFontPreview = useShikiHighlighter()
const codeFontPreviewCode = `function greet(name: string): string {
  return \`Hello, \${name}\`
}`
const codeFontPreviewHtml = computed(() => codeFontPreview.html.value || `<pre><code>${codeFontPreviewCode}</code></pre>`)
const codeFontPreviewStyle = computed(() => ({
  '--typography-code-preview-font-family': cssFontFamilyDeclaration(codeFontFamilyDraft.value, DEFAULT_CODE_FONT_FAMILY),
  '--typography-code-preview-font-size': `${normalizeCodeFontSizePx(codeFontSizeDraft.value)}px`,
}))

function renderCodeFontPreview() {
  void codeFontPreview.highlightLanguage(codeFontPreviewCode, 'typescript', {
    theme: isDark.value ? 'github-dark' : 'github-light',
    transparentPre: true,
  })
}

onMounted(() => {
  renderCodeFontPreview()
})

watch(uiFontSizePx, (value) => { uiFontSizeDraft.value = String(value) })
watch(codeFontSizePx, (value) => { codeFontSizeDraft.value = String(value) })
watch(uiFontFamily, (value) => { uiFontFamilyDraft.value = value })
watch(codeFontFamily, (value) => { codeFontFamilyDraft.value = value })
watch(isDark, () => { renderCodeFontPreview() })

function updateUiFontSizeDraft(value: string | number) { uiFontSizeDraft.value = String(value) }
function updateCodeFontSizeDraft(value: string | number) { codeFontSizeDraft.value = String(value) }
function updateUiFontFamilyDraft(value: string | number) { uiFontFamilyDraft.value = String(value) }
function updateCodeFontFamilyDraft(value: string | number) { codeFontFamilyDraft.value = String(value) }

function commitUiFontSizeDraft() {
  const draft = uiFontSizeDraft.value.trim()
  if (draft === '' || !Number.isFinite(Number(draft))) {
    uiFontSizeDraft.value = String(uiFontSizePx.value)
    return
  }
  setUiFontSizePx(draft)
  uiFontSizeDraft.value = String(uiFontSizePx.value)
}

function commitCodeFontSizeDraft() {
  const draft = codeFontSizeDraft.value.trim()
  if (draft === '' || !Number.isFinite(Number(draft))) {
    codeFontSizeDraft.value = String(codeFontSizePx.value)
    return
  }
  setCodeFontSizePx(draft)
  codeFontSizeDraft.value = String(codeFontSizePx.value)
}

function commitUiFontFamilyDraft() {
  setUiFontFamily(uiFontFamilyDraft.value)
  uiFontFamilyDraft.value = uiFontFamily.value
}

function commitCodeFontFamilyDraft() {
  setCodeFontFamily(codeFontFamilyDraft.value)
  codeFontFamilyDraft.value = codeFontFamily.value
}
</script>
