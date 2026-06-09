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
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { computed } from 'vue'
import {
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
import { useI18n } from 'vue-i18n'
import type { Locale } from '@/i18n'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import { colorSchemes, type ColorSchemeId, type ColorSchemeOption } from '@/constants/color-schemes'
import { useSettingsStore, type ThemePreference } from '@/store/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme } = settingsStore
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
</script>
