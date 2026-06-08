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
      </SettingsSection>

      <SettingsSection :title="t('settings.appearance.colorScheme')">
        <div class="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
          <ColorSchemeCard
            v-for="scheme in colorSchemes"
            :key="scheme.id"
            :scheme="scheme"
            :selected="colorScheme === scheme.id"
            show-description
            @select="setColorScheme(scheme.id)"
          />
        </div>
      </SettingsSection>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import {
  SegmentedControl,
  type SegmentedItem,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memohai/ui'
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type { Locale } from '@/i18n'
import ColorSchemeCard from '@/components/color-scheme-card/index.vue'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import { colorSchemes } from '@/constants/color-schemes'
import { useSettingsStore, type ThemePreference } from '@/store/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme } = settingsStore

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
