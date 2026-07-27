<template>
  <MarketItemCard
    :name="displayName"
    :description="skill.description"
    :homepage="skill.homepage"
    @open="openDetail"
  >
    <template #leading>
      <SkillIcon :icon="skill.icon" />
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
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { Download } from 'lucide-vue-next'
import { Button } from '@felinic/ui'
import type { HandlersSupermarketCatalogSkill } from '@memohai/sdk'
import { formatNamespacedSkillName } from '../utils/display'
import MarketItemCard from './market-item-card.vue'
import SkillIcon from './skill-icon.vue'

const props = defineProps<{
  skill: HandlersSupermarketCatalogSkill
  registryPrefix?: string
}>()

defineEmits<{
  'install': [skill: HandlersSupermarketCatalogSkill]
}>()

const router = useRouter()

const displayName = computed(() => formatNamespacedSkillName(props.skill, props.registryPrefix ?? ''))

function openDetail() {
  if (!props.skill.registry_id || !props.skill.package_id || !props.skill.skill_id) return
  router.push({
    name: 'supermarket-skill-detail',
    params: {
      registryId: props.skill.registry_id,
      packageId: props.skill.package_id,
      skillId: props.skill.skill_id,
    },
  })
}
</script>
