<template>
  <div class="overflow-hidden rounded-lg border border-border bg-card shadow-sm">
    <div
      class="max-h-[45vh] overflow-y-auto overscroll-contain px-3 py-2 pr-2"
      style="scrollbar-gutter: stable;"
    >
      <div
        v-for="(question, questionIndex) in questions"
        :key="question.id"
        :class="questionIndex > 0 ? 'mt-3 border-t border-border/60 pt-3' : ''"
      >
        <p class="whitespace-pre-wrap break-words text-xs font-medium leading-relaxed text-foreground">
          {{ question.text }}
        </p>
        <div>
          <div
            v-if="question.kind !== 'text' && question.options?.length"
            class="mt-2 flex flex-col gap-1"
          >
            <Button
              v-for="option in question.options"
              :key="option.id"
              type="button"
              size="sm"
              variant="ghost"
              class="h-auto min-h-8 w-full justify-start whitespace-normal rounded-md px-2.5 py-1.5 text-left text-xs"
              :class="isOptionSelected(question.id, option.id) ? 'bg-muted text-foreground' : 'text-foreground hover:bg-accent'"
              :title="option.description || option.label"
              :role="question.kind === 'multi_select' ? 'checkbox' : 'radio'"
              :aria-checked="isOptionSelected(question.id, option.id)"
              @click="toggleOption(question, option.id)"
            >
              <span
                class="mr-2 flex size-4 shrink-0 items-center justify-center"
                :class="isOptionSelected(question.id, option.id) ? 'text-foreground' : 'text-muted-foreground'"
              >
                <component
                  :is="optionIcon(question, isOptionSelected(question.id, option.id))"
                  class="size-4"
                />
              </span>
              <span class="min-w-0 flex-1 break-words">{{ option.label }}</span>
            </Button>
            <Button
              v-if="question.allow_custom"
              type="button"
              size="sm"
              variant="ghost"
              class="h-auto min-h-8 w-full justify-start whitespace-normal rounded-md px-2.5 py-1.5 text-left text-xs"
              :class="isCustomSelected(question.id) ? 'bg-muted text-foreground' : 'text-foreground hover:bg-accent'"
              :role="question.kind === 'multi_select' ? 'checkbox' : 'radio'"
              :aria-checked="isCustomSelected(question.id)"
              @click="toggleCustom(question)"
            >
              <span
                class="mr-2 flex size-4 shrink-0 items-center justify-center"
                :class="isCustomSelected(question.id) ? 'text-foreground' : 'text-muted-foreground'"
              >
                <component
                  :is="optionIcon(question, isCustomSelected(question.id))"
                  class="size-4"
                />
              </span>
              <span class="min-w-0 flex-1 break-words">{{ $t('chat.tools.userInputCustomOption') }}</span>
            </Button>
          </div>
          <div
            v-if="question.kind === 'text' || isCustomSelected(question.id)"
            class="mt-1 flex items-center gap-2"
          >
            <input
              :value="draftText(question)"
              class="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :placeholder="question.placeholder || $t('chat.tools.userInputPlaceholder')"
              @input="setDraftText(question, ($event.target as HTMLInputElement).value)"
              @keydown.enter.prevent="handleSubmit"
            >
          </div>
        </div>
      </div>
    </div>
    <div class="flex items-center justify-end gap-2 border-t border-border/60 bg-card px-3 py-2">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        class="text-xs text-muted-foreground hover:text-foreground"
        @click="handleCancel"
      >
        {{ $t('chat.tools.cancelUserInput') }}
      </Button>
      <Button
        type="button"
        size="sm"
        class="text-xs"
        :disabled="!canSubmit"
        @click="handleSubmit"
      >
        {{ $t('chat.tools.submitUserInput') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Button } from '@felinic/ui'
import { Circle, CircleDot, Square, SquareCheck } from 'lucide-vue-next'
import { useChatStore } from '@/store/chat-list'
import type { UIUserInput, UIUserInputQuestion, WSUserInputAnswer } from '@/composables/api/useChat'
import { useChatViewTarget } from '../composables/useChatViewContext'

// Inline Q&A form for the ask_user tool. The parent decides WHICH pending
// user-input to render (it scans the transcript); this component owns the
// draft answers and submits/cancels straight through the chat store.
const props = defineProps<{
  userInput: UIUserInput
}>()

interface PendingUserInputDraft {
  optionIds: string[]
  customSelected: boolean
  customText: string
  text: string
}

const chatStore = useChatStore()
const chatViewTarget = useChatViewTarget()
const drafts = ref<Record<string, PendingUserInputDraft>>({})

const questions = computed(() => props.userInput.questions ?? [])

// A new request invalidates any half-filled answers from the previous one.
watch(
  () => props.userInput.user_input_id,
  () => {
    drafts.value = {}
  },
)

function ensureDraft(questionId: string): PendingUserInputDraft {
  let draft = drafts.value[questionId]
  if (!draft) {
    draft = { optionIds: [], customSelected: false, customText: '', text: '' }
    drafts.value[questionId] = draft
  }
  return draft
}

function isOptionSelected(questionId: string, optionId: string) {
  return drafts.value[questionId]?.optionIds.includes(optionId) ?? false
}

function isCustomSelected(questionId: string) {
  return drafts.value[questionId]?.customSelected ?? false
}

function optionIcon(question: UIUserInputQuestion, selected: boolean) {
  if (question.kind === 'multi_select') return selected ? SquareCheck : Square
  return selected ? CircleDot : Circle
}

function toggleOption(question: UIUserInputQuestion, optionId: string) {
  const draft = ensureDraft(question.id)
  if (question.kind === 'multi_select') {
    draft.optionIds = draft.optionIds.includes(optionId)
      ? draft.optionIds.filter(id => id !== optionId)
      : [...draft.optionIds, optionId]
    return
  }
  draft.optionIds = [optionId]
  draft.customSelected = false
  draft.customText = ''
}

function toggleCustom(question: UIUserInputQuestion) {
  const draft = ensureDraft(question.id)
  if (question.kind === 'multi_select') {
    draft.customSelected = !draft.customSelected
  } else {
    draft.customSelected = true
    draft.optionIds = []
  }
  if (!draft.customSelected) {
    draft.customText = ''
  }
}

function draftText(question: UIUserInputQuestion) {
  const draft = drafts.value[question.id]
  if (!draft) return ''
  return question.kind === 'text' ? draft.text : draft.customText
}

function setDraftText(question: UIUserInputQuestion, value: string) {
  const draft = ensureDraft(question.id)
  if (question.kind === 'text') {
    draft.text = value
    return
  }
  draft.customText = value
}

function answerFor(question: UIUserInputQuestion): WSUserInputAnswer | null {
  const draft = drafts.value[question.id]
  const customText = draft?.customSelected ? draft.customText.trim() : ''
  const text = draft?.text.trim() ?? ''
  if (!draft) return null
  if (question.kind === 'text') {
    return text ? { question_id: question.id, text } : null
  }
  if (draft.customSelected && !customText) return null
  if (question.kind === 'single_select' && draft.optionIds.length + (customText ? 1 : 0) !== 1) return null
  if (draft.optionIds.length === 0 && !customText) return null
  const answer: WSUserInputAnswer = { question_id: question.id }
  if (draft.optionIds.length > 0) answer.option_ids = [...draft.optionIds]
  if (customText) answer.custom_text = customText
  return answer
}

// All questions must be answered per kind before submit; null means incomplete.
const answers = computed<WSUserInputAnswer[] | null>(() => {
  if (!questions.value.length) return null
  const out: WSUserInputAnswer[] = []
  for (const question of questions.value) {
    const answer = answerFor(question)
    if (!answer) return null
    out.push(answer)
  }
  return out
})

const canSubmit = computed(() => answers.value !== null)

function handleSubmit() {
  if (!answers.value) return
  void chatStore.respondUserInput(props.userInput, { answers: answers.value }, chatViewTarget.value)
}

function handleCancel() {
  void chatStore.respondUserInput(props.userInput, {
    canceled: true,
    reason: 'user_canceled',
  }, chatViewTarget.value)
}
</script>
