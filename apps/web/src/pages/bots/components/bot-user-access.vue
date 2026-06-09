<template>
  <section class="space-y-4 rounded-md border p-4 bg-background/50">
    <div class="flex items-center justify-between gap-3">
      <div class="space-y-1">
        <h3 class="text-xs font-medium text-foreground">
          {{ $t('bots.access.userAccess.title') }}
        </h3>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.access.userAccess.subtitle') }}
        </p>
      </div>
      <Button
        v-if="!formVisible"
        size="sm"
        variant="outline"
        class="h-8 text-[11px] font-medium px-3 shrink-0 shadow-none"
        @click="openAddForm"
      >
        <Plus class="mr-1.5 size-3.5" />
        {{ $t('bots.access.userAccess.addMember') }}
      </Button>
    </div>

    <!-- Add form -->
    <div
      v-if="formVisible"
      class="space-y-4 rounded-lg border border-border/60 bg-background/70 p-4"
    >
      <div class="space-y-2">
        <Label class="text-[11px] text-muted-foreground">
          {{ $t('bots.access.userAccess.subjectQuestion') }}
        </Label>
        <div class="grid grid-cols-2 gap-2">
          <button
            type="button"
            class="flex items-center gap-2 rounded-md border px-3 py-2 text-left text-xs transition-colors shadow-none"
            :class="formSubjectType === 'everyone'
              ? 'border-foreground bg-muted text-foreground'
              : 'border-border/60 bg-background/70 hover:bg-accent/50'"
            @click="formSubjectType = 'everyone'"
          >
            <Globe class="size-4 shrink-0" />
            <span class="truncate">{{ $t('bots.access.userAccess.everyone') }}</span>
          </button>
          <button
            type="button"
            class="flex items-center gap-2 rounded-md border px-3 py-2 text-left text-xs transition-colors shadow-none"
            :class="formSubjectType === 'user'
              ? 'border-foreground bg-muted text-foreground'
              : 'border-border/60 bg-background/70 hover:bg-accent/50'"
            @click="formSubjectType = 'user'"
          >
            <User class="size-4 shrink-0" />
            <span class="truncate">{{ $t('bots.access.userAccess.specificMember') }}</span>
          </button>
        </div>
      </div>

      <div
        v-if="formSubjectType === 'user'"
        class="space-y-2"
      >
        <Label class="text-[11px] text-muted-foreground">
          {{ $t('bots.access.userAccess.memberQuestion') }}
        </Label>
        <SearchableSelectPopover
          v-model="formUserId"
          :options="candidateOptions"
          :placeholder="$t('bots.access.userAccess.selectMember')"
          :search-placeholder="$t('bots.access.userAccess.searchMember')"
          :empty-text="$t('bots.access.userAccess.noMemberCandidates')"
          :show-group-headers="false"
        />
      </div>

      <div class="space-y-2">
        <Label class="text-[11px] text-muted-foreground">
          {{ $t('bots.access.userAccess.permissionsQuestion') }}
        </Label>
        <div class="flex flex-wrap gap-4">
          <label
            v-for="permission in permissionOptions"
            :key="permission"
            class="flex items-center gap-2 text-xs cursor-pointer"
          >
            <Checkbox
              :model-value="formPermissions[permission]"
              @update:model-value="(v) => setFormPermission(permission, v === true)"
            />
            {{ permissionLabel(permission) }}
          </label>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 pt-1">
        <Button
          variant="ghost"
          size="sm"
          class="h-8 text-[11px] shadow-none"
          @click="closeForm"
        >
          {{ $t('common.cancel') }}
        </Button>
        <Button
          size="sm"
          class="h-8 text-[11px] shadow-none"
          :disabled="!canSubmit || isSaving"
          @click="handleCreate"
        >
          <Spinner
            v-if="isSaving"
            class="mr-1.5 size-3.5"
          />
          {{ $t('common.save') }}
        </Button>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="isLoading"
      class="flex justify-center py-8"
    >
      <Spinner class="size-5 text-muted-foreground/50" />
    </div>

    <!-- List -->
    <div
      v-else
      class="space-y-2"
    >
      <div
        v-for="grant in grants"
        :key="grant.id || grant.subject_type + (grant.user_id || 'everyone')"
        class="flex items-center gap-3 rounded-lg border border-border/60 bg-background/70 px-3 py-2.5"
      >
        <div class="flex min-w-0 flex-1 items-center gap-2.5">
          <Avatar class="size-7 shrink-0">
            <AvatarImage
              v-if="grant.subject_type === 'user' && grant.user_avatar_url"
              :src="grant.user_avatar_url"
            />
            <AvatarFallback class="bg-muted text-muted-foreground">
              <Globe
                v-if="grant.subject_type === 'everyone'"
                class="size-3.5"
              />
              <span
                v-else
                class="text-[10px]"
              >{{ initials(grant) }}</span>
            </AvatarFallback>
          </Avatar>
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <span class="truncate text-xs font-medium text-foreground">
                {{ grantLabel(grant) }}
              </span>
              <Badge
                v-if="grant.is_owner"
                variant="secondary"
                class="h-4 px-1.5 text-[9px] font-medium"
              >
                {{ $t('bots.access.userAccess.ownerBadge') }}
              </Badge>
            </div>
            <p
              v-if="grant.subject_type === 'user' && grant.user_username"
              class="truncate text-[10px] text-muted-foreground"
            >
              @{{ grant.user_username }}
            </p>
          </div>
        </div>

        <div class="flex items-center gap-3 shrink-0">
          <label
            v-for="permission in permissionOptions"
            :key="permission"
            class="flex items-center gap-1.5 text-[11px]"
            :class="grant.is_owner ? 'text-muted-foreground' : 'cursor-pointer text-foreground'"
          >
            <Checkbox
              :model-value="hasPerm(grant, permission)"
              :disabled="grant.is_owner || isRowBusy(grant)"
              @update:model-value="() => togglePerm(grant, permission)"
            />
            {{ permissionLabel(permission) }}
          </label>

          <ConfirmPopover
            v-if="!grant.is_owner"
            :title="$t('bots.access.userAccess.removeConfirm')"
            @confirm="() => handleDelete(grant)"
          >
            <Button
              variant="ghost"
              size="icon"
              class="size-7 text-muted-foreground hover:text-destructive shadow-none"
              :disabled="isRowBusy(grant)"
            >
              <Trash2 class="size-3.5" />
            </Button>
          </ConfirmPopover>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Plus, Trash2, Globe, User } from 'lucide-vue-next'
