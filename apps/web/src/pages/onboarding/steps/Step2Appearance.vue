<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '@/store/settings'
import { Button, ButtonGroup } from '@memohai/ui'
import { colorSchemes } from '@/constants/color-schemes'

const { t } = useI18n()
const settings = useSettingsStore()
</script>

<template>
  <div class="text-center">
    <h2 class="text-xl font-semibold mb-6">
      {{ t('onboarding.appearance.title') }}
    </h2>

    <div class="space-y-6">
      <div>
        <label class="text-sm font-medium mb-2 block">
          {{ t('onboarding.appearance.language') }}
        </label>
        <ButtonGroup>
          <Button
            :variant="settings.language === 'zh' ? 'default' : 'outline'"
            size="sm"
            @click="settings.setLanguage('zh')"
          >
            中文
          </Button>
          <Button
            :variant="settings.language === 'en' ? 'default' : 'outline'"
            size="sm"
            @click="settings.setLanguage('en')"
          >
            English
          </Button>
        </ButtonGroup>
      </div>

      <div>
        <label class="text-sm font-medium mb-2 block">
          {{ t('onboarding.appearance.theme') }}
        </label>
        <ButtonGroup>
          <Button
            :variant="settings.theme === 'light' ? 'default' : 'outline'"
            size="sm"
            @click="settings.setTheme('light')"
          >
            {{ t('onboarding.appearance.light') }}
          </Button>
          <Button
            :variant="settings.theme === 'dark' ? 'default' : 'outline'"
            size="sm"
            @click="settings.setTheme('dark')"
          >
            {{ t('onboarding.appearance.dark') }}
          </Button>
        </ButtonGroup>
      </div>

      <div>
        <label class="text-sm font-medium mb-2 block">
          {{ t('onboarding.appearance.colorScheme') }}
        </label>
        <ButtonGroup>
          <Button
            v-for="scheme in colorSchemes"
            :key="scheme.id"
            :variant="settings.colorScheme === scheme.id ? 'default' : 'outline'"
            size="sm"
            @click="settings.setColorScheme(scheme.id)"
          >
            {{ t(scheme.labelKey) }}
          </Button>
        </ButtonGroup>
      </div>
    </div>
  </div>
</template>
