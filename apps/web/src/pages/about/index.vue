<template>
  <section class="max-w-7xl mx-auto p-4 pb-12">
    <div class="max-w-3xl mx-auto space-y-6">
      <!-- Header: Logo + Version + Check Button -->
      <div class="flex items-center gap-3">
        <img
          src="/logo.svg"
          alt="Memoh"
          class="size-10 shrink-0 rounded-lg"
        >
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold">
            Memoh
          </p>
          <div class="flex items-center gap-2 mt-0.5">
            <Badge
              v-if="normalizedServerVersion"
              variant="secondary"
            >
              {{ $t('settings.versionTag', { version: normalizedServerVersion }) }}
            </Badge>
            <Badge
              v-if="commitHash"
              variant="outline"
            >
              {{ commitHash }}
            </Badge>
          </div>
        </div>
        <Button
          size="sm"
          variant="secondary"
          :disabled="checking"
          @click="checkForUpdates"
        >
          <Spinner
            v-if="checking"
            class="size-3"
          />
          <RefreshCw
            v-else
            class="size-3"
          />
          {{ checking ? $t('about.checking') : $t('about.checkForUpdates') }}
        </Button>
      </div>

      <!-- Update Result -->
      <template v-if="checkResult">
        <div
          v-if="checkResult.isUpToDate"
          class="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <CircleCheck class="size-3.5 text-green-500" />
          {{ $t('about.upToDate') }}
        </div>

        <template v-else>
          <Separator />

          <div class="flex items-center gap-2">
            <Badge class="bg-[#8B56E3] text-white hover:bg-[#8B56E3]/90">
              {{ $t('about.newVersionAvailable', { version: checkResult.latestVersion }) }}
            </Badge>
          </div>

          <div
            v-if="checkResult.body"
            class="space-y-2"
          >
            <h4 class="text-xs font-medium text-muted-foreground">
              {{ $t('about.releaseNotes') }}
            </h4>
            <div class="prose prose-xs dark:prose-invert max-w-none *:first:mt-0 text-[0.8rem] leading-relaxed">
              <MarkdownRender
                :content="checkResult.body"
                :is-dark="isDark"
                :typewriter="false"
                custom-id="release-notes"
              />
            </div>
          </div>
        </template>
      </template>

      <section>
        <Separator class="mb-4" />
        <div class="grid grid-cols-[1fr_auto_1fr] gap-4 px-3 mb-3">
          <div class="flex items-center gap-2">
            <Globe class="size-3.5 shrink-0 text-muted-foreground" />
            <span class="flex-1 text-xs">{{ $t('settings.language') }}</span>
            <Select
              :model-value="language"
              @update:model-value="(v) => v && setLanguage(v as Locale)"
            >
              <SelectTrigger
                class="w-24"
                :aria-label="$t('settings.language')"
              >
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
          <Separator orientation="vertical" />
          <div class="flex items-center gap-2">
            <span class="flex-1 text-xs">{{ $t('settings.theme') }}</span>
            <div class="flex h-9 items-center rounded-md border border-input p-1">
              <button
                class="flex size-7 items-center justify-center rounded-sm transition-colors"
                :class="theme === 'light' ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground'"
                :aria-label="$t('settings.themeLight')"
                @click="setTheme('light')"
              >
                <Sun class="size-4" />
              </button>
              <button
                class="flex size-7 items-center justify-center rounded-sm transition-colors"
                :class="theme === 'dark' ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:text-foreground'"
                :aria-label="$t('settings.themeDark')"
                @click="setTheme('dark')"
              >
                <Moon class="size-4" />
              </button>
            </div>
          </div>
        </div>
        <div class="space-y-1">
          <a
            href="https://github.com/memohai/memoh"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-9 items-center gap-3 rounded-lg px-3 text-xs text-foreground hover:bg-accent transition-colors"
          >
            <Github class="size-4 text-muted-foreground" />
            {{ $t('about.github') }}
            <ExternalLink class="size-3 ml-auto text-muted-foreground" />
          </a>
          <a
            href="https://docs.memoh.ai"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-9 items-center gap-3 rounded-lg px-3 text-xs text-foreground hover:bg-accent transition-colors"
          >
            <BookOpen class="size-4 text-muted-foreground" />
            {{ $t('about.docs') }}
            <ExternalLink class="size-3 ml-auto text-muted-foreground" />
          </a>
          <a
            href="https://github.com/memohai/memoh/issues"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-9 items-center gap-3 rounded-lg px-3 text-xs text-foreground hover:bg-accent transition-colors"
          >
            <MessageSquare class="size-4 text-muted-foreground" />
            {{ $t('about.feedback') }}
            <ExternalLink class="size-3 ml-auto text-muted-foreground" />
          </a>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { RefreshCw, ExternalLink, Github, BookOpen, MessageSquare, CircleCheck, Globe, Sun, Moon } from 'lucide-vue-next'
import { Badge, Button, Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue, Separator, Spinner } from '@memohai/ui'
import MarkdownRender from 'markstream-vue'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useSettingsStore } from '@/store/settings'
import type { Locale } from '@/i18n'

const GITHUB_REPO = 'memohai/memoh'

interface CheckResult {
  isUpToDate: boolean
  latestVersion: string
  body: string
  htmlUrl: string
}

const { t } = useI18n()

const capabilitiesStore = useCapabilitiesStore()
const { serverVersion, commitHash } = storeToRefs(capabilitiesStore)
const normalizeVersion = (version?: string | null) => (version ?? '').replace(/^v/i, '')
const normalizedServerVersion = computed(() => normalizeVersion(serverVersion.value))

const settingsStore = useSettingsStore()
const { language, theme } = storeToRefs(settingsStore)
const { setLanguage, setTheme } = settingsStore
const isDark = computed(() => settingsStore.theme === 'dark')

const checking = ref(false)
const checkResult = ref<CheckResult | null>(null)

onMounted(async () => {
  await capabilitiesStore.load()
  await checkForUpdates()
})

async function checkForUpdates() {
  checking.value = true
  checkResult.value = null
  try {
    const res = await fetch(`https://api.github.com/repos/${GITHUB_REPO}/releases/latest`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()

    const tagName: string = data.tag_name ?? ''
    const latestVersion = normalizeVersion(tagName)
    const currentVersion = normalizeVersion(serverVersion.value)

    checkResult.value = {
      isUpToDate: latestVersion === currentVersion,
      latestVersion,
      body: data.body ?? '',
      htmlUrl: data.html_url ?? `https://github.com/${GITHUB_REPO}/releases/latest`,
    }
  } catch {
    toast.error(t('about.checkFailed'))
  } finally {
    checking.value = false
  }
}
</script>
