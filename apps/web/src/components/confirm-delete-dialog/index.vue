<template>
  <!-- 删除确认对话框的唯一 owner。抽出前三处(recents 会话删除、sidebar/panel-schedule 与
       dockview/panel-schedule 的任务删除,后两处近乎逐字节相同)各写一遍同一套
       Dialog + Title(+Description)+ outline 取消 + destructive 确认;宽度(sm/md)、
       说明文案放 DialogDescription 还是裸 <p>、取消键删除中是否禁用,三处各漂各的。
       统一为:sm:max-w-sm 紧凑确认框、说明走 DialogDescription(标准对话框解剖,不再裸 <p>)、
       删除进行中取消键禁用(防止关掉对话框但请求仍在飞)。
       文案由调用方传入(标题/说明/确认键各面自己的 i18n key);组件只固定骨架与行为。 -->
  <Dialog
    :open="open"
    @update:open="(value) => emit('update:open', value)"
  >
    <DialogContent class="sm:max-w-sm">
      <DialogHeader>
        <DialogTitle>{{ title }}</DialogTitle>
        <DialogDescription v-if="description">
          {{ description }}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button
          variant="outline"
          :disabled="loading"
          @click="emit('update:open', false)"
        >
          {{ t('common.cancel') }}
        </Button>
        <Button
          variant="destructive"
          :loading="loading"
          @click="emit('confirm')"
        >
          {{ confirmLabel ?? t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@memohai/ui'

defineProps<{
  open: boolean
  title: string
  description?: string
  /** 确认键文案;不传时用 common.confirm(schedule 面传更具体的「删除」)。 */
  confirmLabel?: string
  /** 删除请求进行中:确认键转 spinner,取消键禁用。 */
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  confirm: []
}>()

const { t } = useI18n()
</script>
