<template>
  <section class="max-w-7xl mx-auto px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12">
    <div class="max-w-3xl mx-auto space-y-6">
      <!-- Top Action Bar -->
      <div class="flex items-start justify-between pb-4 border-b border-border/50">
        <div>
          <h1 class="text-lg font-semibold text-foreground">
            {{ $t('sidebar.about') }}
          </h1>
        </div>
        <Button
          size="sm"
          variant="secondary"
          :disabled="checking"
          @click="checkForUpdates"
        >
          <RefreshCw
            class="size-3 mr-1.5 transition-transform"
            :class="{ 'animate-spin': checking }"
          />
          {{ checking ? $t('about.checking') : $t('about.checkForUpdates') }}
        </Button>
      </div>

      <div class="space-y-6">
        <!-- App Info -->
        <div class="flex items-center gap-4">
          <div class="flex items-center justify-center size-16 shrink-0 rounded-xl border bg-card shadow-sm">
            <img
              src="/logo.svg"
              alt="Memoh"
              class="size-10"
            >
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="text-lg font-semibold text-foreground">
              Memoh
            </h2>
            <div class="flex items-center gap-2 mt-1">
              <Badge
                v-if="normalizedServerVersion"
                variant="secondary"
                class="text-[11px] font-medium"
              >
                {{ $t('settings.versionTag', { version: normalizedServerVersion }) }}
              </Badge>
            </div>
          </div>
        </div>

        <!-- Update Result Box -->
        <div
          v-if="checking || checkResult"
          class="rounded-md border bg-card p-4 space-y-4"
        >
          <div
            v-if="checking"
            class="space-y-3"
          >
            <div class="flex items-center gap-2 text-xs text-muted-foreground">
              <Spinner class="size-3.5" />
              {{ $t('about.checking') }}
            </div>
            <!-- Skeleton for release notes -->
            <div class="space-y-2 pt-2">
              <div class="h-4 w-24 bg-muted/50 rounded animate-pulse" />
              <div class="rounded-md bg-muted/10 p-3 border border-border/50">
                <div class="space-y-3">
                  <div class="h-4 w-1/3 bg-muted/50 rounded animate-pulse" />
                  <div class="space-y-2 pl-4">
                    <div class="h-3 w-5/6 bg-muted/50 rounded animate-pulse" />
                    <div class="h-3 w-4/5 bg-muted/50 rounded animate-pulse" />
                    <div class="h-3 w-full bg-muted/50 rounded animate-pulse" />
                  </div>
                  <div class="h-4 w-1/4 bg-muted/50 rounded animate-pulse mt-4" />
                  <div class="space-y-2 pl-4">
                    <div class="h-3 w-3/4 bg-muted/50 rounded animate-pulse" />
                    <div class="h-3 w-5/6 bg-muted/50 rounded animate-pulse" />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <template v-else-if="checkResult">
            <div
              v-if="checkResult.isUpToDate"
              class="flex items-center gap-2 text-xs text-muted-foreground"
            >
              <CircleCheck class="size-3.5 text-success" />
              {{ $t('about.upToDate') }}
            </div>

            <template v-else>
              <div class="flex items-center gap-2">
                <Badge class="bg-primary text-primary-foreground hover:bg-primary/90">
                  {{ $t('about.newVersionAvailable', { version: checkResult.latestVersion }) }}
                </Badge>
              </div>

              <div
                v-if="checkResult.body"
                class="space-y-2 pt-2"
              >
                <h3 class="text-xs font-medium text-foreground">
                  {{ $t('about.releaseNotes') }}
                </h3>
                <div class="rounded-md bg-muted/20 p-3 border border-border/50">
                  <div class="about-markdown-container prose prose-xs dark:prose-invert max-w-none *:first:mt-0 text-[0.8rem] leading-relaxed">
                    <MarkdownRender
                      :content="cleanMarkdownBody(checkResult.body)"
                      :is-dark="isDark"
                      :typewriter="false"
                      custom-id="release-notes"
                    />
                  </div>
                </div>
              </div>
            </template>
          </template>
        </div>

        <!-- Resources Box -->
        <div class="rounded-md border bg-card p-1.5">
          <div class="space-y-0.5">
            <a
              href="https://github.com/memohai/memoh"
              target="_blank"
              rel="noopener noreferrer"
              class="flex h-9 items-center gap-3 rounded-md px-3 text-xs text-foreground hover:bg-accent transition-colors"
            >
              <Github class="size-4 text-muted-foreground" />
              {{ $t('about.github') }}
              <ExternalLink class="size-3 ml-auto text-muted-foreground" />
            </a>
            <a
              href="https://docs.memoh.ai"
              target="_blank"
              rel="noopener noreferrer"
              class="flex h-9 items-center gap-3 rounded-md px-3 text-xs text-foreground hover:bg-accent transition-colors"
            >
              <BookOpen class="size-4 text-muted-foreground" />
              {{ $t('about.docs') }}
              <ExternalLink class="size-3 ml-auto text-muted-foreground" />
            </a>
            <a
              href="https://github.com/memohai/memoh/issues"
              target="_blank"
              rel="noopener noreferrer"
              class="flex h-9 items-center gap-3 rounded-md px-3 text-xs text-foreground hover:bg-accent transition-colors"
            >
              <MessageSquare class="size-4 text-muted-foreground" />
              {{ $t('about.feedback') }}
              <ExternalLink class="size-3 ml-auto text-muted-foreground" />
            </a>
          </div>
        </div>

        <!-- Meta info -->
        <div class="pt-6 flex justify-center">
          <p
            v-if="commitHash"
            class="text-[11px] text-muted-foreground"
          >
            {{ commitHash }}
          </p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { RefreshCw, ExternalLink, Github, BookOpen, MessageSquare, CircleCheck } from 'lucide-vue-next'
