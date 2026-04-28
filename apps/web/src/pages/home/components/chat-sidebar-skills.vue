<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="flex items-center gap-1 border-b border-border px-2 py-1.5 shrink-0">
      <span class="text-[11px] font-medium text-muted-foreground tracking-wide uppercase">
        {{ t('chat.activityTabSkills') }}
      </span>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0 ml-auto"
        :disabled="isLoading"
        :title="t('common.refresh')"
        @click="reload"
      >
        <RefreshCw
          class="size-3.5"
          :class="{ 'animate-spin': isLoading }"
        />
      </Button>
    </div>

    <div class="flex-1 min-h-0 relative">
      <ScrollArea class="absolute inset-0">
        <div class="px-2 py-2">
          <div
            v-if="!skills.length && !isLoading"
            class="flex flex-col items-center justify-center py-12 text-center text-muted-foreground"
          >
            <Sparkles class="mb-2 size-6 opacity-40" />
            <p class="text-xs">
              {{ t('chat.skillsEmpty') }}
            </p>
          </div>

          <div
            v-else
            class="flex flex-col gap-1"
          >
            <button
              v-for="skill in skills"
              :key="skillKey(skill)"
              type="button"
              class="flex items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors disabled:cursor-default"
              :class="skill.source_path
                ? 'cursor-pointer hover:bg-sidebar-accent/40'
                : 'cursor-default'"
              :disabled="!skill.source_path"
              :title="skill.source_path"
              @click="handleSkillClick(skill)"
            >
              <Sparkles class="size-3.5 text-muted-foreground shrink-0 mt-0.5" />
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  <span class="truncate text-xs font-medium text-foreground leading-[18px]">
                    {{ skill.name || t('chat.untitledSkill') }}
                  </span>
                  <Badge
                    v-if="skill.state && skill.state !== 'effective'"
                    variant="outline"
                    class="text-[9px] px-1 py-0 h-3.5 leading-none shrink-0"
                  >
                    {{ stateLabel(skill.state) }}
                  </Badge>
                </div>
                <p
                  v-if="skill.description"
                  class="text-[11px] text-muted-foreground line-clamp-2 leading-[14px] mt-0.5"
                >
                  {{ skill.description }}
                </p>
              </div>
            </button>
          </div>
        </div>
      </ScrollArea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Sparkles, RefreshCw } from 'lucide-vue-next'
import { Button, ScrollArea, Badge } from '@memohai/ui'
import { getBotsByBotIdContainerSkills } from '@memohai/sdk'
import type { HandlersSkillItem } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const openInFileManager = inject(openInFileManagerKey, undefined)

const { data, isLoading, error } = useQuery({
  key: () => ['bot-skills-catalog', props.botId],
  query: async () => {
    const { data: resp } = await getBotsByBotIdContainerSkills({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return (resp.skills ?? []) as HandlersSkillItem[]
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: false,
})

const skills = computed<HandlersSkillItem[]>(() => data.value ?? [])

watch(error, (err) => {
  if (err) toast.error(resolveApiErrorMessage(err, t('bots.skills.loadFailed')))
})

function skillKey(skill: HandlersSkillItem): string {
  return skill.source_path || `${skill.name ?? ''}:${skill.source_kind ?? ''}`
}

function stateLabel(state?: string): string {
  switch (state) {
    case 'disabled': return t('bots.skills.disabledBadge')
    case 'shadowed': return t('bots.skills.shadowedBadge')
    default: return t('bots.skills.effectiveBadge')
  }
}

function reload() {
  queryCache.invalidateQueries({ key: ['bot-skills-catalog', props.botId] })
}

function parentDir(filePath: string): string {
  const idx = filePath.lastIndexOf('/')
  if (idx <= 0) return '/'
  return filePath.slice(0, idx)
}

function handleSkillClick(skill: HandlersSkillItem) {
  const path = skill.source_path
  if (!path) return
  openInFileManager?.(parentDir(path), true)
}
</script>
