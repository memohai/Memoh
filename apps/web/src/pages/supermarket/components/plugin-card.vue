<template>
  <Card
    class="group flex cursor-pointer flex-row items-start gap-3 p-4 transition-colors hover:border-foreground/20 hover:bg-accent/20"
    role="button"
    tabindex="0"
    @click="openDetail"
    @keydown.enter.prevent="openDetail"
    @keydown.space.prevent="openDetail"
  >
    <div class="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-md bg-accent">
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

    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-1.5">
        <h3
          class="truncate text-sm font-medium"
          :title="plugin.name"
        >
          {{ plugin.name }}
        </h3>
        <a
          v-if="plugin.homepage"
          :href="plugin.homepage"
          target="_blank"
          rel="noopener noreferrer"
          class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
          @click.stop
        >
          <ExternalLink class="size-3" />
        </a>
      </div>
      <p class="mt-1 line-clamp-2 text-xs text-muted-foreground">
        {{ plugin.description }}
      </p>
    </div>

    <Button
      size="sm"
      class="shrink-0"
      @click.stop="$emit('install', plugin)"
    >
      <Download class="mr-1.5 size-3.5" />
      {{ $t('supermarket.install') }}
    </Button>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Download, ExternalLink, PackageOpen } from 'lucide-vue-next'
import { Card, Button } from '@memohai/ui'
import type { PluginsManifest } from '@memohai/sdk'
import ProviderIcon from '@/components/provider-icon/index.vue'

const props = defineProps<{
  plugin: PluginsManifest
}>()

defineEmits<{
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
