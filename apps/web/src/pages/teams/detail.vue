<template>
  <section class="absolute inset-0 flex flex-col bg-background">
    <div class="relative flex-1">
      <MasterDetailSidebarLayout>
        <template #sidebar-header>
          <div class="flex flex-col p-4 pb-3">
            <Button
              variant="ghost"
              size="sm"
              class="mb-3 w-fit px-2"
              @click="router.push({ name: 'teams' })"
            >
              <ArrowLeft class="mr-1.5 size-4" />
              {{ t('common.back') }}
            </Button>

            <div class="flex items-center gap-3">
              <div class="group/avatar relative size-10 shrink-0 overflow-hidden rounded-full">
                <Avatar class="size-10 rounded-full">
                  <AvatarImage
                    v-if="team?.avatar_url"
                    :src="team.avatar_url"
                    :alt="team.name"
                  />
                  <AvatarFallback class="text-sm">
                    <Users
                      v-if="!teamInitials"
                      class="size-4"
                    />
                    <template v-else>
                      {{ teamInitials }}
                    </template>
                  </AvatarFallback>
                </Avatar>
                <button
                  type="button"
                  class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
                  :title="t('common.edit')"
                  :aria-label="t('common.edit')"
                  :disabled="!team"
                  @click="avatarDialogOpen = true"
                >
                  <SquarePen class="size-4 text-white" />
                </button>
              </div>
              <div class="min-w-0">
                <h2 class="truncate text-sm font-semibold text-foreground">
                  {{ team?.name ?? t('teams.title') }}
                </h2>
                <p class="mt-0.5 truncate text-[10px] text-muted-foreground">
                  {{ team?.shared_dir_name ? `/team/${team.shared_dir_name}` : t('teams.settingsDescription') }}
                </p>
              </div>
            </div>

            <Button
              v-if="team"
              variant="outline"
              size="sm"
              class="mt-4 justify-start"
              @click="router.push({ name: 'team-workspace', params: { teamId } })"
            >
              <ExternalLink class="mr-1.5 size-4" />
              {{ t('teams.openWorkspace') }}
            </Button>
          </div>
        </template>

        <template #sidebar-content>
          <SidebarMenu
            v-for="tab in teamTabs"
            :key="tab.value"
            class="m-0 p-0"
          >
            <SidebarMenuItem>
              <SidebarMenuButton
                as-child
                :is-active="activeTab === tab.value"
                class="h-10 justify-start px-0 py-0! before:hidden"
              >
                <Toggle
                  class="h-10 w-full justify-start gap-3 border-0 bg-transparent! px-3 text-xs font-medium transition-colors"
                  :model-value="activeTab === tab.value"
                  @update:model-value="(isSelect: boolean) => {
                    if (isSelect) activeTab = tab.value
                  }"
                >
                  <component
                    :is="tab.icon"
                    class="size-4 shrink-0"
                  />
                  <span class="whitespace-nowrap">{{ t(tab.labelKey) }}</span>
                </Toggle>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </template>

        <template #sidebar-footer />

        <template #detail>
          <div class="absolute inset-0 overflow-y-auto bg-background">
            <div class="space-y-6 px-6 pb-6 pt-4">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h1 class="text-lg font-semibold">
                    {{ activeTabTitle }}
                  </h1>
                  <p class="text-xs text-muted-foreground">
                    {{ activeTabDescription }}
                  </p>
                </div>
              </div>

              <div
                v-if="activeTab === 'overview'"
                class="space-y-4"
              >
                <Card>
                  <CardHeader>
                    <CardTitle>{{ t('teams.teamSummary') }}</CardTitle>
                    <CardDescription>{{ t('teams.settingsDescription') }}</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <dl class="grid gap-4 text-sm md:grid-cols-2">
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('teams.name') }}
                        </dt>
                        <dd class="mt-1 font-medium">
                          {{ team?.name || '-' }}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('teams.workspacePath') }}
                        </dt>
                        <dd class="mt-1 font-mono text-xs">
                          {{ team?.shared_dir_name ? `/team/${team.shared_dir_name}` : '-' }}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('teams.memberCount') }}
                        </dt>
                        <dd class="mt-1 font-medium">
                          {{ members.length }}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('teams.createdAt') }}
                        </dt>
                        <dd class="mt-1">
                          {{ formatDate(team?.created_at) || '-' }}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('common.updatedAt') }}
                        </dt>
                        <dd class="mt-1">
                          {{ formatDate(team?.updated_at) || '-' }}
                        </dd>
                      </div>
                      <div>
                        <dt class="text-xs text-muted-foreground">
                          {{ t('teams.id') }}
                        </dt>
                        <dd class="mt-1 truncate font-mono text-xs">
                          {{ team?.id || '-' }}
                        </dd>
                      </div>
                    </dl>
                  </CardContent>
                </Card>
              </div>

              <Card v-else-if="activeTab === 'general' && team">
                <CardHeader>
                  <CardTitle>{{ t('teams.generalSettings') }}</CardTitle>
                  <CardDescription>{{ t('teams.generalSettingsHint') }}</CardDescription>
                </CardHeader>
                <CardContent>
                  <form
                    class="space-y-4"
                    @submit.prevent="submitTeamGeneral"
                  >
                    <div class="grid gap-4 md:grid-cols-2">
                      <div class="space-y-1.5">
                        <label class="text-sm font-medium">{{ t('teams.name') }}</label>
                        <Input
                          v-model="teamForm.name"
                          required
                        />
                      </div>
                      <div class="space-y-1.5">
                        <label class="text-sm font-medium">{{ t('teams.sharedDir') }}</label>
                        <Input
                          v-model="teamForm.shared_dir_name"
                          :placeholder="t('teams.sharedDirPlaceholder')"
                        />
                      </div>
                    </div>
                    <div class="space-y-1.5">
                      <label class="text-sm font-medium">{{ t('teams.description') }}</label>
                      <Textarea
                        v-model="teamForm.description"
                        rows="3"
                      />
                    </div>
                    <div class="flex justify-end">
                      <Button
                        type="submit"
                        :disabled="savingTeam || !teamForm.name.trim()"
                      >
                        {{ t('common.save') }}
                      </Button>
                    </div>
                  </form>
                </CardContent>
              </Card>

              <Card v-else-if="activeTab === 'instructions' && team">
                <CardHeader>
                  <CardTitle>{{ t('teams.instructions') }}</CardTitle>
                  <CardDescription>{{ t('teams.instructionsSettingsHint') }}</CardDescription>
                </CardHeader>
                <CardContent>
                  <form
                    class="space-y-4"
                    @submit.prevent="submitTeamInstructions"
                  >
                    <div class="space-y-1.5">
                      <label class="text-sm font-medium">{{ t('teams.instructions') }}</label>
                      <Textarea
                        v-model="teamForm.instructions"
                        :placeholder="t('teams.instructionsPlaceholder')"
                        rows="8"
                      />
                    </div>
                    <div class="flex justify-end">
                      <Button
                        type="submit"
                        :disabled="savingTeam"
                      >
                        {{ t('common.save') }}
                      </Button>
                    </div>
                  </form>
                </CardContent>
              </Card>

              <Card v-else-if="activeTab === 'members'">
                <CardHeader class="flex flex-row items-center justify-between gap-3">
                  <div>
                    <CardTitle>{{ t('teams.members') }}</CardTitle>
                    <CardDescription>{{ t('teams.membersHint') }}</CardDescription>
                  </div>
                  <Button
                    size="sm"
                    @click="openAddMember"
                  >
                    <Plus class="mr-1.5 size-4" />
                    {{ t('teams.addMember') }}
                  </Button>
                </CardHeader>
                <CardContent>
                  <div
                    v-if="members.length === 0"
                    class="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground"
                  >
                    {{ t('teams.noMembers') }}
                  </div>
                  <ul
                    v-else
                    class="divide-y rounded-md border"
                  >
                    <li
                      v-for="member in members"
                      :key="member.id"
                      class="flex flex-wrap items-center justify-between gap-3 px-3 py-3 text-sm"
                    >
                      <div class="flex min-w-0 items-center gap-3">
                        <Avatar class="size-8">
                          <AvatarFallback class="text-xs">
                            {{ initials(memberLabel(member)) }}
                          </AvatarFallback>
                        </Avatar>
                        <div class="min-w-0">
                          <div class="flex flex-wrap items-center gap-2">
                            <span class="truncate font-medium">{{ memberLabel(member) }}</span>
                            <Badge variant="outline">
                              {{ member.member_type }}
                            </Badge>
                            <Badge
                              v-if="member.role"
                              variant="secondary"
                            >
                              {{ member.role }}
                            </Badge>
                          </div>
                          <p
                            v-if="member.instructions"
                            class="mt-1 line-clamp-1 text-xs text-muted-foreground"
                          >
                            {{ member.instructions }}
                          </p>
                        </div>
                      </div>
                      <div class="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          @click="openEditMember(member)"
                        >
                          <Edit3 class="mr-1.5 size-3.5" />
                          {{ t('common.edit') }}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          @click="removeMember(member.id)"
                        >
                          {{ t('common.remove') }}
                        </Button>
                      </div>
                    </li>
                  </ul>
                </CardContent>
              </Card>

              <Card v-else-if="activeTab === 'danger'">
                <CardHeader>
                  <CardTitle>{{ t('common.dangerZone') }}</CardTitle>
                  <CardDescription>{{ t('teams.dangerHint') }}</CardDescription>
                </CardHeader>
                <CardContent class="flex flex-wrap items-center justify-between gap-3">
                  <div class="text-sm text-muted-foreground">
                    {{ t('teams.deleteTeamHint') }}
                  </div>
                  <Button
                    variant="destructive"
                    :disabled="deletingTeam"
                    @click="showDeleteTeam = true"
                  >
                    <Trash2 class="mr-1.5 size-4" />
                    {{ t('teams.deleteTeam') }}
                  </Button>
                </CardContent>
              </Card>
            </div>
          </div>
        </template>
      </MasterDetailSidebarLayout>
    </div>

    <Dialog v-model:open="showMemberDialog">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ editingMember ? t('teams.editMember') : t('teams.addMember') }}</DialogTitle>
          <DialogDescription>{{ t('teams.memberDialogHint') }}</DialogDescription>
        </DialogHeader>
        <form
          class="space-y-4"
          @submit.prevent="submitMember"
        >
          <div
            v-if="!editingMember"
            class="space-y-1.5"
          >
            <label class="text-sm font-medium">{{ t('teams.memberType') }}</label>
            <Select v-model="memberForm.member_type">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="bot">
                  bot
                </SelectItem>
                <SelectItem value="user">
                  user
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div
            v-if="!editingMember && memberForm.member_type === 'bot'"
            class="space-y-1.5"
          >
            <label class="text-sm font-medium">{{ t('teams.botId') }}</label>
            <BotSelect v-model="memberForm.bot_id" />
          </div>
          <div
            v-else-if="!editingMember"
            class="space-y-1.5"
          >
            <label class="text-sm font-medium">{{ t('teams.userId') }}</label>
            <Input v-model="memberForm.user_id" />
          </div>
          <div
            v-if="editingMember"
            class="rounded-md border bg-muted/40 p-3 text-sm"
          >
            <span class="text-muted-foreground">{{ t('teams.member') }}: </span>
            <span class="font-medium">{{ memberLabel(editingMember) }}</span>
          </div>
          <div class="space-y-1.5">
            <label class="text-sm font-medium">{{ t('teams.role') }}</label>
            <Input
              v-model="memberForm.role"
              :placeholder="t('teams.rolePlaceholder')"
            />
          </div>
          <div class="space-y-1.5">
            <label class="text-sm font-medium">{{ t('teams.memberInstructions') }}</label>
            <Textarea
              v-model="memberForm.instructions"
              rows="4"
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              @click="showMemberDialog = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              :disabled="savingMember"
            >
              {{ t('common.save') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="showDeleteTeam">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('teams.deleteTeam') }}</DialogTitle>
          <DialogDescription>{{ t('teams.deleteTeamConfirm') }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="ghost"
            @click="showDeleteTeam = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deletingTeam"
            @click="deleteTeam"
          >
            {{ t('common.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="avatarUrlModel"
      :fallback-text="teamInitials"
      :title="t('teams.editAvatar')"
      :description="t('teams.editAvatarDescription')"
      :placeholder="t('teams.avatarUrlPlaceholder')"
    />
  </section>
</template>

<script setup lang="ts">
import type {
  HandlersMemberResponse,
  HandlersUpdateTeamRequest,
} from '@memohai/sdk'
import type { Component } from 'vue'
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { toast } from 'vue-sonner'
import {
  AlertTriangle,
  ArrowLeft,
  Edit3,
  ExternalLink,
  FileText,
  LayoutDashboard,
  Plus,
  Settings,
  SquarePen,
  Trash2,
  Users,
} from 'lucide-vue-next'
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Textarea,
  Toggle,
} from '@memohai/ui'
import AvatarEditDialog from '@/components/avatar-edit-dialog/index.vue'
import {
  deleteTeamsByTeamId,
  deleteTeamsByTeamIdMembersByMemberId,
  getTeamsByTeamId,
  getTeamsByTeamIdMembers,
  postTeamsByTeamIdMembers,
  putTeamsByTeamId,
  putTeamsByTeamIdMembersByMemberId,
} from '@memohai/sdk'
import BotSelect from '@/components/bot-select/index.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { resolveApiErrorMessage } from '@/utils/api-error'

type TeamTab = 'overview' | 'general' | 'instructions' | 'members' | 'danger'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const queryCache = useQueryCache()

const teamId = computed(() => String(route.params.teamId ?? ''))
const activeTab = useSyncedQueryParam('tab', 'overview')

const teamTabs: Array<{ value: TeamTab, labelKey: string, descriptionKey: string, icon: Component }> = [
  { value: 'overview', labelKey: 'teams.tabs.overview', descriptionKey: 'teams.overviewHint', icon: LayoutDashboard },
  { value: 'general', labelKey: 'teams.tabs.general', descriptionKey: 'teams.generalSettingsHint', icon: Settings },
  { value: 'instructions', labelKey: 'teams.tabs.instructions', descriptionKey: 'teams.instructionsSettingsHint', icon: FileText },
  { value: 'members', labelKey: 'teams.tabs.members', descriptionKey: 'teams.membersHint', icon: Users },
  { value: 'danger', labelKey: 'teams.tabs.danger', descriptionKey: 'teams.dangerHint', icon: AlertTriangle },
]

const activeTabConfig = computed(() =>
  teamTabs.find((tab) => tab.value === activeTab.value) ?? teamTabs[0],
)
const activeTabTitle = computed(() => t(activeTabConfig.value.labelKey))
const activeTabDescription = computed(() => t(activeTabConfig.value.descriptionKey))

const { data: teamData } = useQuery({
  key: () => ['team', teamId.value],
  query: async () => {
    const { data, error } = await getTeamsByTeamId({ path: { team_id: teamId.value } })
    if (error) throw error
    return data
  },
  enabled: () => !!teamId.value,
})

const { data: membersData } = useQuery({
  key: () => ['team', teamId.value, 'members'],
  query: async () => {
    const { data, error } = await getTeamsByTeamIdMembers({ path: { team_id: teamId.value } })
    if (error) throw error
    return data ?? []
  },
  enabled: () => !!teamId.value,
})

const team = computed(() => teamData.value)
const members = computed(() => membersData.value ?? [])

const teamForm = reactive({
  name: '',
  description: '',
  shared_dir_name: '',
  instructions: '',
})

watch(team, (next) => {
  teamForm.name = next?.name ?? ''
  teamForm.description = next?.description ?? ''
  teamForm.shared_dir_name = next?.shared_dir_name ?? ''
  teamForm.instructions = next?.instructions ?? ''
  if (next?.name) {
    route.meta.breadcrumb = () => next.name
  }
}, { immediate: true })

const avatarDialogOpen = ref(false)

const teamInitials = computed(() => {
  const label = (team.value?.name ?? '').trim()
  if (!label) return ''
  return label.slice(0, 2).toUpperCase()
})

// 头像 v-model 桥接：双向绑定到当前 team.avatar_url，
// 写入时直接调用 PUT /teams/:team_id 保存，对话框关闭即写库。
const avatarUrlModel = computed<string>({
  get: () => team.value?.avatar_url ?? '',
  set: (next) => {
    const trimmed = (next ?? '').trim()
    if (trimmed === (team.value?.avatar_url ?? '')) return
    void saveTeamPatch({ avatar_url: trimmed })
  },
})

const showMemberDialog = ref(false)
const editingMember = ref<HandlersMemberResponse | null>(null)
const memberForm = reactive({
  member_type: 'bot' as 'bot' | 'user',
  bot_id: '',
  user_id: '',
  role: '',
  instructions: '',
})
const showDeleteTeam = ref(false)

const { mutateAsync: updateTeamMutation, isLoading: savingTeam } = useMutation({
  mutation: async (body: HandlersUpdateTeamRequest) => {
    const { data, error } = await putTeamsByTeamId({
      path: { team_id: teamId.value },
      body,
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: addMemberMutation, isLoading: addingMember } = useMutation({
  mutation: async () => {
    const { data, error } = await postTeamsByTeamIdMembers({
      path: { team_id: teamId.value },
      body: {
        member_type: memberForm.member_type,
        bot_id: memberForm.bot_id,
        user_id: memberForm.user_id,
        role: memberForm.role.trim(),
        instructions: memberForm.instructions.trim(),
      },
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: updateMemberMutation, isLoading: updatingMember } = useMutation({
  mutation: async (memberId: string) => {
    const { data, error } = await putTeamsByTeamIdMembersByMemberId({
      path: { team_id: teamId.value, member_id: memberId },
      body: {
        role: memberForm.role.trim(),
        instructions: memberForm.instructions.trim(),
      },
    })
    if (error) throw error
    return data
  },
})

const { mutateAsync: removeMemberMutation } = useMutation({
  mutation: async (memberId: string) => {
    const { error } = await deleteTeamsByTeamIdMembersByMemberId({
      path: { team_id: teamId.value, member_id: memberId },
    })
    if (error) throw error
  },
})

const { mutateAsync: deleteTeamMutation, isLoading: deletingTeam } = useMutation({
  mutation: async () => {
    const { error } = await deleteTeamsByTeamId({
      path: { team_id: teamId.value },
    })
    if (error) throw error
  },
})

const savingMember = computed(() => addingMember.value || updatingMember.value)

async function saveTeamPatch(body: HandlersUpdateTeamRequest) {
  try {
    await updateTeamMutation(body)
    toast.success(t('teams.updateSuccess'))
    void queryCache.invalidateQueries({ key: ['teams'] })
    void queryCache.invalidateQueries({ key: ['team', teamId.value] })
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.updateFailed')))
  }
}

function submitTeamGeneral() {
  if (!teamForm.name.trim()) return
  void saveTeamPatch({
    name: teamForm.name.trim(),
    description: teamForm.description.trim(),
    shared_dir_name: teamForm.shared_dir_name.trim(),
  })
}

function submitTeamInstructions() {
  void saveTeamPatch({
    instructions: teamForm.instructions.trim(),
  })
}

function openAddMember() {
  editingMember.value = null
  memberForm.member_type = 'bot'
  memberForm.bot_id = ''
  memberForm.user_id = ''
  memberForm.role = ''
  memberForm.instructions = ''
  showMemberDialog.value = true
}

function openEditMember(member: HandlersMemberResponse) {
  editingMember.value = member
  memberForm.member_type = member.member_type === 'user' ? 'user' : 'bot'
  memberForm.bot_id = member.bot_id ?? ''
  memberForm.user_id = member.user_id ?? ''
  memberForm.role = member.role ?? ''
  memberForm.instructions = member.instructions ?? ''
  showMemberDialog.value = true
}

async function submitMember() {
  try {
    if (editingMember.value?.id) {
      await updateMemberMutation(editingMember.value.id)
      toast.success(t('teams.memberUpdated'))
    } else {
      await addMemberMutation()
      toast.success(t('teams.memberAdded'))
    }
    showMemberDialog.value = false
    void queryCache.invalidateQueries({ key: ['team', teamId.value, 'members'] })
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, editingMember.value ? t('teams.memberUpdateFailed') : t('teams.memberAddFailed')))
  }
}

async function removeMember(id: string | undefined) {
  if (!id) return
  try {
    await removeMemberMutation(id)
    toast.success(t('teams.memberRemoved'))
    void queryCache.invalidateQueries({ key: ['team', teamId.value, 'members'] })
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.memberRemoveFailed')))
  }
}

async function deleteTeam() {
  try {
    await deleteTeamMutation()
    toast.success(t('teams.teamDeleted'))
    showDeleteTeam.value = false
    void queryCache.invalidateQueries({ key: ['teams'] })
    router.push({ name: 'teams' })
  }
  catch (err) {
    toast.error(resolveApiErrorMessage(err, t('teams.teamDeleteFailed')))
  }
}

function memberLabel(member: HandlersMemberResponse) {
  return member.display_name || member.bot_id || member.user_id || t('teams.unknownMember')
}

function initials(name: string): string {
  return name
    .split(/[\s_-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0])
    .join('')
    .toUpperCase() || '?'
}

function formatDate(value: string | undefined): string {
  if (!value) return ''
  try {
    return new Date(value).toLocaleString()
  }
  catch {
    return value
  }
}
</script>
