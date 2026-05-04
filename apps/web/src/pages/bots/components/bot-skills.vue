<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-sm font-medium">
          {{ $t('bots.skills.title') }}
        </h3>
      </div>
      <div class="flex items-center gap-1">
        <Button
          variant="ghost"
          size="sm"
          class="text-muted-foreground"
          :title="$t('bots.skills.discoveryTitle')"
          @click="isDiscoveryDialogOpen = true"
        >
          <SlidersHorizontal class="mr-2 size-4" />
          {{ $t('bots.skills.discoveryTitle') }}
          <span
            v-if="showDiscoveryIndicator"
            class="ml-2 inline-block size-2 shrink-0 rounded-full bg-primary/80"
          />
        </Button>
        <Button
          size="sm"
          @click="handleCreate"
        >
          <Plus
            class="mr-2"
          />
          {{ $t('bots.skills.addSkill') }}
        </Button>
      </div>
    </div>

    <!-- Loading State -->
    <div
      v-if="isLoading"
      class="flex items-center justify-center py-8 text-xs text-muted-foreground"
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
        <Zap
          class="size-6 text-muted-foreground"
        />
      </div>
      <h3 class="text-sm font-medium">
        {{ $t('bots.skills.emptyTitle') }}
      </h3>
      <p class="text-xs text-muted-foreground mt-1">
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
        :key="skillKey(skill)"
        class="flex min-w-0 flex-col overflow-hidden"
      >
        <CardHeader class="min-w-0 pb-3">
          <div class="flex min-w-0 items-center justify-between gap-2">
            <CardTitle
              class="min-w-0 flex-1 truncate text-sm"
              :title="skill.name"
            >
              {{ skill.name }}
            </CardTitle>
            <div class="flex items-center gap-1 shrink-0">
              <Button
                variant="ghost"
                size="sm"
                class="size-8 p-0"
                :title="!skill.managed ? $t('bots.skills.overrideTitle') : $t('common.edit')"
                @click="handleEdit(skill)"
              >
                <SquarePen
                  class="size-3.5"
                />
              </Button>
              <Button
                v-if="skill.state === 'disabled'"
                variant="ghost"
                size="sm"
                class="size-8 p-0"
                :disabled="isActioning"
                :title="$t('bots.skills.enableAction')"
                @click="handleSkillAction('enable', skill)"
              >
                <Spinner
                  v-if="isSkillActionPending(skill, 'enable')"
                  class="size-3.5"
                />
                <EyeOff
                  v-else
                  class="size-3.5"
                />
              </Button>
              <Button
                v-else
                variant="ghost"
                size="sm"
                class="size-8 p-0"
                :disabled="isActioning"
                :title="$t('bots.skills.disableAction')"
                @click="handleSkillAction('disable', skill)"
              >
                <Spinner
                  v-if="isSkillActionPending(skill, 'disable')"
                  class="size-3.5"
                />
                <Eye
                  v-else
                  class="size-3.5"
                />
              </Button>
              <Button
                v-if="!skill.managed"
                variant="ghost"
                size="sm"
                class="size-8 p-0"
                :disabled="isActioning || skill.state === 'shadowed'"
                :title="skill.state === 'shadowed' ? $t('bots.skills.adoptBlocked') : $t('bots.skills.adoptAction')"
                @click="handleSkillAction('adopt', skill)"
              >
                <Spinner
                  v-if="isSkillActionPending(skill, 'adopt')"
                  class="size-3.5"
                />
                <ArrowDownToLine
                  v-else
                  class="size-3.5"
                />
              </Button>
              <ConfirmPopover
                v-if="skill.managed"
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
                    <Trash2
                      class="size-3.5"
                    />
                  </Button>
                </template>
              </ConfirmPopover>
            </div>
          </div>
          <CardDescription
            class="min-w-0 overflow-hidden line-clamp-2 break-words [overflow-wrap:anywhere]"
            :title="skill.description"
          >
            {{ skill.description || '-' }}
          </CardDescription>
        </CardHeader>
        <CardContent class="mt-auto min-w-0 space-y-1.5 pt-0">
          <div class="flex flex-wrap items-center gap-1.5">
            <Badge
              variant="secondary"
              size="sm"
              class="rounded-full"
            >
              {{ skill.managed ? $t('bots.skills.managedBadge') : $t('bots.skills.discoveredBadge') }}
            </Badge>
            <Badge
              variant="outline"
              size="sm"
              class="rounded-full"
            >
              {{ stateLabel(skill.state) }}
            </Badge>
          </div>
          <p
            v-if="skill.shadowed_by"
            class="text-[11px] text-muted-foreground truncate"
            :title="skill.shadowed_by"
          >
            {{ $t('bots.skills.shadowedBy') }} {{ skill.shadowed_by }}
          </p>
          <p
            v-if="skill.source_path"
            class="text-[11px] text-muted-foreground truncate"
            :title="sourceSummary(skill)"
          >
            {{ sourceSummary(skill) }}
          </p>
        </CardContent>
      </Card>
    </div>

    <!-- Edit Dialog -->
    <Dialog v-model:open="isDialogOpen">
      <DialogContent class="sm:max-w-2xl max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden">
        <DialogHeader class="shrink-0">
          <DialogTitle>{{ isEditing ? $t('common.edit') : $t('bots.skills.addSkill') }}</DialogTitle>
        </DialogHeader>
        <div class="basis-[400px] flex-1 min-h-0 py-4">
          <div class="h-full rounded-md border overflow-hidden">
            <MonacoEditor
              v-model="draftRaw"
              language="markdown"
              :readonly="isSaving"
            />
          </div>
        </div>
        <DialogFooter class="shrink-0">
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

    <Dialog v-model:open="isDiscoveryDialogOpen">
      <DialogContent class="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.skills.discoveryTitle') }}</DialogTitle>
          <DialogDescription class="text-xs">
            {{ $t('bots.skills.discoveryDescription') }}
          </DialogDescription>
        </DialogHeader>

        <div class="space-y-4 py-2">
          <div class="space-y-2">
            <Label class="text-xs font-medium">
              {{ $t('bots.skills.managedPathLabel') }}
            </Label>
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.skills.managedPathDescription') }}
            </p>
            <div class="rounded-md border bg-muted/30 px-3 py-2 font-mono text-xs text-foreground break-all">
              {{ MANAGED_SKILL_PATH }}
            </div>
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.skills.managedPathHint') }}
            </p>
          </div>

          <div class="space-y-2">
            <Label class="text-xs font-medium">
              {{ $t('bots.skills.discoveryPathsLabel') }}
            </Label>
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.skills.discoveryPathsDescription') }}
            </p>
            <Textarea
              v-model="discoveryRootsDraft"
              :disabled="discoveryControlsDisabled"
              :placeholder="$t('bots.skills.discoveryPathPlaceholder')"
              class="min-h-32 font-mono text-xs"
            />
            <p
              v-if="discoveryRootError"
              class="text-xs text-destructive"
            >
              {{ discoveryRootError }}
            </p>
          </div>

          <p class="text-xs text-muted-foreground">
            {{ $t('bots.skills.discoveryDefaultHint', { paths: DEFAULT_DISCOVERY_ROOTS.join(', ') }) }}
          </p>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            :disabled="discoveryControlsDisabled || !isDiscoveryRootsDirty"
            @click="resetDiscoveryRoots"
          >
            {{ $t('bots.skills.discoveryReset') }}
          </Button>
          <Button
            variant="outline"
            :disabled="isSavingDiscoveryRoots"
            @click="closeDiscoveryDialog"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            :disabled="!canSaveDiscoveryRoots"
            @click="handleSaveDiscoveryRoots"
          >
            <Spinner
              v-if="isSavingDiscoveryRoots"
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
import { ArrowDownToLine, Eye, EyeOff, Plus, SlidersHorizontal, Zap, SquarePen, Trash2 } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Badge, Button, Card, CardContent, CardHeader, CardTitle, CardDescription,
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose,
  Label, Spinner, Textarea,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import MonacoEditor from '@/components/monaco-editor/index.vue'
