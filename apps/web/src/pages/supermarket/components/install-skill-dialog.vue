<template>
  <Dialog
    :open="open"
    @update:open="$emit('update:open', $event)"
  >
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ $t('supermarket.skillInstallTitle') }}</DialogTitle>
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
          v-if="skill"
          class="rounded-md border border-border p-3 space-y-1"
        >
          <p class="text-xs font-medium">
            {{ skill.name }}
          </p>
          <p class="text-[11px] text-muted-foreground line-clamp-3">
            {{ skill.description }}
          </p>
          <p
            v-if="skill.files?.length"
            class="text-[11px] text-muted-foreground"
          >
            {{ $t('supermarket.files') }}: {{ skill.files.length }}
          </p>
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
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Button, Spinner,
} from '@memohai/ui'
import {
  postBotsByBotIdSupermarketInstallSkill,
  type HandlersSupermarketSkillEntry,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import BotSelect from '@/components/bot-select/index.vue'

const props = defineProps<{
  open: boolean
  skill: HandlersSupermarketSkillEntry | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'installed': []
}>()

const { t } = useI18n()

const selectedBotId = ref('')
const installing = ref(false)

watch(() => props.open, (open) => {
  if (!open) {
    selectedBotId.value = ''
    installing.value = false
  }
})

async function handleInstall() {
  if (!selectedBotId.value || !props.skill?.id) return
  installing.value = true
  try {
    await postBotsByBotIdSupermarketInstallSkill({
      path: { bot_id: selectedBotId.value },
      body: {
        skill_id: props.skill.id,
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
