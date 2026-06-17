<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button, Kbd, KbdGroup } from '@memohai/ui'
import { RotateCcw } from 'lucide-vue-next'
import { comboFromBinding, displayKeyCombo } from '@/lib/keyboard-combo'
import { detectPlatform, type KeyboardBinding } from '@/lib/keyboard-bindings'
import { useKeyboardShortcutsStore } from '@/store/keyboard-shortcuts'

const props = defineProps<{
  binding: KeyboardBinding
}>()

const emit = defineEmits<{
  edit: []
}>()

const { t } = useI18n()
const store = useKeyboardShortcutsStore()
const platform = detectPlatform()

const tokens = computed(() => displayKeyCombo(comboFromBinding(props.binding), platform))
const overridden = computed(() => store.isOverridden(props.binding.command))
</script>

<template>
  <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3 last:border-b-0">
    <div class="min-w-0">
      <div class="text-sm font-medium text-foreground">
        {{ t(`settings.keyboard.commands.${binding.i18nKey}.label`) }}
      </div>
      <p class="mt-0.5 text-xs text-muted-foreground">
        {{ t(`settings.keyboard.commands.${binding.i18nKey}.description`) }}
      </p>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <KbdGroup>
        <Kbd
          v-for="(token, i) in tokens"
          :key="i"
        >
          {{ token }}
        </Kbd>
      </KbdGroup>
      <Button
        v-if="overridden"
        variant="ghost"
        size="icon"
        :aria-label="t('settings.keyboard.row.reset')"
        :title="t('settings.keyboard.row.reset')"
        @click="store.resetBinding(binding.command)"
      >
        <RotateCcw class="size-4" />
      </Button>
      <Button
        variant="outline"
        size="sm"
        @click="emit('edit')"
      >
        {{ t('common.edit') }}
      </Button>
    </div>
  </div>
</template>