import {
  getBotsById,
  getBotsByBotIdContainerSkills,
  postBotsByBotIdContainerSkills,
  postBotsByBotIdContainerSkillsActions,
  deleteBotsByBotIdContainerSkills,
  putBotsById,
  type HandlersSkillItem,
} from '@memohai/sdk'
import { getBotsQueryKey } from '@memohai/sdk/colada'
import { resolveApiErrorMessage } from '@/utils/api-error'

type SkillItem = HandlersSkillItem & {
  source_path?: string
  source_root?: string
  source_kind?: string
  managed?: boolean
  state?: string
  shadowed_by?: string
}

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

function invalidateSidebarSkills() {
  queryCache.invalidateQueries({ key: ['bot-skills-catalog', props.botId] })
}

const MANAGED_SKILL_PATH = '/data/skills'
const DEFAULT_DISCOVERY_ROOTS = ['/data/.agents/skills', '/root/.agents/skills']
const RESERVED_DISCOVERY_ROOTS = new Set(['/data/skills', '/data/.skills'])
const WORKSPACE_METADATA_KEY = 'workspace'
const SKILL_DISCOVERY_ROOTS_METADATA_KEY = 'skill_discovery_roots'

const isLoading = ref(false)
const isSaving = ref(false)
const isDeleting = ref(false)
const deletingName = ref('')
const isActioning = ref(false)
const actionTargetPath = ref('')
const actionName = ref('')
const skills = ref<SkillItem[]>([])
const isSavingDiscoveryRoots = ref(false)
const isDiscoveryDialogOpen = ref(false)
const discoveryRootsDraft = ref(DEFAULT_DISCOVERY_ROOTS.join('\n'))
const savedDiscoveryRoots = ref<string[]>([...DEFAULT_DISCOVERY_ROOTS])

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