import { Badge, Button, Spinner } from '@memohai/ui'
import MarkdownRender from 'markstream-vue'
import { useCapabilitiesStore } from '@/store/capabilities'
import { useSettingsStore } from '@/store/settings'

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
const isDark = computed(() => settingsStore.theme === 'dark')

const checking = ref(false)
const checkResult = ref<CheckResult | null>(null)

// MutationObserver to robustly strip native tooltips (title attribute)
let titleObserver: MutationObserver | null = null

function stripTitles() {
  const container = document.querySelector('.about-markdown-container')
  if (container) {
    container.querySelectorAll('a').forEach(el => {
      // Remove all attributes that could trigger tooltips (native or custom)
      el.removeAttribute('title')
      el.removeAttribute('aria-label')
      el.removeAttribute('aria-describedby')
    })
  }
}

// Clean GitHub URLs to short references safely
function cleanMarkdownBody(body: string): string {
  if (!body) return ''
  
  return body
    // Match Issue/PR URLs
    .replace(/(?:<)?(https:\/\/github\.com\/[^/]+\/[^/]+\/(?:issues|pull)\/(\d+))(?:>)?/g, (match, url, id, offset, str) => {
      if (str.substring(offset - 2, offset) === '](') return match
      return `[#${id}](${url})`
    })
    // Match Commit URLs
    .replace(/(?:<)?(https:\/\/github\.com\/[^/]+\/[^/]+\/commit\/([a-f0-9]{7})[a-f0-9]*)(?:>)?/g, (match, url, hash, offset, str) => {
      if (str.substring(offset - 2, offset) === '](') return match
      return `[${hash}](${url})`
    })
}

onMounted(async () => {
  // Observe the entire document for any tooltip-related attributes being added
  titleObserver = new MutationObserver(() => stripTitles())
  titleObserver.observe(document.body, { 
    childList: true, 
    subtree: true, 
    attributes: true, 
    attributeFilter: ['title', 'aria-label', 'aria-describedby'] 
  })
  
  await capabilitiesStore.load()
  await checkForUpdates()
})

onUnmounted(() => {
  titleObserver?.disconnect()
})

