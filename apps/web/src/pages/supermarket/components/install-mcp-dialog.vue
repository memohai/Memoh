<template>
  <Dialog
    :open="open"
    @update:open="$emit('update:open', $event)"
  >
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ $t('supermarket.mcpInstallTitle') }}</DialogTitle>
      </DialogHeader>
      <div class="space-y-4 py-2">
        <div class="space-y-1.5">
          <label class="text-xs font-medium">{{ $t('supermarket.selectBot') }}</label>
          <BotSelect
            v-model="selectedBotId"
            trigger-class="w-full"
          />
        </div>

        <div
          v-if="mcp"
          class="rounded-md border border-border p-3 space-y-1"
        >
          <div class="flex items-center gap-2">
            <p class="text-xs font-medium">
              {{ mcp.name }}
            </p>
            <Badge
              v-if="mcp.transport"
              variant="outline"
              size="sm"
            >
              {{ mcp.transport }}
            </Badge>
          </div>
          <p class="text-[11px] text-muted-foreground line-clamp-2">
            {{ mcp.description }}
          </p>
        </div>
      </div>
      <DialogFooter>
        <DialogClose as-child>
          <Button variant="outline">
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="!selectedBotId"
          @click="handleNavigate"
        >
          {{ $t('supermarket.install') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Button, Badge,
} from '@memohai/ui'
import type { HandlersSupermarketMcpEntry } from '@memohai/sdk'
import BotSelect from '@/components/bot-select/index.vue'
import { useSupermarketMcpDraft } from '@/stores/supermarket-mcp-draft'

const props = defineProps<{
  open: boolean
  mcp: HandlersSupermarketMcpEntry | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
}>()

const router = useRouter()
const { setPendingDraft } = useSupermarketMcpDraft()

const selectedBotId = ref('')

watch(() => props.open, (open) => {
  if (!open) {
    selectedBotId.value = ''
  }
})

function handleNavigate() {
  if (!selectedBotId.value || !props.mcp) return
  setPendingDraft(props.mcp)
  emit('update:open', false)
  router.push({
    name: 'bot-detail',
    params: { botId: selectedBotId.value },
    query: { tab: 'mcp' },
  })
}
</script>
