<template>
  <div class="max-w-187 m-auto">
    <h6 class="mt-6 mb-2 flex items-center">
      <FontAwesomeIcon
        :icon="['fas', 'gear']"
        class="mr-2"
      />
      {{ $t('settings.display') }}
    </h6>
    <Separator />

    <div class="mt-4 space-y-4">
      <div class="flex items-center justify-between">
        <Label>{{ $t('settings.language') }}</Label>
        <Select
          :model-value="language"
          @update:model-value="(v) => v && setLanguage(v as Locale)"
        >
          <SelectTrigger class="w-40">
            <SelectValue :placeholder="$t('settings.languagePlaceholder')" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="zh">
                {{ $t('settings.langZh') }}
              </SelectItem>
              <SelectItem value="en">
                {{ $t('settings.langEn') }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <Separator />

      <div class="flex items-center justify-between">
        <Label>{{ $t('settings.theme') }}</Label>
        <Select
          :model-value="theme"
          @update:model-value="(v) => v && setTheme(v as 'light' | 'dark')"
        >
          <SelectTrigger class="w-40">
            <SelectValue :placeholder="$t('settings.themePlaceholder')" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="light">
                {{ $t('settings.themeLight') }}
              </SelectItem>
              <SelectItem value="dark">
                {{ $t('settings.themeDark') }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
    </div>

    <div class="mt-6">
      <Popover>
        <template #default="{ close }">
          <PopoverTrigger as-child>
            <Button variant="outline">
              {{ $t('auth.logout') }}
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-80">
            <p class="mb-4">
              {{ $t('auth.logoutConfirm') }}
            </p>
            <div class="flex justify-end gap-3">
              <Button
                variant="outline"
                @click="close"
              >
                {{ $t('common.cancel') }}
              </Button>
              <Button @click="exit(); close()">
                {{ $t('common.confirm') }}
              </Button>
            </div>
          </PopoverContent>
        </template>
      </Popover>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Select,
  SelectTrigger,
  SelectContent,
  SelectValue,
  SelectGroup,
  SelectItem,
  Label,
  Separator,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@memoh/ui'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useUserStore } from '@/store/user'
import { useSettingsStore } from '@/store/settings'
import type { Locale } from '@/i18n'

const router = useRouter()
const settingsStore = useSettingsStore()
const { language, theme } = storeToRefs(settingsStore)
const { setLanguage, setTheme } = settingsStore

const { exitLogin } = useUserStore()
const exit = () => {
  exitLogin()
  router.replace({ name: 'Login' })
}
</script>
