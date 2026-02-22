<template>
  <section>
    <AddPlatform v-model:open="open" />

    <menu class="grid grid-cols-4 gap-4 [&_li>*]:h-full">
      <PlatformCard
        v-for="item in platformList"
        :key="item.name"
        :platform="item"
        @edit="() => { open = true }"
      />
    </menu>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import AddPlatform from '@/components/add-platform/index.vue'
import PlatformCard from './components/platform-card.vue'
import { usePlatformList } from '@/composables/api/usePlatform'

const open = ref(false)

const { data: platformList } = usePlatformList()
</script>
