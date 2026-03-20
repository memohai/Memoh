<template>
  <section :class="classId">
    <Teleport :to="mountNode">
      <section class="px-4 flex items-center w-full">
        <ChatStatus />
        <section class="ml-2 flex flex-col gap-0.5">
          <h4 class="text-sm">
            Memoh
            <ChatStep />
          </h4>
          <section class="text-xs text-muted-foreground">            
            Memoh Bot (GPT-5.2)
          </section>
        </section>
        <section class="ml-auto">
          <Button variant="ghost">
            <FontAwesomeIcon :icon="['fas', 'arrow-rotate-right']" />
          </Button>
        </section>
      </section>
    </Teleport>
  </section>
</template>

<script lang="ts" setup>
import { onActivated, ref, useId } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import ChatStatus from '@/components/chat/chat-status/index.vue'
import ChatStep from '@/components/chat/chat-step/index.vue'
import { Button } from '@memoh/ui'
const mountNode = ref('.--h-memoh-header')
const classId=useId()
onBeforeRouteLeave(() => {
  mountNode.value=`.${classId}`
})

onActivated(() => {
  mountNode.value='.--h-memoh-header'
})
</script>

