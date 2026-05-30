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
import { Moon, Sun } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type { Locale } from '@/i18n'
import ColorSchemeCard from '@/components/color-scheme-card/index.vue'
import { colorSchemes } from '@/constants/color-schemes'
import { useSettingsStore } from '@/store/settings'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme } = settingsStore
</script>
