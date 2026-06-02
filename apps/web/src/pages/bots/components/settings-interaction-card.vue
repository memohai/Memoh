<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <!-- Header Section -->
    <div class="space-y-0.5">
      <h4 class="text-xs font-medium text-foreground">
        {{ $t('bots.settings.blocks.interaction') }}
      </h4>
      <p class="text-[11px] text-muted-foreground">
        {{ $t('bots.settings.blocks.interactionDescription') }}
      </p>
    </div>
    
    <!-- Model selection inputs with compact spacing -->
    <div class="space-y-3 pt-1">
      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.chatModel') }}</Label>
        <ModelSelect
          v-model="form.chat_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.chatModel')"
        />
      </div>

      <div class="space-y-1.5">
        <div class="space-y-0.5">
          <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.titleModel') }}</Label>
          <p class="text-[10px] text-muted-foreground">
            {{ $t('bots.settings.titleModelDescription') }}
          </p>
        </div>
        <ModelSelect
          v-model="form.title_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('bots.settings.titleModelPlaceholder')"
        />
      </div>
    </div>

    <Separator class="bg-border my-4" />

    <div class="space-y-3">
      <div class="space-y-1.5">
        <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.reasoningEffort') }}</Label>
        <Popover v-model:open="reasoningPopoverOpen">
          <PopoverTrigger as-child>
            <Button
              variant="outline"
              role="combobox"
              :disabled="!chatModelSupportsReasoning"
              class="w-full justify-between font-normal shadow-none h-8 bg-transparent text-xs"
            >
              <span class="flex items-center gap-2">
                <Lightbulb
                  class="size-3"
                  :style="{ opacity: EFFORT_OPACITY[reasoningFormValue] ?? 0.5 }"
                />
                {{ $t(EFFORT_LABELS[reasoningFormValue] ?? reasoningFormValue) }}
              </span>
              <ChevronDown class="size-3.5 shrink-0 text-muted-foreground" />
            </Button>
          </PopoverTrigger>
          <PopoverContent
            class="w-[--reka-popover-trigger-width] p-0 shadow-md rounded-md"
            align="start"
          >
            <ReasoningEffortSelect
              v-model="reasoningFormValue"
              :efforts="availableReasoningEfforts"
              @update:model-value="reasoningPopoverOpen = false"
            />
          </PopoverContent>
        </Popover>
      </div>

      <div class="flex items-center justify-between gap-4 rounded-md border border-border p-3 shadow-none bg-background">
        <div class="space-y-0.5">
          <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.showToolCallsInIM') }}</Label>
          <p class="text-[10px] text-muted-foreground">
            {{ $t('bots.settings.showToolCallsInIMDescription') }}
          </p>
        </div>
        <Switch
          :model-value="form.show_tool_calls_in_im"
          class="shadow-none scale-[0.8] origin-right"
          @update:model-value="(val) => form.show_tool_calls_in_im = !!val"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Label, Separator, Popover, PopoverTrigger, PopoverContent, Button, Switch } from '@memohai/ui'
import { Lightbulb, ChevronDown } from 'lucide-vue-next'
import ModelSelect from './model-select.vue'
import ReasoningEffortSelect from './reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY, REASONING_EFFORT_DISABLE, availableEffortsForMode, resolveEffortLevels, resolveThinkingMode } from './reasoning-effort'
import type { SettingsSettings, ModelsGetResponse, ProvidersGetResponse } from '@memohai/sdk'

const props = defineProps<{
  form: SettingsSettings
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
}>()

const chatModelConfig = computed(() => {
  if (!props.form.chat_model_id) return undefined
  return props.models.find((m) => m.id === props.form.chat_model_id)?.config
})

const thinkingMode = computed(() => resolveThinkingMode(chatModelConfig.value))

const chatModelSupportsReasoning = computed(() => thinkingMode.value !== 'none')

const effortLevels = computed(() => resolveEffortLevels(chatModelConfig.value))

// Options shown in the picker: toggle adds an explicit "off"; only_adaptive
// offers tiers only (thinking is forced on for the bot).
const availableReasoningEfforts = computed(() =>
  availableEffortsForMode(thinkingMode.value, effortLevels.value),
)

watch([effortLevels, thinkingMode], ([levels, mode]) => {
  // Forced-adaptive models can't keep reasoning off.
  if (mode === 'only_adaptive' && !props.form.reasoning_enabled) {
    // eslint-disable-next-line vue/no-mutating-props
    props.form.reasoning_enabled = true
  }
  if (props.form.reasoning_enabled && !levels.includes(props.form.reasoning_effort)) {
    // eslint-disable-next-line vue/no-mutating-props
    props.form.reasoning_effort = levels.includes('medium') ? 'medium' : levels[0] ?? 'medium'
  }
}, { immediate: true })

const reasoningPopoverOpen = ref(false)

const reasoningFormValue = computed({
  get: () => props.form.reasoning_enabled ? props.form.reasoning_effort : REASONING_EFFORT_DISABLE,
  set: (v: string) => {
    if (v === REASONING_EFFORT_DISABLE) {
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_enabled = false
    } else {
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_enabled = true
      // eslint-disable-next-line vue/no-mutating-props
      props.form.reasoning_effort = v
    }
  },
})
</script>
