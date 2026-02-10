<template>
  <div class="flex gap-4 items-start">
    <div class="p-2 rounded-full bg-[#F9F9F9] dark:bg-[#666]">
      <FontAwesomeIcon :icon="['fas', 'robot']" />
    </div>
    <section class="w-[90%]">
      <sup class="font-semibold">
        {{ robotSay.type }}
      </sup>
      <p class="leading-7 text-muted-foreground break-all">
        <LoadingDots v-if="robotSay.state === 'thinking'" />
        <MarkdownRender
          v-else
          :content="robotSay.description"
          custom-id="chat-answer"
        />
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { robot } from '@memoh/shared'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import LoadingDots from '@/components/loading-dots/index.vue'

enableKatex()
enableMermaid()

defineProps<{
  robotSay: robot
}>()
</script>
