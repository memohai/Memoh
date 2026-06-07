<template>
  <section class="max-w-7xl mx-auto px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12">
    <div class="max-w-3xl mx-auto space-y-6">
      <div>
        <h1 class="text-lg font-semibold">
          {{ t('settings.appearance.title') }}
        </h1>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ t('settings.appearance.description') }}
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle class="text-sm">
            {{ t('settings.appearance.interface') }}
          </CardTitle>
          <CardDescription class="text-xs">
            {{ t('settings.appearance.interfaceDescription') }}
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-5">
          <div class="grid gap-2">
            <Label>{{ t('settings.language') }}</Label>
            <Select
              :model-value="language"
              @update:model-value="(value) => value && setLanguage(value as Locale)"
            >
              <SelectTrigger class="w-full sm:w-56">
                <SelectValue :placeholder="t('settings.languagePlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="en">
                  {{ t('settings.langEn') }}
                </SelectItem>
                <SelectItem value="zh">
                  {{ t('settings.langZh') }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div class="grid gap-2">
            <Label>{{ t('settings.theme') }}</Label>
            <div class="grid grid-cols-2 gap-2 sm:flex">
              <Button
                type="button"
                variant="outline"
                class="justify-start gap-2"
                :class="theme === 'light' ? 'border-foreground bg-accent text-foreground' : ''"
                @click="setTheme('light')"
              >
                <Sun class="size-4" />
                {{ t('settings.themeLight') }}
              </Button>
              <Button
                type="button"
                variant="outline"
                class="justify-start gap-2"
                :class="theme === 'dark' ? 'border-foreground bg-accent text-foreground' : ''"
                @click="setTheme('dark')"
              >
                <Moon class="size-4" />
                {{ t('settings.themeDark') }}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- Font Family -->
      <Card>
        <CardHeader>
          <CardTitle class="text-sm">
            {{ t('settings.appearance.fontFamily') }}
          </CardTitle>
          <CardDescription class="text-xs">
            {{ t('settings.appearance.fontFamilyDescription') }}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <button
              v-for="font in fonts"
              :key="font.id"
              type="button"
              class="rounded-lg border bg-background p-2 text-left transition-colors"
              :class="fontFamily === font.id ? 'border-foreground' : 'border-border hover:border-muted-foreground/50'"
              @click="setFontFamily(font.id)"
            >
              <!-- Preview area -->
              <div class="rounded-md border border-border bg-muted px-3 py-2.5 select-none">
                <div
                  class="text-[22px] leading-none font-semibold tracking-tight text-foreground"
                  :style="{ fontFamily: font.family }"
                >
                  Ag
                </div>
                <div
                  class="mt-1.5 text-[11px] leading-none text-muted-foreground"
                  :style="{ fontFamily: font.family }"
                >
                  智能对话 · Interface
                </div>
              </div>
              <!-- Card footer -->
              <div class="mt-2 flex items-center justify-between gap-2 px-0.5">
                <div class="min-w-0">
                  <p class="text-xs font-medium truncate">
                    {{ t(`settings.appearance.fonts.${font.id}`) }}
                  </p>
                  <p class="mt-0.5 text-[10px] text-muted-foreground">
                    {{ font.cjk ? t('settings.appearance.fontCjkUnified') : t('settings.appearance.fontCjkSystem') }}
                  </p>
                </div>
                <Check
                  v-if="fontFamily === font.id"
                  class="size-3.5 shrink-0 text-foreground"
                />
              </div>
            </button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="text-sm">
            {{ t('settings.appearance.colorScheme') }}
          </CardTitle>
          <CardDescription class="text-xs">
            {{ t('settings.appearance.colorSchemeDescription') }}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <ColorSchemeCard
              v-for="scheme in colorSchemes"
              :key="scheme.id"
              :scheme="scheme"
              :selected="colorScheme === scheme.id"
              show-description
              @select="setColorScheme(scheme.id)"
            />
          </div>
        </CardContent>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memohai/ui'
import { Check, Moon, Sun } from 'lucide-vue-next'
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type { Locale } from '@/i18n'
import ColorSchemeCard from '@/components/color-scheme-card/index.vue'
import { colorSchemes } from '@/constants/color-schemes'
import { fonts } from '@/constants/fonts'
import { loadFontStylesheet } from '@/store/settings'
import { useSettingsStore } from '@/store/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme, fontFamily } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme, setFontFamily } = settingsStore

// Load all font stylesheets on mount so preview cards render correctly
onMounted(() => {
  // Preconnect hints for Google Fonts
  for (const origin of ['https://fonts.googleapis.com', 'https://fonts.gstatic.com']) {
    if (!document.querySelector(`link[rel="preconnect"][href="${origin}"]`)) {
      const link = document.createElement('link')
      link.rel = 'preconnect'
      link.href = origin
      if (origin.includes('gstatic')) link.crossOrigin = 'anonymous'
      document.head.appendChild(link)
    }
  }
  for (const font of fonts) {
    if (font.href) loadFontStylesheet(font.href)
  }
})
</script>
