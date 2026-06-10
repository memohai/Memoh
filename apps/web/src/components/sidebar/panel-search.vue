<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="px-1.5 pt-1.5 shrink-0">
      <InputGroup size="sm">
        <InputGroupAddon>
          <Search class="size-3.5 text-muted-foreground" />
        </InputGroupAddon>
        <InputGroupInput
          ref="inputRef"
          v-model="query"
          :placeholder="t('chat.searchSessionPlaceholder')"
        />
      </InputGroup>
    </div>
    <Recents
      class="flex-1 min-h-0"
      :search-query="query"
    />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@memohai/ui'
import { Search } from 'lucide-vue-next'
import Recents from './recents.vue'

const { t } = useI18n()
const query = ref('')
const inputRef = ref<{ $el?: HTMLElement } | null>(null)

onMounted(() => {
  void nextTick(() => {
    const el = inputRef.value?.$el
    const input = el?.tagName === 'INPUT' ? (el as HTMLInputElement) : el?.querySelector('input')
    input?.focus()
  })
})
</script>
