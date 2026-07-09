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
          :disabled="!sessionId || usedTokens <= 0"
          :loading="isCompacting"
          loading-mode="icon"
          @click="triggerCompact"
        >
          <Minimize2 class="size-3.5" />
          {{ $t('chat.compactNow') }}
        </Button>

        <!-- Subagents -->
        <div class="mt-4">
          <SubagentList />
        </div>

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
            <div
              v-for="skill in skills"
              :key="skill"
              class="flex min-h-8 items-center gap-1.5 rounded-md px-2 text-body text-foreground"
            >
              <Sparkles class="size-3.5 shrink-0 text-muted-foreground" />
              <span class="min-w-0 flex-1 truncate text-left">{{ skill }}</span>
            </div>
          </div>
        </div>
      </template>
    </div>
  </ScrollArea>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { ScrollArea, Button } from '@felinic/ui'
import { Sparkles, Minimize2 } from 'lucide-vue-next'
import { useSessionInfo } from '../composables/useSessionInfo'
import SubagentList from './subagent-list.vue'

const props = defineProps<{
  visible: boolean
  overrideModelId?: string
  fallbackContextWindow?: number | null
}>()

const visibleRef = toRef(props, 'visible')
const overrideModelIdRef = computed(() => props.overrideModelId ?? '')
const fallbackContextWindowRef = computed(() => props.fallbackContextWindow ?? null)

const { info, usedTokens, contextWindow, contextPercent, sessionId, isCompacting, triggerCompact } = useSessionInfo({
  visible: visibleRef,
  overrideModelId: overrideModelIdRef,
  fallbackContextWindow: fallbackContextWindowRef,
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

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

</script>
