<template>
  <picture v-if="imageUrl && !failed">
    <source
      v-if="darkImageUrl"
      :srcset="darkImageUrl"
      media="(prefers-color-scheme: dark)"
    >
    <img
      :src="imageUrl"
      alt=""
      :class="imageClass"
      @error="failed = true"
    >
  </picture>
  <Zap
    v-else
    :class="fallbackClass"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Zap } from 'lucide-vue-next'
import type { HandlersSupermarketSkillIcon, HandlersSupermarketSkillImage } from '@memohai/sdk'
import { sdkApiUrl } from '@/lib/api-client'

const props = withDefaults(defineProps<{
  icon?: HandlersSupermarketSkillIcon
  variant?: 'card' | 'detail'
}>(), {
  variant: 'card',
})

const failed = ref(false)
const image = computed(() => props.variant === 'detail'
  ? props.icon?.detail || props.icon?.card
  : props.icon?.card || props.icon?.detail)

function imageURL(value?: HandlersSupermarketSkillImage) {
  const digest = value?.digest?.trim()
  if (!digest || !/^[a-f0-9]{64}$/.test(digest)) return ''
  return sdkApiUrl({ url: '/supermarket/skill-images/{digest}', path: { digest } })
}

const imageUrl = computed(() => imageURL(image.value))
const darkImageUrl = computed(() => imageURL(props.icon?.dark))
const imageClass = computed(() => props.variant === 'detail' ? 'size-11 object-contain' : 'size-5 object-contain')
const fallbackClass = computed(() => props.variant === 'detail'
  ? 'size-8 text-muted-foreground'
  : 'size-4 text-muted-foreground')

watch([imageUrl, darkImageUrl], () => { failed.value = false })
</script>
