<template>
  <MarketItemCard
    :name="plugin.name"
    :description="plugin.description"
    :homepage="plugin.homepage"
    @open="openDetail"
  >
    <template #leading>
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
    </template>

    <template
      v-if="$slots.actions || props.showInstall"
      #actions
    >
      <slot name="actions">
        <Button
          v-if="props.showInstall"
          size="sm"
          class="shrink-0"
          @click="$emit('install', plugin)"
        >
          <Download class="size-3.5" />
          {{ $t('supermarket.install') }}
        </Button>
      </slot>
    </template>
  </MarketItemCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Download, PackageOpen } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { PluginsManifest } from '@memohai/sdk'
import ProviderIcon from '@/components/provider-icon/index.vue'
import MarketItemCard from './market-item-card.vue'

const props = withDefaults(defineProps<{
  plugin: PluginsManifest
  showInstall?: boolean
}>(), {
  showInstall: true,
})

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
