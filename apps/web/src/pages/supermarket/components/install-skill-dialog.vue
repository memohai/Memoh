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
        <FieldStack :label="$t('supermarket.selectBot')">
          <BotSelect
            v-model="selectedBotId"
            trigger-class="w-full"
          />
        </FieldStack>

        <div
          v-if="skill"
          class="space-y-1 rounded-md border border-border p-3"
        >
          <p class="text-xs font-medium">
            {{ displayName }}
          </p>
          <p class="line-clamp-3 text-xs text-muted-foreground">
            {{ skill.description }}
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
          :disabled="!selectedBotId"
          :loading="installing"
          @click="handleInstall"
        >
          {{ installing ? $t('supermarket.installing') : $t('supermarket.install') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FieldStack, toast } from '@felinic/ui'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Button,
} from '@felinic/ui'
import {
  postBotsByBotIdSupermarketInstallSkill,
  type HandlersSupermarketCatalogSkill,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import BotSelect from '@/components/bot-select/index.vue'
import { formatNamespacedSkillName } from '../utils/display'

const props = defineProps<{
  open: boolean
  skill: HandlersSupermarketCatalogSkill | null
  registryPrefix?: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'installed': []
}>()

const { t } = useI18n()

const selectedBotId = ref('')
const installing = ref(false)

const displayName = computed(() => {
  if (!props.skill) return ''
  return formatNamespacedSkillName(props.skill, props.registryPrefix ?? '')
})

watch(() => props.open, (open) => {
  if (!open) {
    selectedBotId.value = ''
    installing.value = false
  }
})

async function handleInstall() {
  if (!selectedBotId.value || !props.skill?.registry_id || !props.skill.package_id || !props.skill.skill_id) return
  installing.value = true
  try {
    await postBotsByBotIdSupermarketInstallSkill({
      path: { bot_id: selectedBotId.value },
      body: {
        registry_id: props.skill.registry_id,
        package_id: props.skill.package_id,
        skill_id: props.skill.skill_id,
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
