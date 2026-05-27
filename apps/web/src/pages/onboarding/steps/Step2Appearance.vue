<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/store/settings'
import { useOnboarding } from '@/composables/useOnboarding'
import { Button, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@memohai/ui'
import { Check, Moon, Sun } from 'lucide-vue-next'
import { colorSchemes } from '@/constants/color-schemes'
import type { Locale } from '@/i18n'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme } = settingsStore
const { nextStep, prevStep } = useOnboarding()

const visible = ref(false)
const exiting = ref(false)

onMounted(() => {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      visible.value = true
    })
  })
})

function go(action: () => void) {
  exiting.value = true
  setTimeout(action, 175)
}
</script>

<template>
  <div
    class="transition-all duration-[175ms] ease-out"
    :class="exiting ? 'scale-[0.88] opacity-0' : 'scale-100 opacity-100'"
  >
    <div class="text-left">
      <h2
        class="text-3xl font-semibold mb-8 transition-all duration-[350ms] ease-out"
        :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
      >
        {{ t('onboarding.appearance.title') }}
      </h2>

      <div class="space-y-6">
        <div
          class="transition-all duration-[350ms] ease-out delay-[80ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <Label class="mb-2 block text-sm font-medium">
            {{ t('settings.language') }}
          </Label>
          <Select
            :model-value="language"
            @update:model-value="(value) => value && setLanguage(value as Locale)"
          >
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="t('settings.languagePlaceholder')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="zh">
                {{ t('settings.langZh') }}
              </SelectItem>
              <SelectItem value="en">
                {{ t('settings.langEn') }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div
          class="transition-all duration-[350ms] ease-out delay-[160ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <Label class="mb-2 block text-sm font-medium">
            {{ t('settings.theme') }}
          </Label>
          <div class="flex gap-2">
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

        <div
          class="transition-all duration-[350ms] ease-out delay-[240ms]"
          :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
        >
          <Label class="mb-2 block text-sm font-medium">
            {{ t('settings.appearance.colorScheme') }}
          </Label>
          <div class="grid grid-cols-2 gap-3">
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
        </div>
      </div>

      <div
        class="mt-12 flex items-center justify-end gap-3 transition-all duration-[350ms] ease-out delay-[320ms]"
        :class="visible ? 'opacity-100 translate-y-0' : 'opacity-0 -translate-y-3'"
      >
        <button
          class="inline-flex h-[42px] items-center justify-center rounded-lg px-4 text-sm font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="go(prevStep)"
        >
          {{ t('onboarding.prev') }}
        </button>
        <button
          class="inline-flex h-[42px] w-[180px] items-center justify-center rounded-lg bg-primary px-5 font-normal text-primary-foreground shadow-none transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          @click="go(nextStep)"
        >
          {{ t('onboarding.next') }}
        </button>
      </div>
    </div>
  </div>
</template>
