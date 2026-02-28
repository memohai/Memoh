<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-medium">
          {{ $t('bots.skills.title') }}
        </h3>
      </div>
      <Button
        size="sm"
        @click="handleCreate"
      >
        <FontAwesomeIcon
          :icon="['fas', 'plus']"
          class="mr-2"
        />
        {{ $t('bots.skills.addSkill') }}
      </Button>
    </div>

    <!-- Loading State -->
    <div
      v-if="isLoading"
      class="flex items-center justify-center py-8 text-sm text-muted-foreground"
    >
      <Spinner class="mr-2" />
      {{ $t('common.loading') }}
    </div>

    <!-- Empty State -->
    <div
      v-else-if="!skills.length"
      class="flex flex-col items-center justify-center py-12 text-center"
    >
      <div class="rounded-full bg-muted p-3 mb-4">
        <FontAwesomeIcon
          :icon="['fas', 'bolt']"
          class="size-6 text-muted-foreground"
        />
      </div>
      <h3 class="text-lg font-medium">
        {{ $t('bots.skills.emptyTitle') }}
      </h3>
      <p class="text-sm text-muted-foreground mt-1">
        {{ $t('bots.skills.emptyDescription') }}
      </p>
    </div>

    <!-- Skills Grid -->
    <div
      v-else
      class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
    >
      <Card
        v-for="skill in skills"
        :key="skill.name"
        class="flex flex-col"
      >
        <CardHeader class="pb-3">
          <div class="flex items-start justify-between gap-2">
            <CardTitle
              class="text-base truncate"
              :title="skill.name"
            >
              {{ skill.name }}
            </CardTitle>
            <div class="flex items-center gap-1 shrink-0">
              <Button
                variant="ghost"
                size="sm"
                class="size-8 p-0"
                :title="$t('common.edit')"
                @click="handleEdit(skill)"
              >
                <FontAwesomeIcon
                  :icon="['fas', 'pen-to-square']"
                  class="size-3.5"
                />
              </Button>
              <ConfirmPopover
                :message="$t('bots.skills.deleteConfirm')"
                :loading="isDeleting && deletingName === skill.name"
                @confirm="handleDelete(skill.name)"
              >
                <template #trigger>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="size-8 p-0 text-destructive hover:text-destructive"
                    :disabled="isDeleting"
                    :title="$t('common.delete')"
                  >
                    <FontAwesomeIcon
                      :icon="['far', 'trash-can']"
                      class="size-3.5"
                    />
                  </Button>
                </template>
              </ConfirmPopover>
            </div>
          </div>
          <CardDescription
            class="line-clamp-2"
            :title="skill.description"
          >
            {{ skill.description || '-' }}
          </CardDescription>
        </CardHeader>
      </Card>
    </div>

    <!-- Edit Dialog -->
    <Dialog v-model:open="isDialogOpen">
      <DialogContent class="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{{ isEditing ? $t('common.edit') : $t('bots.skills.addSkill') }}</DialogTitle>
        </DialogHeader>
        <div class="py-4 h-[400px]">
          <MonacoEditor
            v-model="draftRaw"
            language="markdown"
            :readonly="isSaving"
          />
        </div>
        <DialogFooter>
          <DialogClose as-child>
            <Button
              variant="outline"
              :disabled="isSaving"
            >
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            :disabled="!canSave || isSaving"
            @click="handleSave"
          >
            <Spinner
              v-if="isSaving"
              class="mr-2 size-4"
            />
            {{ $t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Card, CardHeader, CardTitle, CardDescription,
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Spinner,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import {
  getBotsByBotIdContainerSkills,
  postBotsByBotIdContainerSkills,
  deleteBotsByBotIdContainerSkills,
  type HandlersSkillItem,
} from '@memoh/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const isLoading = ref(false)
const isSaving = ref(false)
const isDeleting = ref(false)
const deletingName = ref('')
const skills = ref<HandlersSkillItem[]>([])

const isDialogOpen = ref(false)
const isEditing = ref(false)
const draftRaw = ref('')

const SKILL_TEMPLATE = `---
name: my-skill
description: Brief description
---

# My Skill
`

const canSave = computed(() => {
  return draftRaw.value.trim().length > 0
})

async function fetchSkills() {
  if (!props.botId) return
  isLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerSkills({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    skills.value = data.skills || []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

function handleCreate() {
  isEditing.value = false
  draftRaw.value = SKILL_TEMPLATE
  isDialogOpen.value = true
}

function handleEdit(skill: HandlersSkillItem) {
  isEditing.value = true
  draftRaw.value = skill.raw || ''
  isDialogOpen.value = true
}

async function handleSave() {
  if (!canSave.value) return
  isSaving.value = true
  try {
    await postBotsByBotIdContainerSkills({
      path: { bot_id: props.botId },
      body: {
        skills: [draftRaw.value],
      },
      throwOnError: true,
    })
    toast.success(t('bots.skills.saveSuccess'))
    isDialogOpen.value = false
    await fetchSkills()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.saveFailed')))
  } finally {
    isSaving.value = false
  }
}

async function handleDelete(name?: string) {
  if (!name) return
  isDeleting.value = true
  deletingName.value = name
  try {
    await deleteBotsByBotIdContainerSkills({
      path: { bot_id: props.botId },
      body: {
        names: [name],
      },
      throwOnError: true,
    })
    toast.success(t('bots.skills.deleteSuccess'))
    await fetchSkills()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.deleteFailed')))
  } finally {
    isDeleting.value = false
    deletingName.value = ''
  }
}

onMounted(() => {
  fetchSkills()
})
</script>
