<template>
  <Card
    class="flex flex-col cursor-pointer hover:border-foreground/20 hover:bg-accent/20"
    role="button"
    tabindex="0"
    @click="openDetail"
    @keydown.enter.prevent="openDetail"
    @keydown.space.prevent="openDetail"
  >
    <CardHeader class="pb-3">
      <div class="flex items-start gap-3">
        <div class="size-9 shrink-0 rounded-md bg-accent flex items-center justify-center overflow-hidden">
          <ProviderIcon
            v-if="iconValue"
            :icon="iconValue"
            size="20"
            class="size-5 object-contain"
          >
            <PackageOpen class="size-4 text-muted-foreground" />
          </ProviderIcon>
          <PackageOpen
            v-else
            class="size-4 text-muted-foreground"
          />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-1.5">
            <CardTitle
              class="text-sm truncate"
              :title="plugin.name"
            >
              {{ plugin.name }}
            </CardTitle>
            <a
              v-if="plugin.homepage"
              :href="plugin.homepage"
              target="_blank"
              rel="noopener noreferrer"
              class="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
              @click.stop
            >
              <ExternalLink class="size-3" />
            </a>
          </div>
          <div class="flex items-center gap-1.5 mt-1">
            <span
              v-if="plugin.author?.name"
              class="text-[11px] text-muted-foreground truncate"
            >
              {{ plugin.author.name }}
            </span>
          </div>
        </div>
      </div>
    </CardHeader>
    <CardContent class="flex-1 pb-3">
      <p class="text-xs text-muted-foreground line-clamp-2">
        {{ plugin.description }}
      </p>
    </CardContent>
    <CardFooter class="pt-0 flex items-center justify-between gap-2">
      <div class="flex flex-wrap gap-1 min-w-0 overflow-hidden">
        <Badge
          v-for="tag in plugin.tags?.slice(0, 3)"
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
        @click.stop="$emit('install', plugin)"
      >
        <Download class="size-3.5 mr-1.5" />
        {{ $t('supermarket.install') }}
      </Button>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Download, ExternalLink, PackageOpen } from 'lucide-vue-next'
import { Card, CardHeader, CardTitle, CardContent, CardFooter, Badge, Button } from '@memohai/ui'
import type { PluginsManifest } from '@memohai/sdk'
import ProviderIcon from '@/components/provider-icon/index.vue'

const props = defineProps<{
  plugin: PluginsManifest
}>()

defineEmits<{
  'tag-click': [tag: string]
  'install': [plugin: PluginsManifest]
}>()

const router = useRouter()

const iconValue = computed(() => {
  const icon = props.plugin.icon
  if (!icon) return ''
  if (icon.kind === 'external_url') return icon.url || ''
  if (icon.kind === 'builtin') return icon.name || ''
  return ''
})

function openDetail() {
  if (!props.plugin.id) return
  router.push({ name: 'supermarket-plugin-detail', params: { pluginId: props.plugin.id } })
}
</script>
