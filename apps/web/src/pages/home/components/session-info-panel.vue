<template>
  <ScrollArea class="h-full">
    <div class="px-4 py-3">
      <!-- No session -->
      <div
        v-if="!sessionId"
        class="flex h-40 items-center justify-center"
      >
        <p class="text-body text-muted-foreground">
          {{ $t('chat.infoNoData') }}
        </p>
      </div>

      <template v-else>
        <!-- Key-value rows -->
        <div class="divide-y divide-border text-body">
          <!-- Messages -->
          <div class="flex items-center justify-between py-2">
            <span class="text-muted-foreground">{{ $t('chat.infoMessages') }}</span>
            <span class="font-medium text-foreground tabular-nums">{{ info?.message_count ?? '--' }}</span>
          </div>

          <!-- Context Usage -->
          <div class="py-2 space-y-1.5">
            <div class="flex items-center justify-between">
              <span class="text-muted-foreground">{{ $t('chat.infoContextUsage') }}</span>
              <span class="font-medium text-foreground tabular-nums">
                <template v-if="contextWindow != null">
                  {{ formatTokenCount(usedTokens) }} / {{ formatTokenCount(contextWindow) }}
                  <span class="text-muted-foreground font-normal ml-1">({{ contextPercent.toFixed(1) }}%)</span>
                </template>
                <template v-else>
                  {{ formatTokenCount(usedTokens) }} / --
                </template>
              </span>
            </div>
            <div
              v-if="contextWindow != null && contextWindow > 0"
              class="h-1.5 w-full overflow-hidden rounded-full bg-accent"
            >
              <div
                class="h-full rounded-full transition-all"
                :class="contextBarColor"
                :style="{ width: `${Math.min(contextPercent, 100)}%` }"
              />
            </div>
          </div>

          <!-- Cache Hit Rate -->
          <div class="flex items-center justify-between py-2">
            <span class="text-muted-foreground">{{ $t('chat.infoCacheHitRate') }}</span>
            <span class="font-medium text-foreground tabular-nums">{{ cacheHitRate }}%</span>
          </div>

          <!-- Cache Read -->
          <div class="flex items-center justify-between py-2">
            <span class="text-muted-foreground">{{ $t('chat.infoCacheRead') }}</span>
            <span class="font-medium text-foreground tabular-nums">{{ formatTokenCount(info?.cache_stats?.cache_read_tokens ?? 0) }}</span>
          </div>
        </div>

        <!-- Compact Now -->
        <Button
          variant="secondary"
          size="sm"
          class="mt-3 w-full"
          :disabled="!sessionId || usedTokens <= 0 || isCompacting"
          @click="triggerCompact"
        >
          <Loader2
            v-if="isCompacting"
            class="size-3.5 animate-spin"
          />
          <Minimize2
            v-else
            class="size-3.5"
          />
          {{ $t('chat.compactNow') }}
        </Button>

        <!-- Skills -->
        <div class="mt-4">
          <p class="mb-1.5 text-caption font-medium uppercase tracking-wider text-muted-foreground">
            {{ $t('chat.infoSkills') }}
          </p>
          <p
            v-if="!skills.length"
            class="text-body text-muted-foreground"
          >
            {{ $t('chat.infoNoSkills') }}
          </p>
          <div
            v-else
            class="space-y-0.5"
          >
            <Button
              v-for="skill in skills"
              :key="skill"
              variant="ghost"
              size="sm"
              class="w-full justify-start gap-1.5 px-2 text-body font-normal"
              @click="openSkillFile(skill)"
            >
              <Sparkles class="size-3.5 shrink-0 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate text-left">{{ skill }}</span>
              <ExternalLink class="size-3.5 shrink-0 text-muted-foreground" />
            </Button>
          </div>
        </div>
      </template>
    </div>
  </ScrollArea>
</template>

<script setup lang="ts">
import { computed, inject, ref, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { toast, ScrollArea, Button } from '@memohai/ui'
import { Sparkles, ExternalLink, Loader2, Minimize2 } from 'lucide-vue-next'
import { getBotsByBotIdContainerSkills, postBotsByBotIdSessionsBySessionIdCompact } from '@memohai/sdk'
import type { HandlersSkillItem } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import { useSessionInfo } from '../composables/useSessionInfo'

const props = defineProps<{
  visible: boolean
  overrideModelId?: string
  fallbackContextWindow?: number | null
}>()

const { t } = useI18n()
const openInFileManager = inject(openInFileManagerKey, undefined)
const queryCache = useQueryCache()

type SkillItem = HandlersSkillItem & {
  source_path?: string
  state?: string
}

const visibleRef = toRef(props, 'visible')
const overrideModelIdRef = computed(() => props.overrideModelId ?? '')
const fallbackContextWindowRef = computed(() => props.fallbackContextWindow ?? null)

const { info, usedTokens, contextWindow, contextPercent, currentBotId, sessionId } = useSessionInfo({
  visible: visibleRef,
  overrideModelId: overrideModelIdRef,
  fallbackContextWindow: fallbackContextWindowRef,
})

const { data: skillCatalog } = useQuery({
  key: () => ['bot-skills-catalog', currentBotId.value ?? ''],
  query: async () => {
    const { data } = await getBotsByBotIdContainerSkills({
      path: {
        bot_id: currentBotId.value!,
      },
      throwOnError: true,
    })
    return (data.skills || []) as SkillItem[]
  },
  enabled: () => !!currentBotId.value && props.visible,
  refetchOnWindowFocus: false,
})

const contextBarColor = computed(() => {
  if (contextPercent.value >= 90) return 'bg-destructive'
  if (contextPercent.value >= 70) return 'bg-warning'
  return 'bg-foreground'
})

const cacheHitRate = computed(() => {
  const rate = info.value?.cache_stats?.cache_hit_rate ?? 0
  return rate.toFixed(1)
})

const skills = computed(() => info.value?.skills ?? [])
const effectiveSkillPathByName = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  for (const item of skillCatalog.value || []) {
    if (item.state !== 'effective' || !item.name || !item.source_path) continue
    out[item.name] = item.source_path
  }
  return out
})

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function openSkillFile(skillName: string) {
  openInFileManager?.(effectiveSkillPathByName.value[skillName] || `/data/skills/${skillName}/SKILL.md`, false)
}

const isCompacting = ref(false)

async function triggerCompact() {
  const botId = currentBotId.value
  const sid = sessionId.value
  if (!botId || !sid || isCompacting.value) return

  isCompacting.value = true
  try {
    await postBotsByBotIdSessionsBySessionIdCompact({
      path: { bot_id: botId, session_id: sid },
      throwOnError: true,
    })
    toast.success(t('chat.compactSuccess'))
    queryCache.invalidateQueries({ key: ['session-status', botId, sid] })
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('chat.compactFailed')))
  }
  finally {
    isCompacting.value = false
  }
}
</script>