async function checkForUpdates() {
  checking.value = true
  checkResult.value = null
  try {
    const [res] = await Promise.all([
      fetch(`https://api.github.com/repos/${GITHUB_REPO}/releases/latest`),
      new Promise(resolve => setTimeout(resolve, 800)) // Minimum animation delay
    ])
    
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()

    const tagName: string = data.tag_name ?? ''
    const latestVersion = normalizeVersion(tagName)
    const currentVersion = normalizeVersion(serverVersion.value)

    const isUpToDate = latestVersion === currentVersion
    checkResult.value = {
      isUpToDate,
      latestVersion,
      body: data.body ?? '',
      htmlUrl: data.html_url ?? `https://github.com/${GITHUB_REPO}/releases/latest`,
    }

    if (isUpToDate) {
      toast.success(t('about.upToDate'))
    } else {
      toast.success(t('about.newVersionAvailable', { version: latestVersion }))
    }

    await nextTick()
    stripTitles()
  } catch (error) {
    const reason = error instanceof Error ? error.message : String(error)
    toast.error(`${t('about.checkFailed')}: ${reason}`)
  } finally {
    checking.value = false
  }
}
</script>

<style scoped>
/* Override markstream-vue link animations for the release notes */
.about-markdown-container :deep(.markdown-renderer a.link-node) {
  /* Suppress the breathing/infinite animation */
  --underline-iteration: 0 !important;
  --underline-duration: 0s !important;
  
  /* Reset to a static, thin underline */
  text-decoration: underline !important;
  text-decoration-color: currentColor !important;
  text-decoration-thickness: 1px !important;
  text-underline-offset: 3px !important;
  
  /* Reset color to inherit */
  color: inherit !important;
  opacity: 0.8;
  transition: opacity 0.2s ease, color 0.2s ease;
}

.about-markdown-container :deep(.markdown-renderer a.link-node:hover) {
  opacity: 1;
  color: hsl(var(--primary));
}

/**
 * Suppress native browser tooltips by disabling pointer events on the <a> tag
 * while keeping children (<span>) clickable. 
 */
.about-markdown-container :deep(a.link-no-tooltip) {
  pointer-events: none !important;
  text-decoration: none !important;
}

.about-markdown-container :deep(a.link-no-tooltip > span) {
  pointer-events: auto !important;
  cursor: pointer !important;
  text-decoration: underline !important;
  text-decoration-color: currentColor !important;
  text-decoration-thickness: 1px !important;
  text-underline-offset: 3px !important;
}

/* Forcefully hide ANY custom tooltip containers globally when on this page */
</style>

<style>
/* Global style to suppress tooltip-like floating layers */
body:has(.about-markdown-container) [role="tooltip"],
body:has(.about-markdown-container) [data-role="tooltip"],
body:has(.about-markdown-container) [data-state] {
  /* We use a very specific selector but be careful not to hide functional UI.
     Targeting by data-state is common for Radix/Reka components. */
}

/* If it's a Radix/Reka-ui Tooltip, it usually appears in a Portal at the end of body */
[data-radix-popper-content-wrapper] {
  /* This is a common wrapper for shadcn/ui (radix) tooltips */
}

/* Let's try the most direct way: hide anything that contains the long GitHub URL in a tooltip */
[role="tooltip"] {
  display: none !important;
  visibility: hidden !important;
  opacity: 0 !important;
  pointer-events: none !important;
}

.about-markdown-container :deep(svg) {
  opacity: 0.7;
  transition: opacity 0.2s ease;
}

.about-markdown-container :deep(a.link-node:hover svg) {
  opacity: 1;
}

/* Add the external link icon via CSS ::after */
.about-markdown-container :deep(.markdown-renderer a.link-node::after) {
  content: "";
  display: inline-block;
  width: 0.85em;
  height: 0.85em;
  margin-left: 0.25em;
  margin-bottom: -0.1em;
  background-color: currentColor;
  mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M15 3h6v6'/%3E%3Cpath d='M10 14 21 3'/%3E%3Cpath d='M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6'/%3E%3C/svg%3E");
  -webkit-mask-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M15 3h6v6'/%3E%3Cpath d='M10 14 21 3'/%3E%3Cpath d='M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6'/%3E%3C/svg%3E");
  mask-size: contain;
  -webkit-mask-size: contain;
  mask-repeat: no-repeat;
  -webkit-mask-repeat: no-repeat;
  mask-position: center;
  -webkit-mask-position: center;
  opacity: 0.7;
  transition: opacity 0.2s ease;
}

.about-markdown-container :deep(.markdown-renderer a.link-node:hover::after) {
  opacity: 1;
}
</style>


