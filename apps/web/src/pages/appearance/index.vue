<template>
  <section class="max-w-7xl mx-auto p-4 pb-12">
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
          <div class="grid gap-3 sm:grid-cols-2">
            <button
              v-for="scheme in colorSchemes"
              :key="scheme.id"
              type="button"
              class="rounded-lg border bg-background p-3 text-left transition-colors hover:bg-accent"
              :class="colorScheme === scheme.id ? 'border-foreground' : 'border-border'"
              @click="setColorScheme(scheme.id)"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-xs font-medium">
                    {{ t(scheme.labelKey) }}
                  </p>
                  <p class="mt-1 text-[11px] text-muted-foreground">
                    {{ t(scheme.descriptionKey) }}
                  </p>
                </div>
                <Check
                  v-if="colorScheme === scheme.id"
                  class="size-4 shrink-0"
                />
              </div>
              <div class="mt-3 flex gap-1.5">
                <span
                  v-for="swatch in scheme.swatches"
                  :key="swatch"
                  class="size-5 rounded-full border border-border"
                  :style="{ backgroundColor: swatch }"
                />
              </div>
            </button>
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
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type { Locale } from '@/i18n'
import { colorSchemes } from '@/constants/color-schemes'
import { useSettingsStore } from '@/store/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme } = settingsStore
</script>
