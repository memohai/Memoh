<template>
  <component
    :is="iconComponent"
    v-bind="$attrs"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Component } from 'vue'
import { Mail } from 'lucide-vue-next'
import { Gmail } from '@memohai/icon'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  provider?: string | null
}>()

const iconMap: Record<string, Component> = {
  gmail: Gmail,
}

const iconComponent = computed(() => iconMap[props.provider ?? ''] ?? Mail)
</script>
