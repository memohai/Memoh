<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'envelope']"
        class="size-3 text-muted-foreground"
      />

      <!-- send_email -->
      <template v-if="block.toolName === 'send_email'">
        <span class="text-xs truncate text-foreground">
          <span class="text-muted-foreground">→</span> {{ to }}
        </span>
        <span
          v-if="subject"
          class="text-xs truncate text-muted-foreground"
          :title="subject"
        >
          {{ subject }}
        </span>
      </template>

      <!-- read_email -->
      <template v-else-if="block.toolName === 'read_email'">
        <span
          v-if="readSubject"
          class="text-xs truncate text-foreground"
        >
          {{ readSubject }}
        </span>
        <span
          v-else
          class="font-mono font-medium text-xs text-muted-foreground"
        >
          read_email
        </span>
      </template>

      <!-- list_email / list_email_accounts -->
      <template v-else>
        <span class="font-mono font-medium text-xs text-muted-foreground">
          {{ block.toolName }}
        </span>
      </template>

      <!-- Badge -->
      <Badge
        v-if="block.done && block.toolName === 'list_email' && emailTotal !== null"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolEmailCount', { count: emailTotal }) }}
      </Badge>
      <Badge
        v-else-if="block.done && block.toolName === 'list_email_accounts' && accountCount !== null"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolEmailAccounts', { count: accountCount }) }}
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

    <!-- list_email collapsible -->
    <Collapsible
      v-if="block.done && block.toolName === 'list_email' && emails.length"
      v-model:open="emailsOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': emailsOpen }"
        />
        {{ $t('chat.toolSearchResultsLabel') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div class="px-3 pb-2 space-y-1.5">
          <div
            v-for="(item, i) in emails"
            :key="i"
            class="flex flex-col gap-0.5"
          >
            <div class="flex items-center gap-2">
              <span class="text-xs font-medium text-foreground truncate">{{ item.subject }}</span>
              <span class="text-[10px] text-muted-foreground shrink-0 ml-auto">{{ item.received_at }}</span>
            </div>
            <span class="text-[10px] text-muted-foreground truncate">{{ item.from }}</span>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>

    <!-- read_email collapsible -->
    <Collapsible
      v-if="block.done && block.toolName === 'read_email' && emailBody"
      v-model:open="bodyOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': bodyOpen }"
        />
        {{ $t('chat.toolWriteContent') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-60 overflow-y-auto">{{ emailBody }}</pre>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

interface EmailItem {
  uid: number
  from: string
  subject: string
  received_at: string
}

const props = defineProps<{ block: ToolCallBlock }>()

const emailsOpen = ref(false)
const bodyOpen = ref(false)

const to = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.to as string) ?? ''
})

const subject = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.subject as string) ?? ''
})

function resolveResult() {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const readSubject = computed(() => {
  const r = resolveResult()
  return (r?.subject as string) ?? ''
})

const emailTotal = computed(() => {
  const r = resolveResult()
  if (!r) return null
  const total = r.total
  return typeof total === 'number' ? total : null
})

const accountCount = computed(() => {
  const r = resolveResult()
  if (!r) return null
  const accounts = r.accounts as unknown[] | undefined
  return Array.isArray(accounts) ? accounts.length : null
})

const emails = computed<EmailItem[]>(() => {
  const r = resolveResult()
  if (!r) return []
  const items = r.emails as EmailItem[] | undefined
  return Array.isArray(items) ? items : []
})

const emailBody = computed(() => {
  const r = resolveResult()
  return (r?.body as string) ?? ''
})
</script>
