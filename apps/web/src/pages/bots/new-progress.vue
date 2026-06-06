<template>
  <section class="relative mx-auto flex min-h-[60vh] max-w-2xl flex-col justify-center px-4 py-10 lg:px-6">
    <div class="flex items-center gap-4">
      <Avatar class="size-12 rounded-full">
        <AvatarImage
          v-if="display?.avatar_url?.trim()"
          :src="display.avatar_url.trim()"
          :alt="displayName"
        />
        <AvatarFallback class="text-base">
          {{ avatarFallback }}
        </AvatarFallback>
      </Avatar>
      <div class="min-w-0">
        <h1 class="truncate text-lg font-semibold">
          {{ $t('bots.create.terminalTitle', { name: displayName }) }}
        </h1>
        <p class="mt-0.5 text-xs text-muted-foreground">
          {{ status === 'error' ? $t('bots.create.failedSubtitle') : $t('bots.create.terminalSubtitle') }}
        </p>
      </div>
    </div>

    <BotCreateTerminal
      :lines="lines"
      class="mt-6"
    />

    <div
      v-if="status === 'error'"
      class="mt-6 flex justify-end gap-3"
    >
      <Button
        variant="outline"
        @click="handleBack"
      >
        {{ $t('bots.create.back') }}
      </Button>
      <Button
        :disabled="status !== 'error'"
        @click="handleRetry"
      >
        {{ $t('bots.create.retry') }}
      </Button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { Avatar, AvatarImage, AvatarFallback, Button } from '@memohai/ui'
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQueryCache } from '@pinia/colada'
import { getBotsQueryKey } from '@memohai/sdk/colada'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useBotCreateProgressStore } from '@/store/bot-create-progress'
import BotCreateTerminal from './components/bot-create-terminal.vue'

const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()
const store = useBotCreateProgressStore()
const { status, lines, display, bot, setupError } = storeToRefs(store)

const displayName = computed(() => display.value?.display_name || '')
const avatarFallback = useAvatarInitials(() => displayName.value)

let navigated = false
let readyRedirectTimer: ReturnType<typeof window.setTimeout> | null = null
let resetTimer: ReturnType<typeof window.setTimeout> | null = null

function clearReadyRedirectTimer() {
  if (readyRedirectTimer === null) return
  window.clearTimeout(readyRedirectTimer)
  readyRedirectTimer = null
}

function clearResetTimer() {
  if (resetTimer === null) return
  window.clearTimeout(resetTimer)
  resetTimer = null
}

function clearTimers() {
  clearReadyRedirectTimer()
  clearResetTimer()
}

function goToBot() {
  if (navigated) return
  clearReadyRedirectTimer()
  navigated = true
  const target = bot.value?.name ?? bot.value?.id
  if (setupError.value) {
    toast.error(setupError.value)
  } else {
    toast.success(t('bots.createBotSuccess'))
  }
  void queryCache.invalidateQueries({ key: getBotsQueryKey() })
  if (target) {
    router.replace({ name: 'bot-detail', params: { botName: target } })
  } else {
    router.replace({ name: 'bots' })
  }
  // Clear once the navigation is committed so a fresh create starts clean.
  resetTimer = window.setTimeout(() => {
    resetTimer = null
    store.reset()
  }, 0)
}

function scheduleReadyRedirect() {
  clearReadyRedirectTimer()
  // Brief pause so the final "ready" line is visible before redirecting.
  readyRedirectTimer = window.setTimeout(() => {
    readyRedirectTimer = null
    goToBot()
  }, 700)
}

function guardLiveProgress() {
  if (status.value === 'idle') {
    router.replace({ name: 'bot-new' })
    return
  }
  if (status.value === 'ready' && !navigated) {
    scheduleReadyRedirect()
  }
}

watch(
  status,
  (value) => {
    if (value === 'ready') {
      scheduleReadyRedirect()
    } else {
      clearReadyRedirectTimer()
    }
  },
  { immediate: true },
)

onMounted(() => {
  // Direct navigation or a refresh drops the in-memory stream, so send the user
  // back to the form rather than showing an empty terminal.
  guardLiveProgress()
})

onActivated(guardLiveProgress)
onDeactivated(clearTimers)
onBeforeUnmount(clearTimers)

function handleRetry() {
  navigated = false
  void store.retry()
}

function handleBack() {
  store.reset()
  router.replace({ name: 'bot-new' })
}
</script>
