<template>
  <Card class="flex flex-col">
    <CardHeader class="pb-3">
      <div class="flex items-start gap-3">
        <div
          v-if="mcp.icon"
          class="size-9 shrink-0 rounded-md bg-accent flex items-center justify-center overflow-hidden"
        >
          <img
            :src="mcp.icon"
            :alt="mcp.name"
            class="size-5 object-contain"
          >
        </div>
        <div
          v-else
          class="size-9 shrink-0 rounded-md bg-accent flex items-center justify-center"
        >
          <Plug class="size-4 text-muted-foreground" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-1.5">
            <CardTitle
              class="text-sm truncate"
              :title="mcp.name"
            >
              {{ mcp.name }}
            </CardTitle>
            <a
              v-if="mcp.homepage"
              :href="mcp.homepage"
              target="_blank"
              rel="noopener noreferrer"
              class="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
              @click.stop
            >
              <ExternalLink class="size-3" />
            </a>
          </div>
          <div class="flex items-center gap-1.5 mt-1">
            <Badge
              variant="outline"
              size="sm"
            >
              {{ mcp.transport }}
            </Badge>
            <span
              v-if="mcp.author?.name"
              class="text-[11px] text-muted-foreground truncate"
            >
              {{ mcp.author.name }}
            </span>
          </div>
        </div>
      </div>
    </CardHeader>
    <CardContent class="flex-1 pb-3">
      <p class="text-xs text-muted-foreground line-clamp-2">
        {{ mcp.description }}
      </p>
    </CardContent>
    <CardFooter class="pt-0 flex items-center justify-between gap-2">
      <div class="flex flex-wrap gap-1 min-w-0 overflow-hidden">
        <Badge
          v-for="tag in mcp.tags?.slice(0, 3)"
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
        @click.stop="$emit('install', mcp)"
      >
        <Download class="size-3.5 mr-1.5" />
        {{ $t('supermarket.install') }}
      </Button>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import { Plug, Download, ExternalLink } from 'lucide-vue-next'
import { Card, CardHeader, CardTitle, CardContent, CardFooter, Badge, Button } from '@memohai/ui'
import type { HandlersSupermarketMcpEntry } from '@memohai/sdk'

defineProps<{
  mcp: HandlersSupermarketMcpEntry
}>()

defineEmits<{
  'tag-click': [tag: string]
  'install': [mcp: HandlersSupermarketMcpEntry]
}>()
</script>
