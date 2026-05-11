<template>
  <Card class="flex flex-col">
    <CardHeader class="pb-3">
      <div class="flex items-start justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <div class="size-9 shrink-0 rounded-md bg-accent flex items-center justify-center">
            <Zap class="size-4 text-muted-foreground" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <CardTitle
                class="text-sm truncate"
                :title="skill.name"
              >
                {{ skill.name }}
              </CardTitle>
              <a
                v-if="skill.metadata?.homepage"
                :href="skill.metadata.homepage"
                target="_blank"
                rel="noopener noreferrer"
                class="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                @click.stop
              >
                <ExternalLink class="size-3" />
              </a>
            </div>
            <span
              v-if="skill.metadata?.author?.name"
              class="text-[11px] text-muted-foreground"
            >
              {{ skill.metadata.author.name }}
            </span>
          </div>
        </div>
      </div>
    </CardHeader>
    <CardContent class="flex-1 pb-3">
      <p class="text-xs text-muted-foreground line-clamp-2">
        {{ skill.description }}
      </p>
    </CardContent>
    <CardFooter class="pt-0 flex items-center justify-between gap-2">
      <div class="flex flex-wrap gap-1 min-w-0 overflow-hidden">
        <Badge
          v-for="tag in skill.metadata?.tags?.slice(0, 3)"
          :key="tag"
          variant="secondary"
          size="sm"
          class="cursor-pointer hover:bg-foreground hover:text-background transition-colors"
          @click.stop="$emit('tag-click', tag)"
        >
          {{ tag }}
        </Badge>
      </div>
      <Button
        size="sm"
        class="shrink-0 self-end"
        @click.stop="$emit('install', skill)"
      >
        <Download class="size-3.5 mr-1.5" />
        {{ $t('supermarket.install') }}
      </Button>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import { Zap, Download, ExternalLink } from 'lucide-vue-next'
import { Card, CardHeader, CardTitle, CardContent, CardFooter, Badge, Button } from '@memohai/ui'
import type { HandlersSupermarketSkillEntry } from '@memohai/sdk'

defineProps<{
  skill: HandlersSupermarketSkillEntry
}>()

defineEmits<{
  'tag-click': [tag: string]
  'install': [skill: HandlersSupermarketSkillEntry]
}>()
</script>
