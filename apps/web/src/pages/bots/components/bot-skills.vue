<template>
  <div class="max-w-4xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground">
          {{ $t('bots.skills.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.skills.intro') }}
        </p>
      </div>
      <div
        v-if="skills.length > 0"
        class="flex items-center gap-2 shrink-0"
      >
        <Button
          variant="ghost"
          size="sm"
          class="h-8 text-xs font-medium px-3 shadow-none text-muted-foreground"
          :title="$t('bots.skills.discoveryTitle')"
          @click="isDiscoveryDialogOpen = true"
        >
          <SlidersHorizontal class="mr-2 size-3.5" />
          {{ $t('bots.skills.discoveryTitle') }}
          <span
            v-if="showDiscoveryIndicator"
            class="ml-1.5 inline-block size-1.5 rounded-full bg-primary"
          />
        </Button>
        <Button
          size="sm"
          class="h-8 text-xs font-medium px-4 shadow-none bg-foreground text-background hover:bg-foreground/90"
          @click="handleCreate"
        >
          <Plus class="mr-2 size-3.5" />
          {{ $t('bots.skills.addSkill') }}
        </Button>
      </div>
    </header>

    <!-- Loading State -->
    <div
      v-if="isLoading"
      class="flex items-center justify-center py-12 text-xs text-muted-foreground"
    >
      <Spinner class="mr-2 size-4" />
      {{ $t('common.loading') }}
    </div>

    <!-- Empty State -->
    <div
      v-else-if="!skills.length"
      class="flex flex-col items-center justify-center py-16 text-center border border-dashed border-border/50 bg-muted/5 rounded-lg relative overflow-hidden"
    >
      <!-- Blueprint Grid Background -->
      <div class="absolute inset-0 opacity-[0.03] pointer-events-none bg-[linear-gradient(to_right,#80808012_1px,transparent_1px),linear-gradient(to_bottom,#80808012_1px,transparent_1px)] bg-[size:24px_24px]" />
      
      <div class="relative z-10 flex flex-col items-center">
        <div class="rounded-md bg-background border border-border/50 p-2.5 mb-4 shadow-sm">
          <Zap class="size-5 text-muted-foreground" />
        </div>
        <h3 class="text-sm font-medium text-foreground">
          {{ $t('bots.skills.emptyTitle') }}
        </h3>
        <p class="text-[11px] text-muted-foreground mt-1.5 max-w-sm">
          {{ $t('bots.skills.emptyDescription') }}
        </p>
        <div class="flex items-center gap-3 mt-6">
          <Button
            size="sm"
            class="h-8 text-xs px-4 shadow-none bg-foreground text-background hover:bg-foreground/90"
            @click="handleCreate"
          >
            <Plus class="mr-2 size-3.5" />
            {{ $t('bots.skills.addSkill') }}
          </Button>
          <Button
            variant="outline"
            size="sm"
            class="h-8 text-xs px-4 shadow-none border-border/50 bg-background/60 backdrop-blur-sm"
            @click="isDiscoveryDialogOpen = true"
          >
            <SlidersHorizontal class="mr-2 size-3.5" />
            {{ $t('bots.skills.discoveryTitle') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Skills Grid -->
    <div
      v-else
      class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
    >
      <div
        v-for="skill in skills"
        :key="skillKey(skill)"
        class="rounded-md border p-3 flex flex-col justify-between min-h-[110px] group transition-colors duration-75 relative"
        :class="[
          skill.state === 'shadowed' 
            ? 'border-solid border-border/40 bg-background/40' 
            : 'border-border/60 bg-background hover:bg-accent/30 hover:border-border'
        ]"
      >
        <!-- Top Row / Header -->
        <div class="flex items-center justify-between gap-2 mb-2">
          <div class="flex items-center gap-2 min-w-0">
            <div 
              class="shrink-0"
              :class="[
                skill.state === 'disabled' || skill.state === 'shadowed' 
                  ? 'size-1.5 rounded-full border border-muted-foreground bg-transparent' 
                  : 'size-1.5 rounded-full bg-foreground'
              ]" 
            />
            <h4 
              class="font-mono text-[11px] font-semibold truncate"
              :class="[
                skill.state === 'shadowed' ? 'line-through text-muted-foreground/70' : 'text-foreground'
              ]"
              :title="skill.name"
            >
              {{ skill.name }}
            </h4>
          </div>
          
          <div class="flex items-center gap-1.5 shrink-0">
            <span
              v-if="skill.state === 'shadowed'"
              class="text-[9px] font-mono text-muted-foreground/70"
            >
              ↳ Shadowed
            </span>
            <div
              v-if="skill.managed"
              class="h-5 px-1.5 flex items-center text-[9px] uppercase tracking-wider rounded-full bg-muted/40 text-muted-foreground font-medium"
            >
              {{ $t('bots.skills.managedBadge') }}
            </div>
            <div
              v-else
              class="h-5 px-1.5 flex items-center text-[9px] uppercase tracking-wider rounded-full bg-primary/10 text-primary border border-primary/20 font-medium"
            >
              {{ $t('bots.skills.discoveredBadge') }}
            </div>
          </div>
        </div>

        <!-- Middle Row / Body -->
        <div class="mb-3 flex-1">
          <p 
            class="text-[11px] leading-snug line-clamp-2 break-words [overflow-wrap:anywhere]"
            :class="[
              skill.state === 'shadowed' ? 'text-muted-foreground/50' : 'text-muted-foreground'
            ]"
            :title="skill.description"
          >
            {{ skill.description || '-' }}
          </p>
        </div>

        <!-- Bottom Row / Footer & Action Panel -->
        <div class="flex items-center justify-between gap-2 mt-auto">
          <div 
            class="font-mono text-[9px] truncate px-1.5 py-0.5 rounded bg-muted/30"
            :class="skill.state === 'shadowed' ? 'text-muted-foreground/50' : 'text-muted-foreground'"
            :title="sourceSummary(skill)"
          >
            {{ sourceSummary(skill) }}
          </div>
          
          <!-- Actions (Revealed on Hover) -->
          <div class="flex items-center gap-0.5 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
            <!-- Edit / View -->
            <Button
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
              :title="!skill.managed ? $t('bots.skills.overrideTitle') : $t('common.edit')"
              @click="handleEdit(skill)"
            >
              <SquarePen class="size-3" />
            </Button>
            
            <!-- Enable / Disable -->
            <Button
              v-if="skill.state === 'disabled'"
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
              :disabled="isActioning"
              :title="$t('bots.skills.enableAction')"
              @click="handleSkillAction('enable', skill)"
            >
              <Spinner
                v-if="isSkillActionPending(skill, 'enable')"
                class="size-3"
              />
              <EyeOff
                v-else
                class="size-3"
              />
            </Button>
            <Button
              v-else
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
              :disabled="isActioning"
              :title="$t('bots.skills.disableAction')"
              @click="handleSkillAction('disable', skill)"
            >
              <Spinner
                v-if="isSkillActionPending(skill, 'disable')"
                class="size-3"
              />
              <Eye
                v-else
                class="size-3"
              />
            </Button>

            <!-- Adopt -->
            <Button
              v-if="!skill.managed"
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 text-muted-foreground hover:text-primary transition-colors"
              :disabled="isActioning || skill.state === 'shadowed'"
              :title="skill.state === 'shadowed' ? $t('bots.skills.adoptBlocked') : $t('bots.skills.adoptAction')"
              @click="handleSkillAction('adopt', skill)"
            >
              <Spinner
                v-if="isSkillActionPending(skill, 'adopt')"
                class="size-3"
              />
              <ArrowDownToLine
                v-else
                class="size-3"
              />
            </Button>

            <!-- Delete -->
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
                  class="h-6 w-6 p-0 text-muted-foreground hover:text-destructive transition-colors"
                  :disabled="isDeleting"
                  :title="$t('common.delete')"
                >
                  <Trash2 class="size-3" />
                </Button>
              </template>
            </ConfirmPopover>
          </div>
        </div>
      </div>
    </div>

    <!-- Edit Dialog (Modal IDE) -->
    <Dialog v-model:open="isDialogOpen">
      <DialogContent class="sm:max-w-4xl max-h-[calc(100vh-2rem)] sm:h-[85vh] flex flex-col overflow-hidden p-0 gap-0">
        <DialogHeader class="shrink-0 p-4 border-b border-border/50 bg-background">
          <DialogTitle class="text-sm font-semibold">
            {{ isEditing ? $t('common.edit') : $t('bots.skills.addSkill') }}
          </DialogTitle>
        </DialogHeader>
        
        <div class="flex-1 min-h-0 relative p-4 bg-muted/5">
          <div class="absolute inset-4 rounded-md border border-border/50 bg-background/50 overflow-hidden flex flex-col shadow-sm">
            <MonacoEditor
              v-model="draftRaw"
              language="markdown"
              :readonly="isSaving"
              class="flex-1 min-h-0"
              :options="{
                automaticLayout: true,
                fixedOverflowWidgets: true,
                minimap: { enabled: false },
                scrollBeyondLastLine: false
              }"
            />
          </div>
        </div>

        <DialogFooter class="shrink-0 p-4 border-t border-border/50 bg-background flex items-center justify-end gap-2">
          <DialogClose as-child>
            <Button
              variant="ghost"
              size="sm"
              class="h-8 text-xs font-medium px-4 shadow-none"
              :disabled="isSaving"
            >
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            size="sm"
            class="h-8 text-xs font-medium px-4 min-w-24 shadow-none bg-foreground text-background hover:bg-foreground/90"
            :disabled="!canSave || isSaving"
            @click="handleSave"
          >
            <Spinner
              v-if="isSaving"
              class="mr-2 size-3.5"
            />
            {{ $t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Discovery Modal -->
    <Dialog v-model:open="isDiscoveryDialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle class="text-sm font-semibold">
            {{ $t('bots.skills.discoveryTitle') }}
          </DialogTitle>
          <DialogDescription class="text-[11px] leading-snug">
            {{ $t('bots.skills.discoveryDescription') }}
          </DialogDescription>
        </DialogHeader>

        <div class="space-y-5 py-3">
          <div class="space-y-1.5">
            <Label class="text-[11px] font-medium text-foreground">
              {{ $t('bots.skills.managedPathLabel') }}
            </Label>
            <div class="rounded border border-border/50 bg-muted/20 px-2.5 py-1.5 font-mono text-[10px] text-muted-foreground break-all">
              {{ MANAGED_SKILL_PATH }}
            </div>
            <p class="text-[10px] text-muted-foreground">
              {{ $t('bots.skills.managedPathHint') }}
            </p>
          </div>

          <div class="space-y-1.5">
            <Label class="text-[11px] font-medium text-foreground">
              {{ $t('bots.skills.discoveryPathsLabel') }}
            </Label>
            <Textarea
              v-model="discoveryRootsDraft"
              :disabled="discoveryControlsDisabled"
              :placeholder="$t('bots.skills.discoveryPathPlaceholder')"
              class="min-h-24 font-mono text-[11px] bg-muted/10 border-border/50 shadow-none"
              :class="{ 'border-warning/40 focus-visible:ring-warning/30': hasDiscoveryRootErrors }"
            />
            <p
              v-if="discoveryRootError"
              class="text-[10px] text-warning/80"
            >
              {{ discoveryRootError }}
            </p>
            <p
              v-else
              class="text-[10px] text-muted-foreground"
            >
              {{ $t('bots.skills.discoveryDefaultHint', { paths: DEFAULT_DISCOVERY_ROOTS.join(', ') }) }}
            </p>
          </div>
        </div>

        <DialogFooter class="gap-2 sm:space-x-0">
          <Button
            variant="ghost"
            size="sm"
            class="h-8 text-[11px] px-3 shadow-none text-muted-foreground"
            :disabled="discoveryControlsDisabled || !isDiscoveryRootsDirty"
            @click="resetDiscoveryRoots"
          >
            {{ $t('bots.skills.discoveryReset') }}
          </Button>
          <div class="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              class="h-8 text-xs font-medium px-4 shadow-none"
              :disabled="isSavingDiscoveryRoots"
              @click="closeDiscoveryDialog"
            >
              {{ $t('common.cancel') }}
            </Button>
            <Button
              size="sm"
              class="h-8 text-xs font-medium px-4 min-w-24 shadow-none bg-foreground text-background hover:bg-foreground/90"
              :disabled="!canSaveDiscoveryRoots"
              @click="handleSaveDiscoveryRoots"
            >
              <Spinner
                v-if="isSavingDiscoveryRoots"
                class="mr-2 size-3.5"
              />
              {{ $t('common.save') }}
            </Button>
          </div>
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
  Button,
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
