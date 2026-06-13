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

      <Card>
        <CardHeader>
          <CardTitle class="text-sm">
            {{ t('settings.appearance.typography') }}
          </CardTitle>
        </CardHeader>
        <CardContent class="space-y-5">
          <div class="grid gap-2">
            <div>
              <Label for="ui-font-size">{{ t('settings.appearance.uiFontSize') }}</Label>
              <p
                id="ui-font-size-description"
                class="mt-1 text-xs text-muted-foreground"
              >
                {{ t('settings.appearance.uiFontSizeDescription') }}
              </p>
            </div>
            <Input
              id="ui-font-size"
              type="number"
              min="12"
              max="20"
              step="1"
              :model-value="uiFontSizeDraft"
              :placeholder="String(DEFAULT_UI_FONT_SIZE_PX)"
              :class="typographyNumberInputClass"
              aria-describedby="ui-font-size-description"
              @update:model-value="(value) => updateUiFontSizeDraft(value)"
              @change="commitUiFontSizeDraft"
              @blur="commitUiFontSizeDraft"
              @keydown.enter="commitUiFontSizeDraft"
            />
          </div>

          <div class="grid gap-2">
            <div>
              <Label for="code-font-size">{{ t('settings.appearance.codeFontSize') }}</Label>
              <p
                id="code-font-size-description"
                class="mt-1 text-xs text-muted-foreground"
              >
                {{ t('settings.appearance.codeFontSizeDescription') }}
              </p>
            </div>
            <Input
              id="code-font-size"
              type="number"
              min="11"
              max="20"
              step="1"
              :model-value="codeFontSizeDraft"
              :placeholder="String(DEFAULT_CODE_FONT_SIZE_PX)"
              :class="typographyNumberInputClass"
              aria-describedby="code-font-size-description"
              @update:model-value="(value) => updateCodeFontSizeDraft(value)"
              @change="commitCodeFontSizeDraft"
              @blur="commitCodeFontSizeDraft"
              @keydown.enter="commitCodeFontSizeDraft"
            />
          </div>

          <div class="grid gap-2">
            <div>
              <Label for="ui-font-family">{{ t('settings.appearance.uiFontFamily') }}</Label>
              <p
                id="ui-font-family-description"
                class="mt-1 text-xs text-muted-foreground"
              >
                {{ t('settings.appearance.uiFontFamilyDescription') }}
              </p>
            </div>
            <Input
              id="ui-font-family"
              :model-value="uiFontFamilyDraft"
              :placeholder="defaultUiFontFamily"
              :class="typographyFamilyInputClass"
              aria-describedby="ui-font-family-description"
              @update:model-value="(value) => updateUiFontFamilyDraft(value)"
              @change="commitUiFontFamilyDraft"
              @blur="commitUiFontFamilyDraft"
              @keydown.enter="commitUiFontFamilyDraft"
            />
          </div>

          <div class="grid gap-2">
            <div>
              <Label for="code-font-family">{{ t('settings.appearance.codeFontFamily') }}</Label>
              <p
                id="code-font-family-description"
                class="mt-1 text-xs text-muted-foreground"
              >
                {{ t('settings.appearance.codeFontFamilyDescription') }}
              </p>
            </div>
            <Input
              id="code-font-family"
              :model-value="codeFontFamilyDraft"
              :placeholder="defaultCodeFontFamily"
              :class="typographyFamilyInputClass"
              aria-describedby="code-font-family-description"
              @update:model-value="(value) => updateCodeFontFamilyDraft(value)"
              @change="commitCodeFontFamilyDraft"
              @blur="commitCodeFontFamilyDraft"
              @keydown.enter="commitCodeFontFamilyDraft"
            />
            <div
              class="typography-code-preview pointer-events-none mt-1 border-l-4 border-l-warning-border pl-2"
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
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@memohai/ui'
import { Moon, Sun } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useShikiHighlighter } from '@/composables/useShikiHighlighter'
import type { Locale } from '@/i18n'
import ColorSchemeCard from '@/components/color-scheme-card/index.vue'
import { colorSchemes } from '@/constants/color-schemes'
import { useSettingsStore } from '@/store/settings'
import { cssFontFamilyDeclaration, DEFAULT_CODE_FONT_FAMILY, DEFAULT_CODE_FONT_SIZE_PX, DEFAULT_UI_FONT_SIZE_PX, normalizeCodeFontSizePx } from '@/store/settings/typography'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const { language, theme, colorScheme, uiFontFamily, codeFontFamily, uiFontSizePx, codeFontSizePx, defaultUiFontFamily, defaultCodeFontFamily } = storeToRefs(settingsStore)
const { setLanguage, setTheme, setColorScheme, setUiFontFamily, setCodeFontFamily, setUiFontSizePx, setCodeFontSizePx } = settingsStore

const typographyNumberInputClass = 'h-8 w-20 tabular-nums [&:not(:focus)]:[-moz-appearance:textfield] [&:not(:focus)::-webkit-inner-spin-button]:m-0 [&:not(:focus)::-webkit-inner-spin-button]:appearance-none [&:not(:focus)::-webkit-outer-spin-button]:m-0 [&:not(:focus)::-webkit-outer-spin-button]:appearance-none'
const typographyFamilyInputClass = 'h-8 font-mono text-xs'

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
    theme: settingsStore.theme === 'dark' ? 'github-dark' : 'github-light',
    transparentPre: true,
  })
}

onMounted(() => {
  renderCodeFontPreview()
})

watch(() => settingsStore.theme, () => {
  renderCodeFontPreview()
})

watch(uiFontSizePx, (value) => {
  uiFontSizeDraft.value = String(value)
})

watch(codeFontSizePx, (value) => {
  codeFontSizeDraft.value = String(value)
})

watch(uiFontFamily, (value) => {
  uiFontFamilyDraft.value = value
})

watch(codeFontFamily, (value) => {
  codeFontFamilyDraft.value = value
})

function updateUiFontSizeDraft(value: string | number) {
  uiFontSizeDraft.value = String(value)
}

function updateCodeFontSizeDraft(value: string | number) {
  codeFontSizeDraft.value = String(value)
}

function updateUiFontFamilyDraft(value: string | number) {
  uiFontFamilyDraft.value = String(value)
}

function updateCodeFontFamilyDraft(value: string | number) {
  codeFontFamilyDraft.value = String(value)
}

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