const { data: bot, refetch: refetchBot } = useQuery({
  key: () => ['bot', props.botId],
  query: async () => {
    const { data } = await getBotsById({ path: { id: props.botId }, throwOnError: true })
    return data
  },
  enabled: () => !!props.botId,
})

const discoveryRootErrors = computed(() => validateDiscoveryRoots(discoveryRootsDraft.value))
const discoveryRootError = computed(() => discoveryRootErrors.value[0] || '')
const hasDiscoveryRootErrors = computed(() => discoveryRootErrors.value.length > 0)
const normalizedDiscoveryRootDrafts = computed(() => normalizeDiscoveryRoots(parseDiscoveryRoots(discoveryRootsDraft.value)))
const isDiscoveryRootsDirty = computed(() => !areStringListsEqual(normalizedDiscoveryRootDrafts.value, savedDiscoveryRoots.value))
const savedDiscoveryRootsText = computed(() => savedDiscoveryRoots.value.join('\n'))
const isDiscoveryDraftModified = computed(() => discoveryRootsDraft.value !== savedDiscoveryRootsText.value)
const usesDefaultDiscoveryRoots = computed(() => areStringListsEqual(savedDiscoveryRoots.value, DEFAULT_DISCOVERY_ROOTS))
const showDiscoveryIndicator = computed(() => !usesDefaultDiscoveryRoots.value || isDiscoveryRootsDirty.value)
const discoveryControlsDisabled = computed(() => isSavingDiscoveryRoots.value || !bot.value)
const canSaveDiscoveryRoots = computed(() => {
  return !!bot.value && isDiscoveryRootsDirty.value && !hasDiscoveryRootErrors.value && !isSavingDiscoveryRoots.value
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

function cleanDiscoveryRoot(value: string) {
  const trimmed = value.trim()
  if (!trimmed.startsWith('/')) {
    return trimmed
  }

  const parts = trimmed.split('/')
  const stack: string[] = []
  for (const part of parts) {
    if (!part || part === '.') continue
    if (part === '..') {
      stack.pop()
      continue
    }
    stack.push(part)
  }
  return `/${stack.join('/')}`
}

function parseDiscoveryRoots(value: string) {
  return value
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean)
}

function normalizeDiscoveryRoots(values: string[]) {
  const normalized: string[] = []
  const seen = new Set<string>()

  for (const value of values) {
    const cleaned = cleanDiscoveryRoot(value)
    if (!cleaned || !cleaned.startsWith('/')) continue
    if (RESERVED_DISCOVERY_ROOTS.has(cleaned) || seen.has(cleaned)) continue
    seen.add(cleaned)
    normalized.push(cleaned)
  }

  return normalized
}

function validateDiscoveryRoots(value: string) {
  const seen = new Set<string>()
  const errors: string[] = []

  for (const item of parseDiscoveryRoots(value)) {
    const trimmed = item.trim()

    const cleaned = cleanDiscoveryRoot(trimmed)
    if (!cleaned.startsWith('/')) {
      errors.push(t('bots.skills.discoveryPathAbsolute'))
      continue
    }
    if (RESERVED_DISCOVERY_ROOTS.has(cleaned)) {
      errors.push(t('bots.skills.discoveryPathReserved'))
      continue
    }
    if (seen.has(cleaned)) {
      errors.push(t('bots.skills.discoveryPathDuplicate'))
      continue
    }

    seen.add(cleaned)
  }

  return [...new Set(errors)]
}

function areStringListsEqual(left: string[], right: string[]) {
  return left.length === right.length && left.every((item, index) => item === right[index])
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function readDiscoveryRoots(metadata: Record<string, unknown> | undefined) {
  const workspace = metadata?.[WORKSPACE_METADATA_KEY]
  if (!isRecord(workspace)) {
    return [...DEFAULT_DISCOVERY_ROOTS]
  }

  if (!Object.prototype.hasOwnProperty.call(workspace, SKILL_DISCOVERY_ROOTS_METADATA_KEY)) {
    return [...DEFAULT_DISCOVERY_ROOTS]
  }

  const rawRoots = workspace[SKILL_DISCOVERY_ROOTS_METADATA_KEY]
  if (!Array.isArray(rawRoots)) {
    return []
  }

  return normalizeDiscoveryRoots(
    rawRoots.filter((value): value is string => typeof value === 'string'),
  )
}

function withDiscoveryRootsMetadata(metadata: Record<string, unknown> | undefined, roots: string[]) {
  const nextMetadata = isRecord(metadata) ? { ...metadata } : {}
  const workspaceSection = isRecord(nextMetadata[WORKSPACE_METADATA_KEY])
    ? { ...(nextMetadata[WORKSPACE_METADATA_KEY] as Record<string, unknown>) }
    : {}

  workspaceSection[SKILL_DISCOVERY_ROOTS_METADATA_KEY] = normalizeDiscoveryRoots(roots)
  nextMetadata[WORKSPACE_METADATA_KEY] = workspaceSection
  return nextMetadata
}

function syncDiscoveryRoots(roots: string[]) {
  const nextRoots = [...roots]
  discoveryRootsDraft.value = nextRoots.join('\n')
  savedDiscoveryRoots.value = nextRoots
}

function resetDiscoveryRoots() {
  syncDiscoveryRoots(savedDiscoveryRoots.value)
}

function closeDiscoveryDialog() {
  resetDiscoveryRoots()
  isDiscoveryDialogOpen.value = false
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

function skillKey(skill: SkillItem) {
  return skill.source_path || `${skill.name || 'unknown'}:${skill.source_kind || 'unknown'}`
}

function isSkillActionPending(skill: SkillItem, action: string) {
  return isActioning.value && actionTargetPath.value === skill.source_path && actionName.value === action
}

function sourceKindLabel(kind?: string) {
  switch (kind) {
    case 'legacy':
      return t('bots.skills.legacyBadge')
    case 'compat':
      return t('bots.skills.compatBadge')
    default:
      return t('bots.skills.managedBadge')
  }
}

function sourceSummary(skill: SkillItem) {
  const sourcePath = skill.source_path || ''
  if (!sourcePath) return ''
  if (!skill.source_kind || skill.source_kind === 'managed') {
    return sourcePath
  }
  return `${sourceKindLabel(skill.source_kind)} · ${sourcePath}`
}

function stateLabel(state?: string) {
  switch (state) {
    case 'disabled':
      return t('bots.skills.disabledBadge')
    case 'shadowed':
      return t('bots.skills.shadowedBadge')
    default:
      return t('bots.skills.effectiveBadge')
  }
}

async function handleSkillAction(action: 'adopt' | 'disable' | 'enable', skill: SkillItem) {
  if (!skill.source_path) return
  isActioning.value = true
  actionTargetPath.value = skill.source_path
  actionName.value = action
  try {
    await postBotsByBotIdContainerSkillsActions({
      path: { bot_id: props.botId },
      body: {
        action,
        target_path: skill.source_path,
      },
      throwOnError: true,
    })
    toast.success(
      action === 'adopt'
        ? t('bots.skills.adoptSuccess')
        : action === 'disable'
          ? t('bots.skills.disableSuccess')
          : t('bots.skills.enableSuccess'),
    )
    await fetchSkills()
    invalidateSidebarSkills()
  } catch (error) {
    toast.error(resolveApiErrorMessage(
      error,
      action === 'adopt'
        ? t('bots.skills.adoptFailed')
        : action === 'disable'
          ? t('bots.skills.disableFailed')
          : t('bots.skills.enableFailed'),
    ))
  } finally {
    isActioning.value = false
    actionTargetPath.value = ''
    actionName.value = ''
  }
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
    invalidateSidebarSkills()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.saveFailed')))
  } finally {
    isSaving.value = false
  }
}

async function handleSaveDiscoveryRoots() {
  if (!canSaveDiscoveryRoots.value) return

  isSavingDiscoveryRoots.value = true
  try {
    const metadata = withDiscoveryRootsMetadata(
      bot.value?.metadata as Record<string, unknown> | undefined,
      normalizedDiscoveryRootDrafts.value,
    )

    await putBotsById({
      path: { id: props.botId },
      body: { metadata },
      throwOnError: true,
    })

    void queryCache.invalidateQueries({ key: ['bot', props.botId] })
    void queryCache.invalidateQueries({ key: ['bot'] })
    void queryCache.invalidateQueries({ key: getBotsQueryKey() })

    syncDiscoveryRoots(normalizedDiscoveryRootDrafts.value)
    isDiscoveryDialogOpen.value = false
    toast.success(t('bots.skills.discoverySaveSuccess'))

    await Promise.all([
      refetchBot(),
      fetchSkills(),
    ])
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.discoverySaveFailed')))
  } finally {
    isSavingDiscoveryRoots.value = false
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
    invalidateSidebarSkills()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.skills.deleteFailed')))
  } finally {
    isDeleting.value = false
    deletingName.value = ''
  }
}

watch(() => props.botId, () => {
  if (!props.botId) return
  isDiscoveryDialogOpen.value = false
  syncDiscoveryRoots(DEFAULT_DISCOVERY_ROOTS)
  void fetchSkills()
}, { immediate: true })

// Refresh local skills list when chat-sidebar invalidates the shared catalog cache.
watch(
  () => {
    const entries = queryCache.getEntries({ key: ['bot-skills-catalog', props.botId] })
    return entries[0]?.state.value.data
  },
  (next, prev) => {
    if (!props.botId) return
    if (next === prev) return
    void fetchSkills()
  },
)

watch(bot, (value) => {
  if (!value) return
  if (isDiscoveryRootsDirty.value && !isSavingDiscoveryRoots.value) return
  syncDiscoveryRoots(readDiscoveryRoots(value.metadata as Record<string, unknown> | undefined))
}, { immediate: true })

watch(isDiscoveryDialogOpen, (open, prevOpen) => {
  if (!open && prevOpen && !isSavingDiscoveryRoots.value && (isDiscoveryDraftModified.value || hasDiscoveryRootErrors.value)) {
    resetDiscoveryRoots()
  }
})
</script>
