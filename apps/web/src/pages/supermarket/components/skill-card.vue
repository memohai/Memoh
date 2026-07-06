<template>
  <MarketItemCard
    :name="skill.name"
    :description="skill.description"
    :homepage="skill.metadata?.homepage"
    @open="openDetail"
  >
    <template #leading>
      <Zap class="size-4 text-muted-foreground" />
    </template>

    <template #actions>
      <Button
        size="sm"
        class="shrink-0"
        @click="$emit('install', skill)"
      >
        <Download class="size-3.5" />
        {{ $t('supermarket.install') }}
      </Button>
    </template>
  </MarketItemCard>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Zap, Download } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { HandlersSupermarketSkillEntry } from '@memohai/sdk'
import MarketItemCard from './market-item-card.vue'

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
