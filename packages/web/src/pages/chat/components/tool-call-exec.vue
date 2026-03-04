<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'terminal']"
        class="size-3 text-muted-foreground"
      />
      <span
        class="font-mono text-xs truncate text-foreground"
        :title="command"
      >
        $ {{ displayCommand }}
      </span>
      <Badge
        v-if="block.done && exitCode !== null"
        :variant="exitCode === 0 ? 'secondary' : 'destructive'"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolExecExit', { code: exitCode }) }}
      </Badge>
      <Badge
        v-else-if="block.done && isError"
        variant="destructive"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolExecError') }}
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
      v-if="hasOutput"
      v-model:open="outputOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': outputOpen }"
        />
        {{ $t('chat.toolExecOutput') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre
          v-if="stdout"
          class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all"
        >{{ stdout }}</pre>
        <pre
          v-if="stderr"
          class="px-3 pb-2 text-xs text-destructive/80 overflow-x-auto whitespace-pre-wrap break-all"
        >{{ stderr }}</pre>
        <pre
          v-if="errorText"
          class="px-3 pb-2 text-xs text-destructive overflow-x-auto whitespace-pre-wrap break-all"
        >{{ errorText }}</pre>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const outputOpen = ref(false)

const command = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.command as string) ?? ''
})

const displayCommand = computed(() => {
  const lines = command.value.split('\n')
  if (lines.length <= 1) return command.value
  return lines[0] + ' ...'
})

function resolveResult() {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  return sc ?? result
}

const isError = computed(() => {
  if (!props.block.result) return false
  const result = props.block.result as Record<string, unknown>
  return result.isError === true
})

const errorText = computed(() => {
  if (!isError.value) return ''
  const result = props.block.result as Record<string, unknown>
  const content = result.content as Array<Record<string, unknown>> | undefined
  if (!Array.isArray(content)) return ''
  return content
    .filter((c) => c.type === 'text')
    .map((c) => c.text as string)
    .join('\n')
})

const exitCode = computed(() => {
  const r = resolveResult()
  if (!r) return null
  const code = r.exit_code
  return typeof code === 'number' ? code : null
})

const stdout = computed(() => {
  const r = resolveResult()
  return (r?.stdout as string) ?? ''
})

const stderr = computed(() => {
  const r = resolveResult()
  return (r?.stderr as string) ?? ''
})

const hasOutput = computed(() =>
  props.block.done && (stdout.value || stderr.value || errorText.value),
)
</script>
