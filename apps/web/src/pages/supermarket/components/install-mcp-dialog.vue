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
          <NativeSelect
            v-model="selectedBotId"
            class="w-full"
          >
            <option
              value=""
              disabled
            >
              {{ $t('supermarket.selectBotPlaceholder') }}
            </option>
            <option
              v-for="bot in bots"
              :key="bot.id"
              :value="bot.id"
            >
              {{ bot.name }}
            </option>
          </NativeSelect>
        </div>

        <div
          v-if="mcp?.env?.length"
          class="space-y-3"
        >
          <div>
            <label class="text-xs font-medium">{{ $t('supermarket.envVariables') }}</label>
            <p class="text-[11px] text-muted-foreground mt-0.5">
              {{ $t('supermarket.envDescription') }}
            </p>
          </div>
          <div
            v-for="envVar in mcp.env"
            :key="envVar.key"
            class="space-y-1"
          >
            <label class="text-[11px] font-medium text-muted-foreground">
              {{ envVar.key }}
              <span
                v-if="envVar.description"
                class="font-normal"
              > — {{ envVar.description }}</span>
            </label>
            <Input
              v-model="envValues[envVar.key!]"
              :placeholder="envVar.defaultValue || ''"
              class="text-xs"
            />
          </div>
        </div>
      </div>
      <DialogFooter>
        <DialogClose as-child>
          <Button
            variant="outline"
            :disabled="installing"
          >
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="!selectedBotId || installing"
          @click="handleInstall"
        >
          <Spinner
            v-if="installing"
            class="mr-2 size-4"
          />
          {{ installing ? $t('supermarket.installing') : $t('supermarket.install') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { toast } from 'vue-sonner'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Button, Input, NativeSelect, Spinner,
} from '@memohai/ui'
import { getBotsQuery } from '@memohai/sdk/colada'
import {
  postBotsByBotIdSupermarketInstallMcp,
  type HandlersSupermarketMcpEntry,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  open: boolean
  mcp: HandlersSupermarketMcpEntry | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'installed': []
}>()

const { t } = useI18n()

const { data: botsData } = useQuery(getBotsQuery())
const bots = computed(() => botsData.value?.items ?? [])

const selectedBotId = ref('')
const installing = ref(false)
const envValues = reactive<Record<string, string>>({})

watch(() => props.mcp, (mcp) => {
  Object.keys(envValues).forEach((k) => delete envValues[k])
  if (mcp?.env) {
    for (const e of mcp.env) {
      if (e.key) envValues[e.key] = e.defaultValue ?? ''
    }
  }
}, { immediate: true })

watch(() => props.open, (open) => {
  if (!open) {
    selectedBotId.value = ''
    installing.value = false
  }
})

async function handleInstall() {
  if (!selectedBotId.value || !props.mcp?.id) return
  installing.value = true
  try {
    await postBotsByBotIdSupermarketInstallMcp({
      path: { bot_id: selectedBotId.value },
      body: {
        mcp_id: props.mcp.id,
        env: { ...envValues },
      },
      throwOnError: true,
    })
    toast.success(t('supermarket.installSuccess'))
    emit('update:open', false)
    emit('installed')
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('supermarket.installFailed')))
  } finally {
    installing.value = false
  }
}
</script>
