<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <!-- Header Section -->
    <div class="space-y-0.5">
      <h4 class="text-xs font-medium text-foreground">
        {{ $t('bots.settings.blocks.global') }}
      </h4>
      <p class="text-[11px] text-muted-foreground">
        {{ $t('bots.settings.blocks.globalDescription') }}
      </p>
    </div>

    <!-- Configuration inputs with paragraph-like spacing -->
    <div class="space-y-3 pt-1">
      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.chatLanguage') }}</Label>
        <SearchableSelectPopover
          :model-value="form.language || 'auto'"
          :options="languageOptions"
          :placeholder="$t('bots.settings.chatLanguagePlaceholder')"
          :search-placeholder="$t('common.search')"
          :empty-text="$t('common.noData')"
          :show-group-headers="false"
          @update:model-value="(v) => form.language = (v === 'auto' ? '' : v)"
        >
          <template #trigger="{ open, displayLabel, selectedOption }">
            <Button
              variant="outline"
              role="combobox"
              :aria-expanded="open"
              :aria-label="$t('bots.settings.chatLanguage')"
              class="shadow-none border-border bg-transparent rounded-md text-xs h-8 w-full font-normal justify-between"
            >
              <span class="flex min-w-0 items-center gap-2 truncate">
                <Sparkles 
                  v-if="selectedOption?.value === 'auto' || !selectedOption" 
                  class="size-3.5 shrink-0 text-primary" 
                />
                <span
                  class="truncate"
                  :title="displayLabel || $t('bots.settings.chatLanguagePlaceholder')"
                >{{ displayLabel || $t('bots.settings.chatLanguagePlaceholder') }}</span>
              </span>
              <Search
                class="ml-2 size-3.5 shrink-0 text-muted-foreground"
              />
            </Button>
          </template>
          <template #option-icon="{ option }">
            <Sparkles 
              v-if="option.value === 'auto'" 
              class="size-3.5 shrink-0 text-primary" 
            />
          </template>
          <template #option-label="{ option }">
            <span
              class="truncate flex-1 text-left text-xs"
              :class="{ 'text-muted-foreground': option.value === 'auto' }"
              :title="option.label"
            >
              {{ option.label }}
            </span>
          </template>
        </SearchableSelectPopover>
      </div>

      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.timezone') }}</Label>
        <TimezoneSelect
          :model-value="form.timezone || emptyTimezoneValue"
          :placeholder="$t('bots.timezonePlaceholder')"
          allow-empty
          :empty-label="$t('bots.timezoneInherited')"
          class="text-xs h-8"
          @update:model-value="(val: string) => form.timezone = val === emptyTimezoneValue ? '' : val"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Label, Button } from '@memohai/ui'
import { Search, Sparkles } from 'lucide-vue-next'
import TimezoneSelect from '@/components/timezone-select/index.vue'
import { emptyTimezoneValue } from '@/utils/timezones'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { ISO639_LANGUAGES } from '@/utils/languages'
import type { SettingsSettings } from '@memohai/sdk'

const { t } = useI18n()

defineProps<{
  form: SettingsSettings
}>()

const languageOptions = computed<SearchableSelectOption[]>(() => {
  const autoOption: SearchableSelectOption = {
    value: 'auto',
    label: t('languages.auto') || t('bots.settings.chatLanguagePlaceholder'),
    keywords: ['auto', t('languages.auto'), t('bots.settings.chatLanguagePlaceholder')],
  }
  const codeOptions: SearchableSelectOption[] = ISO639_LANGUAGES.map((lang) => ({
    value: lang.code,
    label: `${lang.code} (${lang.name} / ${lang.nativeName})`,
    keywords: [lang.code, lang.name, lang.nativeName],
  }))
  return [autoOption, ...codeOptions]
})
</script>
