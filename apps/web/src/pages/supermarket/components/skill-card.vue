<template>
  <Card
    class="group flex cursor-pointer flex-row items-start gap-3 p-4 transition-colors hover:border-foreground/20 hover:bg-accent/20"
    role="button"
    tabindex="0"
    @click="openDetail"
    @keydown.enter.prevent="openDetail"
    @keydown.space.prevent="openDetail"
  >
    <div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-accent">
      <Zap class="size-4 text-muted-foreground" />
    </div>

    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-1.5">
        <h3
          class="truncate text-sm font-medium"
          :title="skill.name"
        >
          {{ skill.name }}
        </h3>
        <a
          v-if="skill.metadata?.homepage"
          :href="skill.metadata.homepage"
          target="_blank"
          rel="noopener noreferrer"
          class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
          @click.stop
        >
          <ExternalLink class="size-3" />
        </a>
      </div>
      <p class="mt-1 line-clamp-2 text-xs text-muted-foreground">
        {{ skill.description }}
      </p>
    </div>

    <Button
      size="sm"
      class="shrink-0"
      @click.stop="$emit('install', skill)"
    >
      <Download class="mr-1.5 size-3.5" />
      {{ $t('supermarket.install') }}
    </Button>
  </Card>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Zap, Download, ExternalLink } from 'lucide-vue-next'
import { Card, Button } from '@memohai/ui'
import type { HandlersSupermarketSkillEntry } from '@memohai/sdk'

const props = defineProps<{
  skill: HandlersSupermarketSkillEntry
}>()

defineEmits<{
  'install': [skill: HandlersSupermarketSkillEntry]
}>()

const router = useRouter()

function openDetail() {
  if (!props.skill.id) return
  router.push({ name: 'supermarket-skill-detail', params: { skillId: props.skill.id } })
}
</script>