import { toast } from 'vue-sonner'
import {
  Button,
  Label,
  Checkbox,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Spinner,
  Badge,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { BOT_PERMISSION_ORDER, expandBotPermissions, type BotPermission } from '@/utils/bot-permissions'
import {
  getBotsByBotIdUserAccess,
  getBotsByBotIdUserAccessCandidates,
  postBotsByBotIdUserAccess,
  putBotsByBotIdUserAccessByGrantId,
  deleteBotsByBotIdUserAccessByGrantId,
} from '@memohai/sdk'
import type { BotsUserGrant, HandlersBotUserCandidate } from '@memohai/sdk'

type Permission = BotPermission

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: grantsData, isLoading } = useQuery({
  key: () => ['bot-user-access', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdUserAccess({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const grants = computed<BotsUserGrant[]>(() => grantsData.value?.items ?? [])

const formVisible = ref(false)
const formSubjectType = ref<'user' | 'everyone'>('user')
const formUserId = ref('')
const formPermissions = reactive<Record<Permission, boolean>>({
  chat: true,
  workspace_read: false,
  workspace_write: false,
  workspace_exec: false,
  manage: false,
})
const isSaving = ref(false)
const busyGrantIds = ref<Set<string>>(new Set())
const permissionOptions = BOT_PERMISSION_ORDER

const { data: candidatesData } = useQuery({
  key: () => ['bot-user-access-candidates', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdUserAccessCandidates({
      path: { bot_id: props.botId },
      query: { limit: 200 },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId && formVisible.value,
})

const candidateOptions = computed<SearchableSelectOption[]>(() => {
  const taken = new Set(
    grants.value
      .filter((g) => g.subject_type === 'user' && g.user_id)
      .map((g) => g.user_id as string),
  )
  return (candidatesData.value?.items ?? [])
    .filter((c: HandlersBotUserCandidate) => c.id && !taken.has(c.id))
    .map((c: HandlersBotUserCandidate) => ({
      value: c.id ?? '',
      label: c.display_name || c.username || (c.id ?? ''),
      description: c.username ? `@${c.username}` : undefined,
      keywords: [c.username ?? '', c.display_name ?? ''],
    }))
})

const everyoneExists = computed(() => grants.value.some((g) => g.subject_type === 'everyone'))

const canSubmit = computed(() => {
  if (buildPermissions().length === 0) return false
  if (formSubjectType.value === 'everyone') return !everyoneExists.value
  return !!formUserId.value
})

function openAddForm() {
  formVisible.value = true
  formSubjectType.value = everyoneExists.value ? 'user' : 'user'
  formUserId.value = ''
  formPermissions.chat = true
  formPermissions.workspace_read = false
  formPermissions.workspace_write = false
  formPermissions.workspace_exec = false
  formPermissions.manage = false
}

function closeForm() {
  formVisible.value = false
  formUserId.value = ''
}

function buildPermissions(): string[] {
  const selected = new Set<Permission>()
  for (const permission of permissionOptions) {
    if (formPermissions[permission]) selected.add(permission)
  }
  return normalizePermissionSelection(selected)
}

function setFormPermission(permission: Permission, checked: boolean) {
  if (permission !== 'manage' && !checked && formPermissions.manage) {
    formPermissions.manage = false
  }
  formPermissions[permission] = checked
  if (permission === 'manage' && checked) {
    for (const item of permissionOptions) formPermissions[item] = true
  }
  if (permission === 'workspace_write' && checked) {
    formPermissions.workspace_read = true
  }
  if (permission === 'workspace_read' && !checked) {
    formPermissions.workspace_write = false
  }
}

function normalizePermissionSelection(selected: Set<Permission>): Permission[] {
  if (selected.has('manage')) {
    for (const permission of permissionOptions) selected.add(permission)
  }
  if (selected.has('workspace_write')) selected.add('workspace_read')
  return permissionOptions.filter(permission => selected.has(permission))
}

function permissionLabel(permission: Permission): string {
  switch (permission) {
    case 'chat': return t('bots.access.userAccess.permissionChat')
    case 'workspace_read': return t('bots.access.userAccess.permissionWorkspaceRead')
    case 'workspace_write': return t('bots.access.userAccess.permissionWorkspaceWrite')
    case 'workspace_exec': return t('bots.access.userAccess.permissionWorkspaceExec')
    case 'manage': return t('bots.access.userAccess.permissionManage')
  }
}

function initials(grant: BotsUserGrant): string {
  const name = grant.user_display_name || grant.user_username || '?'
  return name.trim().charAt(0).toUpperCase()
}

function grantLabel(grant: BotsUserGrant): string {
  if (grant.subject_type === 'everyone') return t('bots.access.userAccess.everyone')
  return grant.user_display_name || grant.user_username || grant.user_id || ''
}

function hasPerm(grant: BotsUserGrant, perm: Permission): boolean {
  return expandBotPermissions(grant.permissions).includes(perm)
}

function isRowBusy(grant: BotsUserGrant): boolean {
  return !!grant.id && busyGrantIds.value.has(grant.id)
}

function invalidate() {
  // Workspace Manage flows into Channel Members as inherited Manage, so refresh
  // the channel managers view too — otherwise the sibling tab shows a stale
  // inherited state after a grant change while runtime permissions already moved.
  return Promise.all([
    queryCache.invalidateQueries({ key: ['bot-user-access', props.botId] }),
    queryCache.invalidateQueries({ key: ['bot-channel-managers', props.botId] }),
  ])
}

async function handleCreate() {
  if (!canSubmit.value || isSaving.value) return
  isSaving.value = true
  try {
    await postBotsByBotIdUserAccess({
      path: { bot_id: props.botId },
      body: {
        subject_type: formSubjectType.value,
        user_id: formSubjectType.value === 'user' ? formUserId.value : undefined,
        permissions: buildPermissions(),
      },
      throwOnError: true,
    })
    await invalidate()
    toast.success(t('bots.access.userAccess.saved'))
    closeForm()
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.userAccess.saveFailed')))
  }
  finally {
    isSaving.value = false
  }
}

async function togglePerm(grant: BotsUserGrant, perm: Permission) {
  if (grant.is_owner || !grant.id) return
  const current = new Set<Permission>(expandBotPermissions(grant.permissions))
  if (perm !== 'manage' && current.has('manage') && current.has(perm)) {
    current.delete('manage')
  }
  if (current.has(perm)) current.delete(perm)
  else current.add(perm)
  if (perm === 'workspace_read' && !current.has('workspace_read')) current.delete('workspace_write')
  const next = normalizePermissionSelection(current)
  if (next.length === 0) {
    toast.error(t('bots.access.userAccess.atLeastOnePermission'))
    return
  }
  busyGrantIds.value.add(grant.id)
  try {
    await putBotsByBotIdUserAccessByGrantId({
      path: { bot_id: props.botId, grant_id: grant.id },
      body: { permissions: next },
      throwOnError: true,
    })
    await invalidate()
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.userAccess.saveFailed')))
  }
  finally {
    busyGrantIds.value.delete(grant.id)
  }
}

async function handleDelete(grant: BotsUserGrant) {
  if (grant.is_owner || !grant.id) return
  busyGrantIds.value.add(grant.id)
  try {
    await deleteBotsByBotIdUserAccessByGrantId({
      path: { bot_id: props.botId, grant_id: grant.id },
      throwOnError: true,
    })
    await invalidate()
    toast.success(t('bots.access.userAccess.removed'))
  }
  catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.access.userAccess.removeFailed')))
  }
  finally {
    busyGrantIds.value.delete(grant.id)
  }
}
</script>
