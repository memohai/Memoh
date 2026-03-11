<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'address-book']"
        class="size-3 text-muted-foreground"
      />
      <span class="font-mono font-medium text-xs text-muted-foreground">
        get_contacts
      </span>
      <Badge
        v-if="block.done && contacts.length"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolContactsCount', { count: contacts.length }) }}
      </Badge>
      <Badge
        v-else-if="block.done"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolDone') }}
      </Badge>
      <Badge
        v-else
        variant="outline"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolRunning') }}
      </Badge>
    </div>

    <Collapsible
      v-if="block.done && contacts.length"
      v-model:open="contactsOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': contactsOpen }"
        />
        {{ $t('chat.toolSearchResultsLabel') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div class="px-3 pb-2 space-y-1.5">
          <div
            v-for="(item, i) in contacts"
            :key="i"
            class="flex items-center gap-2 text-xs"
          >
            <span class="text-foreground truncate">
              {{ item.display_name || item.username || item.target }}
            </span>
            <Badge
              v-if="item.platform"
              variant="outline"
              class="text-[10px] shrink-0"
            >
              {{ item.platform }}
            </Badge>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

interface Contact {
  route_id: string
  platform: string
  conversation_type: string
  target: string
  display_name: string
  username: string
}

const props = defineProps<{ block: ToolCallBlock }>()

const contactsOpen = ref(false)

const contacts = computed<Contact[]>(() => {
  if (!props.block.done || !props.block.result) return []
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc ?? result).contacts as Contact[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
